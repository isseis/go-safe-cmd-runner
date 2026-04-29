package filevalidator

import (
	"debug/elf"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/security/binaryanalyzer"
	elfanalyzertesting "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSafeReadFile is a helper function for tests to safely read files.
// It enforces that the file is within the test directory.
// Only for tests. This doesn't check symlinks.
func testSafeReadFile(testDir, filePath string) ([]byte, error) {
	// Clean and validate the file path
	cleanPath := filepath.Clean(filePath)

	// Ensure the path is within the test directory
	relPath, err := filepath.Rel(testDir, cleanPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return nil, os.ErrNotExist
	}

	return os.ReadFile(cleanPath)
}

// safeTempDir creates a temporary directory and resolves any symlinks in its path
// to ensure consistent behavior across different environments.
func safeTempDir(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	// Resolve any symlinks in the path
	realPath, err := filepath.EvalSymlinks(tempDir)
	require.NoErrorf(t, err, "Failed to resolve symlinks in temp dir: %v", err)
	return realPath
}

func TestValidator_RecordAndVerify(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	err := os.WriteFile(testFilePath, []byte("test content"), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// Resolve any symlinks in the test file path
	testFilePath, err = filepath.EvalSymlinks(testFilePath)
	require.NoErrorf(t, err, "Failed to resolve symlinks in test file path: %v", err)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Test SaveRecord
	t.Run("SaveRecord", func(t *testing.T) {
		_, _, err := validator.SaveRecord(testFilePath, false)
		require.NoError(t, err, "SaveRecord failed")

		// Verify the hash file exists
		rp, err := common.NewResolvedPath(testFilePath)
		require.NoError(t, err, "NewResolvedPath failed")
		hashFilePath, err := validator.HashFilePath(rp)
		require.NoError(t, err, "HashFilePath failed")

		_, err = os.Lstat(hashFilePath)
		assert.NoError(t, err)
	})

	// Test Verify with unmodified file
	t.Run("Verify unmodified", func(t *testing.T) {
		err = validator.Verify(testFilePath)
		require.NoError(t, err, "Verify should succeed with unmodified file")
	})

	// Test Verify with modified file
	t.Run("Verify modified", func(t *testing.T) {
		// Modify the file
		err := os.WriteFile(testFilePath, []byte("modified content"), 0o644)
		require.NoError(t, err, "Failed to modify test file")

		err = validator.Verify(testFilePath)
		assert.Error(t, err, "Expected error with modified file")
		assert.ErrorIs(t, err, ErrMismatch, "Expected ErrMismatch")
	})

	// Test Verify with non-existent file
	t.Run("Verify non-existent", func(t *testing.T) {
		err := validator.Verify(filepath.Join(tempDir, "nonexistent.txt"))
		assert.Error(t, err, "Expected an error for non-existent file")
	})
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		algo    HashAlgorithm
		hashDir string
		wantErr bool
	}{
		{
			name:    "valid",
			algo:    &SHA256{},
			hashDir: safeTempDir(t),
			wantErr: false,
		},
		{
			name:    "nil algorithm",
			algo:    nil,
			hashDir: safeTempDir(t),
			wantErr: true,
		},
		{
			name:    "non-existent directory",
			algo:    &SHA256{},
			hashDir: "/non/existent/dir",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.algo, tt.hashDir)
			if tt.wantErr {
				assert.Error(t, err, "New() should return an error")
			} else {
				assert.NoError(t, err, "New() should not return an error")
			}
		})
	}
}

func TestValidator_Record_Symlink(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	require.NoError(t, os.WriteFile(testFilePath, []byte("test content"), 0o644), "Failed to create test file")

	// Get the real path (resolving symlinks)
	resolvedTestFilePath, err := filepath.EvalSymlinks(testFilePath)
	require.NoError(t, err, "Failed to resolve symlinks in test file path")

	// Create a symlink to the test file
	symlinkPath := filepath.Join(tempDir, "symlink.txt")
	require.NoError(t, os.Symlink(resolvedTestFilePath, symlinkPath), "Failed to create symlink")

	// Resolve the symlink to get the expected path
	resolvedSymlinkPath, err := filepath.EvalSymlinks(symlinkPath)
	require.NoError(t, err, "Failed to resolve symlink")
	expectedPath := resolvedSymlinkPath

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Test SaveRecord with symlink
	// Symlinks are resolved before writing the hash file
	_, _, err = validator.SaveRecord(symlinkPath, false)
	assert.NoError(t, err, "SaveRecord failed")

	// Use store.Load() to verify the recorded path and hash
	store := validator.Store()
	require.NotNil(t, store, "Store should not be nil")

	rpSymlink, err := common.NewResolvedPath(resolvedSymlinkPath)
	require.NoError(t, err, "NewResolvedPath failed")
	record, err := store.Load(rpSymlink)
	assert.NoError(t, err, "store.Load failed")
	assert.Equal(t, expectedPath, record.FilePath, "Expected recorded path '%s', got '%s'", expectedPath, record.FilePath)
	assert.NotEmpty(t, record.ContentHash, "Expected non-empty content hash, got empty")
}

func TestValidator_Verify_Symlink(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	require.NoError(t, os.WriteFile(testFilePath, []byte("test content"), 0o644), "Failed to create test file")

	// Resolve any symlinks in the test file path
	testFilePath, err := filepath.EvalSymlinks(testFilePath)
	require.NoError(t, err, "Failed to resolve symlinks in test file path")

	// Create a validator and record the original file
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	_, _, err = validator.SaveRecord(testFilePath, false)
	require.NoError(t, err, "SaveRecord failed")

	// Create a symlink to the test file
	symlinkPath := filepath.Join(tempDir, "symlink.txt")
	require.NoError(t, os.Symlink(testFilePath, symlinkPath), "Failed to create symlink")

	// Test Verify with symlink
	err = validator.Verify(symlinkPath)
	assert.NoError(t, err, "Verify failed")
}

func TestValidator_Record_EmptyHashFile(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	require.NoError(t, os.WriteFile(testFilePath, []byte("test content"), 0o644), "Failed to create test file")

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Get the hash file path
	rpCorrupt, err := common.NewResolvedPath(testFilePath)
	require.NoError(t, err, "NewResolvedPath failed")
	hashFilePath, err := validator.HashFilePath(rpCorrupt)
	require.NoError(t, err, "HashFilePath failed")

	// Create the hash directory
	require.NoError(t, os.MkdirAll(filepath.Dir(hashFilePath), 0o750), "Failed to create hash directory")

	// Create an empty (corrupted) hash file
	require.NoError(t, os.WriteFile(hashFilePath, []byte(""), 0o640), "Failed to create empty hash file")

	// With the FileAnalysisRecord format, a corrupted/empty file is treated as "no valid record"
	// and is overwritten. SaveRecord() should succeed, creating a valid record.
	hashFile, _, err := validator.SaveRecord(testFilePath, false)
	require.NoError(t, err, "SaveRecord should succeed by overwriting the corrupted empty file")

	// The record file should now contain valid content
	content, err := os.ReadFile(hashFile)
	require.NoError(t, err, "Failed to read hash file")
	assert.NotEmpty(t, content, "Hash file should not be empty after SaveRecord")
}

// TestValidator_FileAnalysisRecordFormat tests that hash files are created in FileAnalysisRecord format
func TestValidator_FileAnalysisRecordFormat(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	require.NoError(t, os.WriteFile(testFilePath, []byte("test content"), 0o644), "Failed to create test file")

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Save the file record
	_, _, err = validator.SaveRecord(testFilePath, false)
	require.NoError(t, err, "SaveRecord failed")

	// Load from store and verify the FileAnalysisRecord format
	store := validator.Store()
	require.NotNil(t, store, "Store should not be nil")

	rpFormat, err := common.NewResolvedPath(testFilePath)
	require.NoError(t, err, "NewResolvedPath failed")
	record, err := store.Load(rpFormat)
	require.NoError(t, err, "Failed to load record from store")

	// Verify the record fields
	assert.Equal(t, testFilePath, record.FilePath, "File path is empty")
	assert.True(t, strings.HasPrefix(record.ContentHash, "sha256:"),
		"ContentHash should have sha256: prefix, got: %s", record.ContentHash)
}

// TestValidator_HashAlgorithmConsistency tests that the validator uses the configured
// hash algorithm consistently in both recording and verification.
// This test would fail with hardcoded sha256.Sum256() but passes with v.algorithm.Sum().
func TestValidator_HashAlgorithmConsistency(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file with content that will produce different hashes with different algorithms
	// Using content that's shorter than 64 chars to ensure MockHashAlgorithm pads with zeros
	testContent := "short"
	testFilePath := filepath.Join(tempDir, "test_file.txt")
	err := os.WriteFile(testFilePath, []byte(testContent), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// Create validator with MockHashAlgorithm (not SHA-256)
	mockAlgo := &MockHashAlgorithm{}
	validator, err := New(mockAlgo, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Save the file record - this should use mockAlgo.Sum()
	hashFilePath, _, err := validator.SaveRecord(testFilePath, false)
	require.NoError(t, err, "Failed to record file")

	// Verify the file - this should also use mockAlgo.Sum()
	// If the code was still using hardcoded sha256.Sum256(), this would fail
	// because the hash in the recorded file would be from MockHashAlgorithm
	// but the verification would use SHA-256
	err = validator.Verify(testFilePath)
	assert.NoError(t, err, "Verification failed - this indicates hash algorithm inconsistency")

	// Additional verification: Check that the hash file (FileAnalysisRecord) contains
	// the expected algorithm name and hash in the content_hash field.
	hashFileContent, err := testSafeReadFile(tempDir, hashFilePath)
	require.NoError(t, err, "Failed to read hash file")

	var record map[string]any
	err = json.Unmarshal(hashFileContent, &record)
	require.NoError(t, err, "Failed to unmarshal hash file")

	// Verify the FileAnalysisRecord structure: record["content_hash"] = "mock:<hash>"
	require.Contains(t, record, "content_hash", "Hash file should contain 'content_hash' field")
	contentHash, ok := record["content_hash"].(string)
	require.True(t, ok, "content_hash should be a string")

	// Verify the algorithm name is correctly stored as prefix
	assert.True(t, strings.HasPrefix(contentHash, "mock:"),
		"content_hash should have 'mock:' prefix, got: %s", contentHash)

	// Verify the hash value is what MockHashAlgorithm would produce
	expectedHashValue, err := mockAlgo.Sum(strings.NewReader(testContent))
	require.NoError(t, err, "Failed to calculate expected hash")

	assert.Equal(t, "mock:"+expectedHashValue, contentHash,
		"content_hash should contain the hash from MockHashAlgorithm")
}

// TestValidator_CrossAlgorithmVerificationFails tests that verification properly fails when
// attempting to verify a file that was recorded with a different algorithm.
// This ensures proper algorithm validation and prevents security issues.
func TestValidator_CrossAlgorithmVerificationFails(t *testing.T) {
	tempDir := safeTempDir(t)

	testFilePath := filepath.Join(tempDir, "cross_algo_test.txt")
	err := os.WriteFile(testFilePath, []byte("test content"), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// SaveRecord with MockHashAlgorithm
	mockValidator, err := New(&MockHashAlgorithm{}, tempDir)
	require.NoError(t, err, "Failed to create mock validator")
	_, _, err = mockValidator.SaveRecord(testFilePath, false)
	require.NoError(t, err, "Failed to record file with MockHashAlgorithm")

	// Attempt verification with SHA-256 validator (different algorithm)
	sha256Validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create SHA-256 validator")

	// This should fail because the stored hash uses mock: prefix but sha256 validator
	// computes sha256: prefix — the two content_hash values will not match.
	err = sha256Validator.Verify(testFilePath)
	assert.Error(t, err, "Cross-algorithm verification should fail")
	assert.ErrorIs(t, err, ErrMismatch, "Expected ErrMismatch for cross-algorithm verification")
}

// TestValidator_VerifyAndRead tests the VerifyAndRead method which atomically
// verifies file integrity and returns its content to prevent TOCTOU attacks.
func TestValidator_VerifyAndRead(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	testContent := "test content for VerifyAndRead"

	t.Run("successful verification and read", func(t *testing.T) {
		// Create a test file
		testFile := createTestFile(t, testContent)

		// Save the record
		_, _, err = validator.SaveRecord(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// VerifyAndRead should succeed
		content, err := validator.VerifyAndRead(testFile)
		assert.NoError(t, err, "VerifyAndRead should succeed")
		assert.Equal(t, testContent, string(content), "Content should match")
	})

	t.Run("file content mismatch", func(t *testing.T) {
		// Create a test file and record its hash
		testFile := createTestFile(t, testContent)
		_, _, err = validator.SaveRecord(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// Modify the file content
		modifiedContent := "modified content"
		err = os.WriteFile(testFile, []byte(modifiedContent), 0o644)
		require.NoError(t, err, "Failed to modify test file")

		// VerifyAndRead should fail with mismatch error
		content, err := validator.VerifyAndRead(testFile)
		assert.Error(t, err, "VerifyAndRead should fail with modified file")
		assert.ErrorIs(t, err, ErrMismatch, "Should return ErrMismatch")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("missing hash file", func(t *testing.T) {
		// Create a test file but don't record its hash
		testFile := createTestFile(t, testContent)

		// VerifyAndRead should fail with hash file not found
		content, err := validator.VerifyAndRead(testFile)
		assert.Error(t, err, "VerifyAndRead should fail without hash file")
		assert.ErrorIs(t, err, ErrHashFileNotFound, "Should return ErrHashFileNotFound")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("non-existent file", func(t *testing.T) {
		nonExistentFile := filepath.Join(tempDir, "non_existent.txt")

		// VerifyAndRead should fail with file not found
		content, err := validator.VerifyAndRead(nonExistentFile)
		assert.Error(t, err, "VerifyAndRead should fail with non-existent file")
		assert.True(t, os.IsNotExist(err), "Should return file not found error")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("invalid file path", func(t *testing.T) {
		// Test with empty path
		content, err := validator.VerifyAndRead("")
		assert.Error(t, err, "VerifyAndRead should fail with empty path")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("directory instead of file", func(t *testing.T) {
		// Create a directory
		dirPath := filepath.Join(tempDir, "test_directory")
		err = os.MkdirAll(dirPath, 0o755)
		require.NoError(t, err, "Failed to create directory")

		// VerifyAndRead should fail with directory path
		content, err := validator.VerifyAndRead(dirPath)
		assert.Error(t, err, "VerifyAndRead should fail with directory path")
		assert.Nil(t, content, "Content should be nil on error")
	})
}

// TestValidator_VerifyAndRead_TOCTOUPrevention tests that VerifyAndRead methods
// properly prevent Time-of-check to time-of-use attacks by performing atomic operations.
func TestValidator_VerifyAndRead_TOCTOUPrevention(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	testContent := "original content for TOCTOU test"

	t.Run("VerifyAndRead atomic operation", func(t *testing.T) {
		// Create a test file and record its hash
		testFile := createTestFile(t, testContent)
		_, _, err = validator.SaveRecord(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// VerifyAndRead should return the content that matches the hash
		// Even if the file is modified between verification and reading,
		// the method should detect the mismatch because it reads the content
		// and then verifies the hash of that same content
		content, err := validator.VerifyAndRead(testFile)
		assert.NoError(t, err, "VerifyAndRead should succeed")
		assert.Equal(t, testContent, string(content), "Content should match original")
	})

	t.Run("verify read consistency", func(t *testing.T) {
		// This test verifies that VerifyAndRead reads and verifies the same content
		// to prevent TOCTOU attacks where the file could be modified between
		// reading and verification.
		testFile := createTestFile(t, testContent)
		_, _, err = validator.SaveRecord(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// Test VerifyAndRead twice; both reads must be consistent.
		content1, err := validator.VerifyAndRead(testFile)
		require.NoError(t, err, "VerifyAndRead should succeed")

		content2, err := validator.VerifyAndRead(testFile)
		require.NoError(t, err, "VerifyAndRead should succeed on second read")

		// Both reads should return the same content.
		assert.Equal(t, content1, content2, "Both reads should return identical content")
		assert.Equal(t, testContent, string(content1), "Content should match original")
	})
}

// TestNew_RecordAndVerify tests that New creates a validator
// that uses the FileAnalysisRecord format and preserves existing fields.
func TestNew_RecordAndVerify(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator with analysis store support
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator with analysis store")

	// Verify that Store returns a non-nil store
	store := validator.Store()
	require.NotNil(t, store, "Store should return non-nil store")

	// Create a test file
	testContent := "test content for analysis store"
	testFilePath := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0o644)
	require.NoError(t, err, "Failed to create test file")

	t.Run("SaveRecord with analysis store format", func(t *testing.T) {
		// Save the record
		_, _, err = validator.SaveRecord(testFilePath, false)
		require.NoError(t, err, "Failed to record hash")

		// Load record directly from store to verify format
		rpStore, err := common.NewResolvedPath(testFilePath)
		require.NoError(t, err, "NewResolvedPath failed")
		record, err := store.Load(rpStore)
		require.NoError(t, err, "Failed to load record from store")

		// Verify the record fields
		assert.Equal(t, testFilePath, record.FilePath, "FilePath should match")
		assert.True(t, strings.HasPrefix(record.ContentHash, "sha256:"), "ContentHash should have sha256 prefix")
	})

	t.Run("Verify with analysis store format", func(t *testing.T) {
		// Verify the hash
		err = validator.Verify(testFilePath)
		assert.NoError(t, err, "Verify should succeed")
	})

	t.Run("Verify modified file fails", func(t *testing.T) {
		// Modify the file
		err = os.WriteFile(testFilePath, []byte("modified content"), 0o644)
		require.NoError(t, err, "Failed to modify test file")

		// Verify should fail
		err = validator.Verify(testFilePath)
		assert.ErrorIs(t, err, ErrMismatch, "Verify should fail with modified file")
	})
}

// TestNew_PreservesExistingFields tests that updating a record
// preserves existing fields like SyscallAnalysis.
func TestNew_PreservesExistingFields(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator with analysis store support
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator with analysis store")

	store := validator.Store()
	require.NotNil(t, store, "Store should return non-nil store")

	// Create a test file
	testContent := "test content for preserving fields"
	testFilePath := filepath.Join(tempDir, "test_preserve.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// First, save a record with SyscallAnalysis directly via store
	rpPreserve, err := common.NewResolvedPath(testFilePath)
	require.NoError(t, err, "NewResolvedPath failed")
	err = store.Save(rpPreserve, &fileanalysis.Record{
		ContentHash: "sha256:old_hash",
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				Architecture:     "x86_64",
				AnalysisWarnings: []string{"test reason"},
			},
		},
	})
	require.NoError(t, err, "Failed to save initial record")

	// Now use validator.SaveRecord with force to update the content hash
	_, _, err = validator.SaveRecord(testFilePath, true)
	require.NoError(t, err, "Failed to record hash with force")

	// Load the record and verify SyscallAnalysis is preserved
	rpPreserveLoad, err := common.NewResolvedPath(testFilePath)
	require.NoError(t, err, "NewResolvedPath failed")
	record, err := store.Load(rpPreserveLoad)
	require.NoError(t, err, "Failed to load updated record")

	// Verify the content hash was updated
	assert.True(t, strings.HasPrefix(record.ContentHash, "sha256:"), "ContentHash should have sha256 prefix")
	assert.NotEqual(t, "sha256:old_hash", record.ContentHash, "ContentHash should be updated")

	// Verify the SyscallAnalysis was preserved
	require.NotNil(t, record.SyscallAnalysis, "SyscallAnalysis should be preserved")
	assert.Equal(t, "x86_64", record.SyscallAnalysis.Architecture, "Architecture should be preserved")
	require.Len(t, record.SyscallAnalysis.AnalysisWarnings, 1, "AnalysisWarnings should be preserved")
	assert.Equal(t, "test reason", record.SyscallAnalysis.AnalysisWarnings[0], "AnalysisWarning content should be preserved")
}

// TestNew_CreatesDirectory tests that New
// automatically creates the hash directory if it doesn't exist.
// This verifies that New() handles directory creation correctly via
// NewStore(), which creates it before newValidator() validates it.
func TestNew_CreatesDirectory(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "nonexistent_subdir")

	// Verify directory does not exist yet
	_, err := os.Stat(hashDir)
	require.True(t, os.IsNotExist(err), "hashDir should not exist before calling New")

	// New should succeed even though hashDir doesn't exist
	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err, "New should create the directory automatically")
	require.NotNil(t, validator)

	// Verify directory was created
	info, err := os.Stat(hashDir)
	require.NoError(t, err, "hashDir should exist after New")
	assert.True(t, info.IsDir(), "hashDir should be a directory")

	// Verify the store is usable
	store := validator.Store()
	require.NotNil(t, store, "Store should return non-nil store")
}

// collidingHashFilePathGetter always maps every file path to the same
// record file, simulating a hash file path collision.
type collidingHashFilePathGetter struct {
	fixedName string
}

func (c *collidingHashFilePathGetter) GetHashFilePath(hashDir common.ResolvedPath, _ common.ResolvedPath) (string, error) {
	return filepath.Join(hashDir.String(), c.fixedName), nil
}

// newCollisionValidator creates a Validator whose HashFilePathGetter always
// returns the same record path, so that recording two different files will
// trigger a collision on the second call.
func newCollisionValidator(t *testing.T) (*Validator, string) {
	t.Helper()
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o750))

	getter := &collidingHashFilePathGetter{fixedName: "collision.json"}

	store, err := fileanalysis.NewStore(hashDir, getter)
	require.NoError(t, err)

	resolvedHashDir, err := common.NewResolvedPath(hashDir)
	require.NoError(t, err)

	v, err := newValidator(&SHA256{}, resolvedHashDir, getter)
	require.NoError(t, err)
	v.store = store

	return v, tempDir
}

func TestValidator_Record_HashFilePathCollision(t *testing.T) {
	v, tempDir := newCollisionValidator(t)

	// Create two different test files
	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(tempDir, "file2.txt")
	require.NoError(t, os.WriteFile(file1, []byte("content1"), 0o644))
	require.NoError(t, os.WriteFile(file2, []byte("content2"), 0o644))

	// SaveRecord first file — should succeed
	_, _, err := v.SaveRecord(file1, false)
	require.NoError(t, err, "first SaveRecord should succeed")

	// SaveRecord second file (different path, same record file) — should fail with collision
	_, _, err = v.SaveRecord(file2, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHashFilePathCollision,
		"second SaveRecord should detect collision")

	// Even with force=true, collision should still be detected
	_, _, err = v.SaveRecord(file2, true)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHashFilePathCollision,
		"force=true should not bypass collision detection")
}

func TestValidator_Verify_HashFilePathCollision(t *testing.T) {
	v, tempDir := newCollisionValidator(t)

	// Create two different test files
	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(tempDir, "file2.txt")
	require.NoError(t, os.WriteFile(file1, []byte("content1"), 0o644))
	require.NoError(t, os.WriteFile(file2, []byte("content2"), 0o644))

	// SaveRecord first file
	_, _, err := v.SaveRecord(file1, false)
	require.NoError(t, err)

	// Verify second file — record belongs to file1, should detect collision
	err = v.Verify(file2)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHashFilePathCollision,
		"Verify should detect collision when record belongs to a different file")
}

// TestAnalyze_Force verifies that record --force updates DynLibDeps.
// This covers Phase 3 completion criterion: DynLibDeps is updated by `record --force`.
func TestAnalyze_Force(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("content"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	// First record without force — succeeds.
	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	// Second record without force — fails (already exists).
	_, _, err = v.SaveRecord(targetFile, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHashFileExists)

	// SaveRecord with force=true — succeeds (updates the record).
	_, _, err = v.SaveRecord(targetFile, true)
	require.NoError(t, err, "force=true should overwrite existing record")
}

// TestRecord_BinaryAnalyzerNil_NoError verifies that SaveRecord() succeeds when
// binaryAnalyzer is nil (binary analysis disabled).
func TestRecord_BinaryAnalyzerNil_NoError(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("content"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	// binaryAnalyzer is nil by default — do not call SetBinaryAnalyzer.

	_, _, err = v.SaveRecord(targetFile, false)
	assert.NoError(t, err, "SaveRecord should succeed when binaryAnalyzer is nil")
}

// stubBinaryAnalyzer is a test double for binaryanalyzer.BinaryAnalyzer.
type stubBinaryAnalyzer struct {
	result             binaryanalyzer.AnalysisResult
	detectedSymbols    []binaryanalyzer.DetectedSymbol
	dynamicLoadSymbols []binaryanalyzer.DetectedSymbol
	err                error
}

func (s *stubBinaryAnalyzer) AnalyzeNetworkSymbols(_, _ string) binaryanalyzer.AnalysisOutput {
	return binaryanalyzer.AnalysisOutput{
		Result:             s.result,
		DetectedSymbols:    s.detectedSymbols,
		DynamicLoadSymbols: s.dynamicLoadSymbols,
		Error:              s.err,
	}
}

// recordWithBinaryAnalyzer is a test helper that records a file using the given stub analyzer.
func recordWithBinaryAnalyzer(t *testing.T, stub *stubBinaryAnalyzer) (*fileanalysis.Record, error) {
	t.Helper()
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.bin")
	require.NoError(t, os.WriteFile(targetFile, []byte("binary content"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	v.SetBinaryAnalyzer(stub)

	_, _, recErr := v.SaveRecord(targetFile, false)
	if recErr != nil {
		return nil, recErr
	}
	record, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	return record, nil
}

func TestRecord_NetworkDetected_SetsSymbolAnalysis(t *testing.T) {
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.NetworkDetected,
		detectedSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "socket", Category: "network"},
		},
	}
	record, err := recordWithBinaryAnalyzer(t, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis, "SymbolAnalysis should be set")
	require.Len(t, record.SymbolAnalysis.DetectedSymbols, 1)
	assert.Equal(t, "socket", record.SymbolAnalysis.DetectedSymbols[0])
	assert.Empty(t, record.SymbolAnalysis.DynamicLoadSymbols)
}

func TestRecord_NoNetworkSymbols_SetsSymbolAnalysis(t *testing.T) {
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.NoNetworkSymbols,
	}
	record, err := recordWithBinaryAnalyzer(t, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis, "SymbolAnalysis should be set")
	assert.Empty(t, record.SymbolAnalysis.DetectedSymbols)
}

func TestRecord_DynamicLoadSymbols_Stored(t *testing.T) {
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.NoNetworkSymbols,
		dynamicLoadSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "dlopen", Category: "dynamic_load"},
		},
	}
	record, err := recordWithBinaryAnalyzer(t, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	require.Len(t, record.SymbolAnalysis.DynamicLoadSymbols, 1)
	assert.Equal(t, "dlopen", record.SymbolAnalysis.DynamicLoadSymbols[0])
}

func TestRecord_NotSupportedBinary_SymbolAnalysisNil(t *testing.T) {
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.NotSupportedBinary,
	}
	record, err := recordWithBinaryAnalyzer(t, stub)
	require.NoError(t, err)
	assert.Nil(t, record.SymbolAnalysis, "SymbolAnalysis should be nil for non-ELF")
}

func TestRecord_StaticBinary_SymbolAnalysisNil(t *testing.T) {
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.StaticBinary,
	}
	record, err := recordWithBinaryAnalyzer(t, stub)
	require.NoError(t, err)
	assert.Nil(t, record.SymbolAnalysis, "SymbolAnalysis should be nil for static binary")
}

func TestRecord_AnalysisError_RecordNotSaved(t *testing.T) {
	stub := &stubBinaryAnalyzer{
		result: binaryanalyzer.AnalysisError,
		err:    errors.New("analysis failed"),
	}
	_, err := recordWithBinaryAnalyzer(t, stub)
	assert.Error(t, err, "SaveRecord should fail when binaryAnalyzer returns AnalysisError")
}

func TestRecord_Force_OverwritesSymbolAnalysis(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.bin")
	require.NoError(t, os.WriteFile(targetFile, []byte("binary content"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	// First record with network symbols detected.
	v.SetBinaryAnalyzer(&stubBinaryAnalyzer{
		result: binaryanalyzer.NetworkDetected,
		detectedSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "socket", Category: "network"},
		},
	})
	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	// Second record (force=true) with no network symbols: should overwrite.
	v.SetBinaryAnalyzer(&stubBinaryAnalyzer{
		result: binaryanalyzer.NoNetworkSymbols,
	})
	_, _, err = v.SaveRecord(targetFile, true)
	require.NoError(t, err)

	record, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Empty(t, record.SymbolAnalysis.DetectedSymbols,
		"SymbolAnalysis should be overwritten by second record")
}

// TestRecord_Force_NetworkToStaticBinary_ClearsSymbolAnalysis verifies that when
// a binary previously recorded as a dynamic ELF (with SymbolAnalysis set) is
// re-recorded with --force and the analyzer now returns StaticBinary, the stored
// SymbolAnalysis is cleared to nil rather than left as stale data.
func TestRecord_Force_NetworkToStaticBinary_ClearsSymbolAnalysis(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.bin")
	require.NoError(t, os.WriteFile(targetFile, []byte("binary content"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	// First record: dynamic ELF with network symbols.
	v.SetBinaryAnalyzer(&stubBinaryAnalyzer{
		result: binaryanalyzer.NetworkDetected,
		detectedSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "socket", Category: "network"},
		},
	})
	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	record, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	require.NotNil(t, record.SymbolAnalysis, "first record should have SymbolAnalysis")

	// Second record (force=true): same binary now analysed as static — SymbolAnalysis must be nil.
	v.SetBinaryAnalyzer(&stubBinaryAnalyzer{
		result: binaryanalyzer.StaticBinary,
	})
	_, _, err = v.SaveRecord(targetFile, true)
	require.NoError(t, err)

	record, loadErr = v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	assert.Nil(t, record.SymbolAnalysis,
		"SymbolAnalysis must be nil after re-recording as StaticBinary")
}

// ---------------------------------------------------------------------------
// Stub implementations for SyscallAnalyzerInterface and LibcCacheInterface
// ---------------------------------------------------------------------------

// stubLibcCache implements LibcCacheInterface for tests.
type stubLibcCache struct {
	syscalls []common.SyscallInfo
	err      error
}

func (s *stubLibcCache) GetOrCreateSyscalls(_, _ string, _ []string, _ elf.Machine) ([]common.SyscallInfo, error) {
	return s.syscalls, s.err
}

// ---------------------------------------------------------------------------
// Tests for helper functions
// ---------------------------------------------------------------------------

func TestFindLibcEntry(t *testing.T) {
	t.Run("returns_libc_entry_when_present", func(t *testing.T) {
		deps := []fileanalysis.LibEntry{
			{SOName: "libm.so.6", Path: "/lib/libm.so.6", Hash: "sha256:aaa"},
			{SOName: "libc.so.6", Path: "/lib/libc.so.6", Hash: "sha256:bbb"},
		}
		entry := findLibcEntry(deps)
		require.NotNil(t, entry)
		assert.Equal(t, "libc.so.6", entry.SOName)
	})

	t.Run("returns_nil_when_absent", func(t *testing.T) {
		deps := []fileanalysis.LibEntry{
			{SOName: "libm.so.6", Path: "/lib/libm.so.6", Hash: "sha256:aaa"},
		}
		assert.Nil(t, findLibcEntry(deps))
	})
}

func TestMergeSyscallInfos(t *testing.T) {
	libc := []common.SyscallInfo{
		{Number: 1, Name: "write", Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
		{Number: 2, Name: "read", Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
	}
	direct := []common.SyscallInfo{
		{Number: 1, Name: "write_direct", Occurrences: []common.SyscallOccurrence{{Source: ""}}},
	}

	merged := mergeSyscallInfos(libc, direct)
	byNum := make(map[int]common.SyscallInfo)
	for _, m := range merged {
		byNum[m.Number] = m
	}

	// Number 1: both libc and direct entries are merged into one group with two Occurrences.
	require.Len(t, byNum[1].Occurrences, 2)
	assert.Equal(t, "libc_symbol_import", byNum[1].Occurrences[0].Source)
	assert.Equal(t, "", byNum[1].Occurrences[1].Source)
	// Number 2: libc entry kept with one Occurrence.
	require.Len(t, byNum[2].Occurrences, 1)
	assert.Equal(t, "libc_symbol_import", byNum[2].Occurrences[0].Source)

	// Output must be sorted by Number ascending.
	require.Len(t, merged, 2)
	assert.Equal(t, 1, merged[0].Number, "first entry must be Number=1")
	assert.Equal(t, 2, merged[1].Number, "second entry must be Number=2")
}

func TestMergeSyscallInfos_SortOrder(t *testing.T) {
	// Feed entries in reverse order to confirm output is always sorted ascending.
	libc := []common.SyscallInfo{
		{Number: 99, Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
		{Number: 3, Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
		{Number: 42, Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
	}
	merged := mergeSyscallInfos(libc, nil)
	require.Len(t, merged, 3)
	assert.Equal(t, 3, merged[0].Number)
	assert.Equal(t, 42, merged[1].Number)
	assert.Equal(t, 99, merged[2].Number)
}

func TestBuildSyscallAnalysisData(t *testing.T) {
	t.Run("architecture_x86_64", func(t *testing.T) {
		all := []common.SyscallInfo{
			{Number: -1, Occurrences: []common.SyscallOccurrence{{Source: "", DeterminationMethod: "unknown:decode_failed"}}},
			{Number: 42, Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
		}
		data := buildSyscallData(all, nil, elf.EM_X86_64, nil, true)
		assert.Equal(t, "x86_64", data.Architecture)
		// All syscalls are retained (no filtering): Number=-1 and Number=42 both present.
		assert.Len(t, data.DetectedSyscalls, 2)
	})

	t.Run("architecture_arm64", func(t *testing.T) {
		all := []common.SyscallInfo{
			{Number: -1, Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
		}
		data := buildSyscallData(all, nil, elf.EM_AARCH64, nil, true)
		assert.Equal(t, "arm64", data.Architecture)
	})

	t.Run("non_network_resolved_syscall_retained", func(t *testing.T) {
		all := []common.SyscallInfo{
			{Number: 1, Name: "write", Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
			{Number: 3, Name: "read", Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}},
		}
		data := buildSyscallData(all, nil, elf.EM_X86_64, nil, true)
		// Non-network, resolved syscalls must be retained (no filtering).
		assert.Len(t, data.DetectedSyscalls, 2)
		numbers := make([]int, len(data.DetectedSyscalls))
		for i, s := range data.DetectedSyscalls {
			numbers[i] = s.Number
		}
		assert.Contains(t, numbers, 1)
		assert.Contains(t, numbers, 3)
	})
}

func TestBuildSyscallData_DebugInfo(t *testing.T) {
	all := []common.SyscallInfo{
		{
			Number: 1,
			Name:   "write",
			Occurrences: []common.SyscallOccurrence{{
				Location:            0x1000,
				DeterminationMethod: "immediate",
				Source:              "immediate",
			}},
		},
	}
	stats := &common.SyscallDeterminationStats{ImmediateTotal: 1}

	withoutDebug := buildSyscallData(all, nil, elf.EM_X86_64, stats, false)
	require.NotNil(t, withoutDebug)
	require.Len(t, withoutDebug.DetectedSyscalls, 1)
	assert.Nil(t, withoutDebug.DetectedSyscalls[0].Occurrences)
	assert.Nil(t, withoutDebug.DeterminationStats)

	withDebug := buildSyscallData(all, nil, elf.EM_X86_64, stats, true)
	require.NotNil(t, withDebug)
	require.Len(t, withDebug.DetectedSyscalls, 1)
	require.Len(t, withDebug.DetectedSyscalls[0].Occurrences, 1)
	assert.Equal(t, uint64(0x1000), withDebug.DetectedSyscalls[0].Occurrences[0].Location)
	require.NotNil(t, withDebug.DeterminationStats)
	assert.Equal(t, 1, withDebug.DeterminationStats.ImmediateTotal)
}

func TestRecord_DebugInfo_ELF(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.bin")
	elfanalyzertesting.CreateDynamicELFFile(t, targetFile)

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	v.SetSyscallAnalyzer(&stubSyscallAnalyzerWithDebugInfo{})

	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	withoutDebug, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	require.NotNil(t, withoutDebug.SyscallAnalysis)
	require.Len(t, withoutDebug.SyscallAnalysis.DetectedSyscalls, 1)
	assert.Nil(t, withoutDebug.SyscallAnalysis.DetectedSyscalls[0].Occurrences)
	assert.Nil(t, withoutDebug.SyscallAnalysis.DeterminationStats)

	v.SetIncludeDebugInfo(true)
	_, _, err = v.SaveRecord(targetFile, true)
	require.NoError(t, err)

	withDebug, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	require.NotNil(t, withDebug.SyscallAnalysis)
	require.Len(t, withDebug.SyscallAnalysis.DetectedSyscalls, 1)
	require.Len(t, withDebug.SyscallAnalysis.DetectedSyscalls[0].Occurrences, 1)
	assert.Equal(t, uint64(0x1000), withDebug.SyscallAnalysis.DetectedSyscalls[0].Occurrences[0].Location)
	require.NotNil(t, withDebug.SyscallAnalysis.DeterminationStats)
	assert.Equal(t, 1, withDebug.SyscallAnalysis.DeterminationStats.ImmediateTotal)
}

// ---------------------------------------------------------------------------
// Tests for Validator with LibcCache and SyscallAnalyzer integration
// ---------------------------------------------------------------------------

// newValidatorWithLibcCache creates a test validator with stub libc cache and
// a non-ELF target file (so ELF open fails gracefully for basic tests).
func newValidatorWithStubs(t *testing.T, libcCache LibcCacheInterface) (*Validator, string) {
	t.Helper()
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.bin")
	require.NoError(t, os.WriteFile(targetFile, []byte("not an ELF"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	if libcCache != nil {
		v.SetLibcCache(libcCache)
	}
	return v, targetFile
}

// TestRecord_LibcCache_NonELFFile verifies that non-ELF files are recorded
// without error even when a LibcCache is injected.
func TestRecord_LibcCache_NonELFFile(t *testing.T) {
	stub := &stubLibcCache{
		syscalls: []common.SyscallInfo{{Number: 42, Occurrences: []common.SyscallOccurrence{{Source: "libc_symbol_import"}}}},
	}
	v, targetFile := newValidatorWithStubs(t, stub)

	_, _, err := v.SaveRecord(targetFile, false)
	require.NoError(t, err, "non-ELF file should be recorded without error")

	record, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	// Non-ELF: no SyscallAnalysis
	assert.Nil(t, record.SyscallAnalysis)
}

// TestRecord_LibcCache_Error_CausesRecordFailure verifies that a fatal libc
// cache error causes analyzeELFSyscalls to return an error.
func TestRecord_LibcCache_Error_CausesRecordFailure(t *testing.T) {
	// Use an in-memory dynamic ELF so that openELFFile succeeds and reaches the
	// libc cache path without depending on any system binary.
	tempDir := safeTempDir(t)
	elfPath := filepath.Join(tempDir, "test.elf")
	elfanalyzertesting.CreateDynamicELFFile(t, elfPath)

	stub := &stubLibcCache{err: errors.New("libc file not accessible")}
	v, err := New(&SHA256{}, tempDir)
	require.NoError(t, err)
	v.SetLibcCache(stub)

	// Inject a DynLibDeps record with a libc entry so the libc cache is called.
	record := &fileanalysis.Record{
		DynLibDeps: []fileanalysis.LibEntry{
			{SOName: "libc.so.6", Path: "/lib/x86_64-linux-gnu/libc.so.6", Hash: "sha256:aabb"},
		},
	}
	analyzeErr := v.analyzeELFSyscalls(record, elfPath)
	require.Error(t, analyzeErr, "fatal libc cache error must propagate")
	require.Contains(t, analyzeErr.Error(), "libc cache error")
}

// TestRecord_LibcCache_UnsupportedArch_SkipsAndContinues verifies that
// ErrUnsupportedArch from libc cache is skipped and the record is still saved.
func TestRecord_LibcCache_UnsupportedArch_SkipsAndContinues(t *testing.T) {
	stub := &stubLibcCache{
		err: ErrUnsupportedArch,
	}
	v, targetFile := newValidatorWithStubs(t, stub)

	_, _, err := v.SaveRecord(targetFile, false)
	// non-ELF file → ELF open exits early, cache not called → no error
	require.NoError(t, err)
}

// TestRecord_Force_ELFToNonELF_ClearsSyscallAnalysis verifies that force re-recording
// a file that was previously recorded as ELF (with SyscallAnalysis set) and is now
// treated as non-ELF clears SyscallAnalysis (schema contract: nil for non-ELF).
func TestRecord_Force_ELFToNonELF_ClearsSyscallAnalysis(t *testing.T) {
	// Use an in-memory dynamic ELF for the first record so SyscallAnalysis gets
	// populated, then replace the file with non-ELF bytes and force re-record.
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// Create an in-memory ELF at the target path.
	targetFile := filepath.Join(tempDir, "target.bin")
	elfanalyzertesting.CreateDynamicELFFile(t, targetFile)

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	// Inject a stub syscall analyzer that returns one syscall for any ELF.
	v.SetSyscallAnalyzer(&stubSyscallAnalyzerReturnsOne{})

	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	// Verify first record has SyscallAnalysis.
	record, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	require.NotNil(t, record.SyscallAnalysis, "precondition: SyscallAnalysis must be set after ELF record")

	// Overwrite target with non-ELF bytes and force re-record.
	require.NoError(t, os.WriteFile(targetFile, []byte("not an ELF"), 0o644))
	_, _, err = v.SaveRecord(targetFile, true)
	require.NoError(t, err)

	record, loadErr = v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	assert.Nil(t, record.SyscallAnalysis,
		"SyscallAnalysis must be nil after re-recording a non-ELF file")
}

// stubSyscallAnalyzerReturnsOne is a SyscallAnalyzerInterface that returns a single
// fake syscall for any ELF file, used to seed SyscallAnalysis in test records.
type stubSyscallAnalyzerReturnsOne struct{}

func (s *stubSyscallAnalyzerReturnsOne) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
	return []common.SyscallInfo{{Number: 1, Name: "write"}}, nil, nil, nil
}

func (s *stubSyscallAnalyzerReturnsOne) EvaluatePLTCallArgs(_ *elf.File, _ string) (*common.SyscallArgEvalResult, error) {
	return nil, nil
}

func (s *stubSyscallAnalyzerReturnsOne) GetSyscallTable(_ elf.Machine) (SyscallNumberTable, bool) {
	return nil, false
}

// TestRecord_Force_SyscallsToNone_ClearsSyscallAnalysis verifies that force re-recording
// an ELF that previously had syscalls detected now clears SyscallAnalysis when the
// analyzer returns zero results (schema contract: nil when no syscalls detected).
func TestRecord_Force_SyscallsToNone_ClearsSyscallAnalysis(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// Create an in-memory ELF at the target path.
	targetFile := filepath.Join(tempDir, "target.bin")
	elfanalyzertesting.CreateDynamicELFFile(t, targetFile)

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	// First record: analyzer returns one syscall.
	v.SetSyscallAnalyzer(&stubSyscallAnalyzerReturnsOne{})
	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	record, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	require.NotNil(t, record.SyscallAnalysis, "precondition: SyscallAnalysis must be set")

	// Second record (force=true): analyzer returns no syscalls.
	v.SetSyscallAnalyzer(&stubSyscallAnalyzerReturnsNone{})
	_, _, err = v.SaveRecord(targetFile, true)
	require.NoError(t, err)

	record, loadErr = v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	assert.Nil(t, record.SyscallAnalysis,
		"SyscallAnalysis must be nil when re-recording with zero detected syscalls")
}

// stubSyscallAnalyzerReturnsNone is a SyscallAnalyzerInterface that returns no syscalls.
type stubSyscallAnalyzerReturnsNone struct{}

func (s *stubSyscallAnalyzerReturnsNone) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
	return nil, nil, nil, nil
}

func (s *stubSyscallAnalyzerReturnsNone) EvaluatePLTCallArgs(_ *elf.File, _ string) (*common.SyscallArgEvalResult, error) {
	return nil, nil
}

func (s *stubSyscallAnalyzerReturnsNone) GetSyscallTable(_ elf.Machine) (SyscallNumberTable, bool) {
	return nil, false
}

type stubSyscallAnalyzerWithDebugInfo struct{}

func (s *stubSyscallAnalyzerWithDebugInfo) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
	return []common.SyscallInfo{{
		Number: 1,
		Name:   "write",
		Occurrences: []common.SyscallOccurrence{{
			Location:            0x1000,
			DeterminationMethod: "immediate",
			Source:              "immediate",
		}},
	}}, nil, &common.SyscallDeterminationStats{ImmediateTotal: 1}, nil
}

func (s *stubSyscallAnalyzerWithDebugInfo) EvaluatePLTCallArgs(_ *elf.File, _ string) (*common.SyscallArgEvalResult, error) {
	return nil, nil
}

func (s *stubSyscallAnalyzerWithDebugInfo) GetSyscallTable(_ elf.Machine) (SyscallNumberTable, bool) {
	return nil, false
}

// TestRecord_Force_NetworkToNotSupportedBinary_ClearsSymbolAnalysis verifies the same
// nil-transition for NotSupportedBinary (non-ELF / non-Mach-O binaries).
func TestRecord_Force_NetworkToNotSupportedBinary_ClearsSymbolAnalysis(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.bin")
	require.NoError(t, os.WriteFile(targetFile, []byte("binary content"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	// First record: dynamic ELF with network symbols.
	v.SetBinaryAnalyzer(&stubBinaryAnalyzer{
		result: binaryanalyzer.NetworkDetected,
		detectedSymbols: []binaryanalyzer.DetectedSymbol{
			{Name: "socket", Category: "network"},
		},
	})
	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	// Second record (force=true): now treated as unsupported format.
	v.SetBinaryAnalyzer(&stubBinaryAnalyzer{
		result: binaryanalyzer.NotSupportedBinary,
	})
	_, _, err = v.SaveRecord(targetFile, true)
	require.NoError(t, err)

	record, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	assert.Nil(t, record.SymbolAnalysis,
		"SymbolAnalysis must be nil after re-recording as NotSupportedBinary")
}

// ---------------------------------------------------------------------------
// Tests for KnownNetworkLibDeps (SOName-based network detection)
// ---------------------------------------------------------------------------

// recordWithDynLibDepsAndBinaryAnalyzer is a test helper that creates a record with
// pre-populated DynLibDeps, then re-records with force=true using the given binaryAnalyzer stub.
// Since dynlibAnalyzer is not set, the re-record preserves the DynLibDeps from the first record.
func recordWithDynLibDepsAndBinaryAnalyzer(
	t *testing.T,
	dynLibDeps []fileanalysis.LibEntry,
	stub *stubBinaryAnalyzer,
) (*fileanalysis.Record, error) {
	t.Helper()
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.bin")
	require.NoError(t, os.WriteFile(targetFile, []byte("binary content"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	// First record: create the record (no analyzers).
	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	// Manually inject DynLibDeps into the stored record via store.Update.
	resolvedPath, pathErr := common.NewResolvedPath(targetFile)
	require.NoError(t, pathErr)
	err = v.store.Update(resolvedPath, func(record *fileanalysis.Record) error {
		record.DynLibDeps = dynLibDeps
		return nil
	})
	require.NoError(t, err)

	// Re-record with force=true and binaryAnalyzer set (dynlibAnalyzer is nil,
	// so stored DynLibDeps is preserved from the previous record).
	v.SetBinaryAnalyzer(stub)
	_, _, recErr := v.SaveRecord(targetFile, true)
	if recErr != nil {
		return nil, recErr
	}
	record, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	return record, nil
}

func TestRecord_KnownNetworkLibDeps_CurlDetected(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "libcurl.so.4", Path: "/usr/lib/libcurl.so.4", Hash: "sha256:aaa"},
		{SOName: "libz.so.1", Path: "/usr/lib/libz.so.1", Hash: "sha256:bbb"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis, "SymbolAnalysis should be set")
	assert.Equal(t, []string{"libcurl.so.4"}, record.SymbolAnalysis.KnownNetworkLibDeps)
}

func TestRecord_KnownNetworkLibDeps_PythonVersioned(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "libpython3.11.so.1.0", Path: "/usr/lib/libpython3.11.so.1.0", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis, "SymbolAnalysis should be set")
	assert.Equal(t, []string{"libpython3.11.so.1.0"}, record.SymbolAnalysis.KnownNetworkLibDeps)
}

func TestRecord_KnownNetworkLibDeps_NonNetworkOnly(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "libz.so.1", Path: "/usr/lib/libz.so.1", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis, "SymbolAnalysis should be set")
	assert.Empty(t, record.SymbolAnalysis.KnownNetworkLibDeps,
		"KnownNetworkLibDeps should be empty when no known network libs are in DynLibDeps")
}

func TestRecord_KnownNetworkLibDeps_StaleValueCleared(t *testing.T) {
	dynLibDepsWithCurl := []fileanalysis.LibEntry{
		{SOName: "libcurl.so.4", Path: "/usr/lib/libcurl.so.4", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}

	// Build a validator with a record that has KnownNetworkLibDeps set from a previous run.
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	targetFile := filepath.Join(tempDir, "target.bin")
	require.NoError(t, os.WriteFile(targetFile, []byte("binary content"), 0o644))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	_, _, err = v.SaveRecord(targetFile, false)
	require.NoError(t, err)

	resolvedPath, pathErr := common.NewResolvedPath(targetFile)
	require.NoError(t, pathErr)

	// Inject KnownNetworkLibDeps as if from a previous run.
	err = v.store.Update(resolvedPath, func(r *fileanalysis.Record) error {
		r.DynLibDeps = dynLibDepsWithCurl
		r.SymbolAnalysis = &fileanalysis.SymbolAnalysisData{
			KnownNetworkLibDeps: []string{"libcurl.so.4"},
		}
		return nil
	})
	require.NoError(t, err)

	// Replace DynLibDeps with a non-network lib and re-record; KnownNetworkLibDeps must be cleared.
	dynLibDepsNoNetwork := []fileanalysis.LibEntry{
		{SOName: "libz.so.1", Path: "/usr/lib/libz.so.1", Hash: "sha256:bbb"},
	}
	err = v.store.Update(resolvedPath, func(r *fileanalysis.Record) error {
		r.DynLibDeps = dynLibDepsNoNetwork
		return nil
	})
	require.NoError(t, err)

	v.SetBinaryAnalyzer(stub)
	_, _, err = v.SaveRecord(targetFile, true)
	require.NoError(t, err)

	updated, loadErr := v.LoadRecord(targetFile)
	require.NoError(t, loadErr)
	require.NotNil(t, updated.SymbolAnalysis)
	assert.Empty(t, updated.SymbolAnalysis.KnownNetworkLibDeps,
		"KnownNetworkLibDeps must be cleared when DynLibDeps no longer contains known network libs")
}

func TestRecord_KnownNetworkLibDeps_SymbolAnalysisNil(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "libcurl.so.4", Path: "/usr/lib/libcurl.so.4", Hash: "sha256:aaa"},
	}
	// StaticBinary → SymbolAnalysis is nil
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.StaticBinary}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	assert.Nil(t, record.SymbolAnalysis,
		"SymbolAnalysis should be nil for static binary, KnownNetworkLibDeps not recorded")
}

// ---------------------------------------------------------------------------
// Mach-O install name tests for KnownNetworkLibDeps
// ---------------------------------------------------------------------------

func TestRecord_KnownNetworkLibDeps_MachoInstallNameRuby(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/local/opt/ruby/lib/libruby.3.2.dylib", Path: "/usr/local/opt/ruby/lib/libruby.3.2.dylib", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Equal(t, []string{"/usr/local/opt/ruby/lib/libruby.3.2.dylib"}, record.SymbolAnalysis.KnownNetworkLibDeps)
}

func TestRecord_KnownNetworkLibDeps_MachoInstallNameCurl(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/local/lib/libcurl.4.dylib", Path: "/usr/local/lib/libcurl.4.dylib", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Equal(t, []string{"/usr/local/lib/libcurl.4.dylib"}, record.SymbolAnalysis.KnownNetworkLibDeps)
}

func TestRecord_KnownNetworkLibDeps_MachoInstallNamePython(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/local/opt/python/lib/libpython3.11.dylib", Path: "/usr/local/opt/python/lib/libpython3.11.dylib", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Equal(t, []string{"/usr/local/opt/python/lib/libpython3.11.dylib"}, record.SymbolAnalysis.KnownNetworkLibDeps)
}

func TestRecord_KnownNetworkLibDeps_MachoRpathInstallName(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "@rpath/libcurl.dylib", Path: "@rpath/libcurl.dylib", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Equal(t, []string{"@rpath/libcurl.dylib"}, record.SymbolAnalysis.KnownNetworkLibDeps)
}

func TestRecord_KnownNetworkLibDeps_MachoNonNetworkLib(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libz.1.dylib", Path: "/usr/lib/libz.1.dylib", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Empty(t, record.SymbolAnalysis.KnownNetworkLibDeps)
}

func TestRecord_KnownNetworkLibDeps_MachoFalsePositivePrefix(t *testing.T) {
	dynLibDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/local/lib/libpythonista.dylib", Path: "/usr/local/lib/libpythonista.dylib", Hash: "sha256:aaa"},
	}
	stub := &stubBinaryAnalyzer{result: binaryanalyzer.NoNetworkSymbols}
	record, err := recordWithDynLibDepsAndBinaryAnalyzer(t, dynLibDeps, stub)
	require.NoError(t, err)
	require.NotNil(t, record.SymbolAnalysis)
	assert.Empty(t, record.SymbolAnalysis.KnownNetworkLibDeps)
}

// stubPLTAnalyzer is a SyscallAnalyzerInterface that returns a fixed result for EvaluatePLTCallArgs.
type stubPLTAnalyzer struct {
	result *common.SyscallArgEvalResult
	err    error
}

func (s *stubPLTAnalyzer) AnalyzeSyscallsFromELF(_ *elf.File) ([]common.SyscallInfo, []common.SyscallArgEvalResult, *common.SyscallDeterminationStats, error) {
	return nil, nil, nil, nil
}

func (s *stubPLTAnalyzer) EvaluatePLTCallArgs(_ *elf.File, _ string) (*common.SyscallArgEvalResult, error) {
	return s.result, s.err
}

func (s *stubPLTAnalyzer) GetSyscallTable(_ elf.Machine) (SyscallNumberTable, bool) {
	return nil, false
}

func TestBuildArgEvalResults(t *testing.T) {
	libcWithMprotect := []common.SyscallInfo{{Name: "mprotect"}}
	libcWithoutMprotect := []common.SyscallInfo{{Name: "write"}}

	ptrResult := func(name string, status common.SyscallArgEvalStatus) *common.SyscallArgEvalResult {
		r := common.SyscallArgEvalResult{SyscallName: name, Status: status}
		return &r
	}

	tests := []struct {
		name          string
		libcSyscalls  []common.SyscallInfo
		directResults []common.SyscallArgEvalResult
		pltResult     *common.SyscallArgEvalResult
		wantMprotect  common.SyscallArgEvalStatus
		wantCount     int
	}{
		{
			name:          "no_libc_mprotect_returns_direct_unchanged",
			libcSyscalls:  libcWithoutMprotect,
			directResults: []common.SyscallArgEvalResult{{SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet}},
			wantMprotect:  common.SyscallArgEvalExecNotSet,
			wantCount:     1,
		},
		{
			// direct: exec_not_set (low risk), PLT: exec_confirmed (high risk) → must use PLT
			name:          "direct_exec_not_set_plt_exec_confirmed_uses_plt",
			libcSyscalls:  libcWithMprotect,
			directResults: []common.SyscallArgEvalResult{{SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet}},
			pltResult:     ptrResult("mprotect", common.SyscallArgEvalExecConfirmed),
			wantMprotect:  common.SyscallArgEvalExecConfirmed,
			wantCount:     1,
		},
		{
			// direct: exec_confirmed (high risk), PLT: exec_not_set (low risk) → must keep direct
			name:          "direct_exec_confirmed_plt_exec_not_set_keeps_direct",
			libcSyscalls:  libcWithMprotect,
			directResults: []common.SyscallArgEvalResult{{SyscallName: "mprotect", Status: common.SyscallArgEvalExecConfirmed}},
			pltResult:     ptrResult("mprotect", common.SyscallArgEvalExecNotSet),
			wantMprotect:  common.SyscallArgEvalExecConfirmed,
			wantCount:     1,
		},
		{
			name:         "no_direct_mprotect_plt_exec_confirmed",
			libcSyscalls: libcWithMprotect,
			pltResult:    ptrResult("mprotect", common.SyscallArgEvalExecConfirmed),
			wantMprotect: common.SyscallArgEvalExecConfirmed,
			wantCount:    1,
		},
		{
			name:         "libc_mprotect_plt_returns_nil_falls_back_to_exec_unknown",
			libcSyscalls: libcWithMprotect,
			pltResult:    nil,
			wantMprotect: common.SyscallArgEvalExecUnknown,
			wantCount:    1,
		},
		{
			// Non-mprotect direct results must be preserved alongside the best mprotect result.
			name:         "non_mprotect_direct_results_preserved",
			libcSyscalls: libcWithMprotect,
			directResults: []common.SyscallArgEvalResult{
				{SyscallName: "mmap", Status: common.SyscallArgEvalExecConfirmed},
				{SyscallName: "mprotect", Status: common.SyscallArgEvalExecNotSet},
			},
			pltResult:    ptrResult("mprotect", common.SyscallArgEvalExecConfirmed),
			wantMprotect: common.SyscallArgEvalExecConfirmed,
			wantCount:    2, // mmap + mprotect
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			analyzer := &stubPLTAnalyzer{result: tc.pltResult}
			// Use a non-nil elfFile so the PLT analysis branch is reachable.
			elfFile := new(elf.File)
			results := buildArgEvalResults(tc.libcSyscalls, tc.directResults, elfFile, analyzer)

			assert.Len(t, results, tc.wantCount)

			var got *common.SyscallArgEvalResult
			for i := range results {
				if results[i].SyscallName == "mprotect" {
					got = &results[i]
					break
				}
			}
			require.NotNil(t, got, "expected a mprotect ArgEvalResult in results")
			assert.Equal(t, tc.wantMprotect, got.Status)
		})
	}
}
