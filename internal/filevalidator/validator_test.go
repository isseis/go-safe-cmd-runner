package filevalidator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	privtesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testutil"
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

	// Test Record
	t.Run("Record", func(t *testing.T) {
		_, err := validator.Record(testFilePath, false)
		require.NoError(t, err, "Record failed")

		// Verify the hash file exists
		hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(testFilePath))
		require.NoError(t, err, "GetHashFilePath failed")

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

	// Test Record with symlink
	// Symlinks are resolved before writing the hash file
	_, err = validator.Record(symlinkPath, false)
	assert.NoError(t, err, "Record failed")

	// Use store.Load() to verify the recorded path and hash
	store := validator.GetStore()
	require.NotNil(t, store, "Store should not be nil")

	record, err := store.Load(common.ResolvedPath(resolvedSymlinkPath))
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

	_, err = validator.Record(testFilePath, false)
	require.NoError(t, err, "Record failed")

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
	hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(testFilePath))
	require.NoError(t, err, "GetHashFilePath failed")

	// Create the hash directory
	require.NoError(t, os.MkdirAll(filepath.Dir(hashFilePath), 0o750), "Failed to create hash directory")

	// Create an empty (corrupted) hash file
	require.NoError(t, os.WriteFile(hashFilePath, []byte(""), 0o640), "Failed to create empty hash file")

	// With the FileAnalysisRecord format, a corrupted/empty file is treated as "no valid record"
	// and is overwritten. Record() should succeed, creating a valid record.
	hashFile, err := validator.Record(testFilePath, false)
	require.NoError(t, err, "Record should succeed by overwriting the corrupted empty file")

	// The record file should now contain valid content
	content, err := os.ReadFile(hashFile)
	require.NoError(t, err, "Failed to read hash file")
	assert.NotEmpty(t, content, "Hash file should not be empty after Record")
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

	// Record the file
	_, err = validator.Record(testFilePath, false)
	require.NoError(t, err, "Record failed")

	// Load from store and verify the FileAnalysisRecord format
	store := validator.GetStore()
	require.NotNil(t, store, "Store should not be nil")

	record, err := store.Load(common.ResolvedPath(testFilePath))
	require.NoError(t, err, "Failed to load record from store")

	// Verify the record fields
	assert.Equal(t, testFilePath, record.FilePath, "File path is empty")
	assert.True(t, strings.HasPrefix(record.ContentHash, "sha256:"),
		"ContentHash should have sha256: prefix, got: %s", record.ContentHash)
	assert.False(t, record.UpdatedAt.IsZero(), "UpdatedAt should be set")
}

// TestValidator_VerifyFromHandle tests the VerifyFromHandle method
func TestValidator_VerifyFromHandle(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Create a test file
	testFile := createTestFile(t, "test content for VerifyFromHandle")

	// Record the hash
	_, err = validator.Record(testFile, false)
	require.NoError(t, err, "Failed to record hash")

	// Open the file
	file, err := os.Open(testFile)
	require.NoError(t, err, "Failed to open test file")
	defer func() {
		err := file.Close()
		assert.NoError(t, err, "Failed to close file")
	}()

	// Test VerifyFromHandle
	err = validator.VerifyFromHandle(file, common.ResolvedPath(testFile))
	assert.NoError(t, err, "VerifyFromHandle failed")
}

// TestValidator_VerifyFromHandle_Mismatch tests hash mismatch case
func TestValidator_VerifyFromHandle_Mismatch(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Create a test file
	testFile := createTestFile(t, "test content")

	// Record the hash
	_, err = validator.Record(testFile, false)
	require.NoError(t, err, "Failed to record hash")

	// Create another file with different content
	testFile2 := createTestFile(t, "different content")

	// Open the second file
	file, err := os.Open(testFile2)
	require.NoError(t, err, "Failed to open test file")
	defer func() {
		err := file.Close()
		assert.NoError(t, err, "Failed to close file")
	}()

	// Test VerifyFromHandle - should fail with mismatch
	err = validator.VerifyFromHandle(file, common.ResolvedPath(testFile))
	assert.ErrorIs(t, err, ErrMismatch, "Expected ErrMismatch")
}

// TestValidator_VerifyWithPrivileges tests the VerifyWithPrivileges method
func TestValidator_VerifyWithPrivileges(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Create a test file
	testFile := createTestFile(t, "test content for VerifyWithPrivileges")

	// Record the hash
	_, err = validator.Record(testFile, false)
	require.NoError(t, err, "Failed to record hash")

	// Test VerifyWithPrivileges with nil privilege manager (should fail now)
	err = validator.VerifyWithPrivileges(testFile, nil)
	assert.Error(t, err, "Expected error with nil privilege manager")
	assert.ErrorIs(t, err, ErrPrivilegeManagerNotAvailable, "Expected privilege manager error")
}

// TestValidator_VerifyWithPrivileges_NoPrivilegeManager tests error handling without privilege manager
func TestValidator_VerifyWithPrivileges_NoPrivilegeManager(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Test with non-existent file should return validation error (not privilege manager error)
	err = validator.VerifyWithPrivileges("/tmp/non_existent_file", nil)
	assert.Error(t, err, "Expected error for non-existent file")
	// Should get validation error before privilege manager check
	assert.True(t, errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, ErrPrivilegeManagerNotAvailable),
		"Expected file not found or privilege manager error, got: %v", err)
}

// TestValidator_VerifyWithPrivileges_MockPrivilegeManager tests with mock privilege manager
func TestValidator_VerifyWithPrivileges_MockPrivilegeManager(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	// Create a test file and record its hash first
	testFile := createTestFile(t, "test content for VerifyWithPrivileges")
	_, err = validator.Record(testFile, false)
	require.NoError(t, err, "Failed to record hash")

	t.Run("privilege manager not supported", func(t *testing.T) {
		mockPM := privtesting.NewMockPrivilegeManager(false)
		err = validator.VerifyWithPrivileges(testFile, mockPM)
		assert.Error(t, err, "Expected error with unsupported privilege manager")
		assert.ErrorIs(t, err, ErrPrivilegedExecutionNotSupported, "Expected privileged execution not supported error")
	})

	t.Run("privilege manager supported but fails", func(t *testing.T) {
		mockPM := privtesting.NewFailingMockPrivilegeManager(true)
		// Use a file that would require permissions to simulate the scenario
		restrictedFile := "/root/restricted_file"
		err = validator.VerifyWithPrivileges(restrictedFile, mockPM)
		assert.Error(t, err, "Expected error with failing privilege manager")
		// Should get either privilege execution error or validation error
		assert.True(t, errors.Is(err, privtesting.ErrMockPrivilegeElevationFailed) ||
			errors.Is(err, os.ErrPermission) ||
			errors.Is(err, os.ErrNotExist),
			"Expected privilege execution, permission denied, or file not found error, got: %v", err)
	})

	t.Run("privilege manager supported and succeeds", func(t *testing.T) {
		mockPM := privtesting.NewMockPrivilegeManager(true)
		err = validator.VerifyWithPrivileges(testFile, mockPM)
		assert.NoError(t, err, "Expected no error with working privilege manager")
	})
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

	// Record the file - this should use mockAlgo.Sum()
	hashFilePath, err := validator.Record(testFilePath, false)
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

	// Record with MockHashAlgorithm
	mockValidator, err := New(&MockHashAlgorithm{}, tempDir)
	require.NoError(t, err, "Failed to create mock validator")
	_, err = mockValidator.Record(testFilePath, false)
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

		// Record the hash
		_, err = validator.Record(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// VerifyAndRead should succeed
		content, err := validator.VerifyAndRead(testFile)
		assert.NoError(t, err, "VerifyAndRead should succeed")
		assert.Equal(t, testContent, string(content), "Content should match")
	})

	t.Run("file content mismatch", func(t *testing.T) {
		// Create a test file and record its hash
		testFile := createTestFile(t, testContent)
		_, err = validator.Record(testFile, false)
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

// TestValidator_VerifyAndReadWithPrivileges tests the VerifyAndReadWithPrivileges method
// which performs atomic verification and reading with privilege escalation.
func TestValidator_VerifyAndReadWithPrivileges(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	testContent := "test content for VerifyAndReadWithPrivileges"

	t.Run("nil privilege manager", func(t *testing.T) {
		// Create a test file
		testFile := createTestFile(t, testContent)

		// VerifyAndReadWithPrivileges should fail with nil privilege manager
		content, err := validator.VerifyAndReadWithPrivileges(testFile, nil)
		assert.Error(t, err, "VerifyAndReadWithPrivileges should fail with nil privilege manager")
		assert.ErrorIs(t, err, ErrPrivilegeManagerNotAvailable, "Should return ErrPrivilegeManagerNotAvailable")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("privilege execution not supported", func(t *testing.T) {
		// Create a test file and record its hash
		testFile := createTestFile(t, testContent)
		_, err = validator.Record(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// Use mock privilege manager that doesn't support privileged execution
		mockPM := privtesting.NewMockPrivilegeManager(false)

		// VerifyAndReadWithPrivileges should fail
		content, err := validator.VerifyAndReadWithPrivileges(testFile, mockPM)
		assert.Error(t, err, "VerifyAndReadWithPrivileges should fail when privileges not supported")
		assert.ErrorIs(t, err, ErrPrivilegedExecutionNotSupported, "Should return ErrPrivilegedExecutionNotSupported")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("successful privileged verification and read", func(t *testing.T) {
		// Create a test file and record its hash
		testFile := createTestFile(t, testContent)
		_, err = validator.Record(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// Use mock privilege manager that supports privileged execution
		mockPM := privtesting.NewMockPrivilegeManager(true)

		// VerifyAndReadWithPrivileges should succeed
		content, err := validator.VerifyAndReadWithPrivileges(testFile, mockPM)
		assert.NoError(t, err, "VerifyAndReadWithPrivileges should succeed")
		assert.Equal(t, testContent, string(content), "Content should match")
	})

	t.Run("file content mismatch with privileges", func(t *testing.T) {
		// Create a test file and record its hash
		testFile := createTestFile(t, testContent)
		_, err = validator.Record(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// Modify the file content
		modifiedContent := "modified content with privileges"
		err = os.WriteFile(testFile, []byte(modifiedContent), 0o644)
		require.NoError(t, err, "Failed to modify test file")

		// Use mock privilege manager
		mockPM := privtesting.NewMockPrivilegeManager(true)

		// VerifyAndReadWithPrivileges should fail with mismatch error
		content, err := validator.VerifyAndReadWithPrivileges(testFile, mockPM)
		assert.Error(t, err, "VerifyAndReadWithPrivileges should fail with modified file")
		assert.ErrorIs(t, err, ErrMismatch, "Should return ErrMismatch")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("missing hash file with privileges", func(t *testing.T) {
		// Create a test file but don't record its hash
		testFile := createTestFile(t, testContent)

		// Use mock privilege manager
		mockPM := privtesting.NewMockPrivilegeManager(true)

		// VerifyAndReadWithPrivileges should fail with hash file not found
		content, err := validator.VerifyAndReadWithPrivileges(testFile, mockPM)
		assert.Error(t, err, "VerifyAndReadWithPrivileges should fail without hash file")
		assert.ErrorIs(t, err, ErrHashFileNotFound, "Should return ErrHashFileNotFound")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("privilege manager execution failure", func(t *testing.T) {
		// Use a path that would naturally require privilege escalation
		restrictedFile := "/root/restricted_file_test.txt"

		// Create hash file for this restricted path (this is artificial for testing)
		// In real scenarios, the hash would have been recorded when the file was accessible
		restrictedPath, err := common.NewResolvedPath(restrictedFile)
		require.NoError(t, err, "Failed to create resolved path")

		// Create a FileAnalysisRecord for the restricted file using the analysis store.
		err = validator.GetStore().Update(restrictedPath, func(record *fileanalysis.Record) error {
			record.ContentHash = "sha256:mock_hash_for_restricted_file"
			return nil
		})
		require.NoError(t, err, "Failed to write hash record")

		// Use failing mock privilege manager
		mockPM := privtesting.NewFailingMockPrivilegeManager(true)

		// VerifyAndReadWithPrivileges should fail with privilege execution error
		content, err := validator.VerifyAndReadWithPrivileges(restrictedFile, mockPM)
		assert.Error(t, err, "VerifyAndReadWithPrivileges should fail with failing privilege manager")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("invalid file path with privileges", func(t *testing.T) {
		// Use mock privilege manager
		mockPM := privtesting.NewMockPrivilegeManager(true)

		// Test with empty path
		content, err := validator.VerifyAndReadWithPrivileges("", mockPM)
		assert.Error(t, err, "VerifyAndReadWithPrivileges should fail with empty path")
		assert.Nil(t, content, "Content should be nil on error")
	})

	t.Run("realistic permission scenario", func(t *testing.T) {
		// Test the behavior with a path that would typically require elevated permissions
		// This test validates the error handling when privilege escalation would be needed
		restrictedFile := "/root/some_restricted_file.txt"

		// Create hash record for this restricted path (simulating it was recorded when accessible)
		restrictedPath, err := common.NewResolvedPath(restrictedFile)
		require.NoError(t, err, "Failed to create resolved path")

		// Create a FileAnalysisRecord for the restricted file using the analysis store.
		err = validator.GetStore().Update(restrictedPath, func(record *fileanalysis.Record) error {
			record.ContentHash = "sha256:some_hash_value"
			return nil
		})
		require.NoError(t, err, "Failed to write hash record")

		// Create a mock privilege manager
		mockPM := privtesting.NewMockPrivilegeManager(true)

		// VerifyAndReadWithPrivileges should fail because the file doesn't exist
		// In a real scenario, this would either succeed with privileges or fail with permission denied
		_, err = validator.VerifyAndReadWithPrivileges(restrictedFile, mockPM)
		assert.Error(t, err, "VerifyAndReadWithPrivileges should fail for non-existent file")

		// The specific error depends on the file system, but it should be related to file access
		assert.True(t, os.IsNotExist(err) || os.IsPermission(err),
			"Error should be file not found or permission denied, got: %v", err)
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
		_, err = validator.Record(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// VerifyAndRead should return the content that matches the hash
		// Even if the file is modified between verification and reading,
		// the method should detect the mismatch because it reads the content
		// and then verifies the hash of that same content
		content, err := validator.VerifyAndRead(testFile)
		assert.NoError(t, err, "VerifyAndRead should succeed")
		assert.Equal(t, testContent, string(content), "Content should match original")
	})

	t.Run("VerifyAndReadWithPrivileges atomic operation", func(t *testing.T) {
		// Create a test file and record its hash
		testFile := createTestFile(t, testContent)
		_, err = validator.Record(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// Use mock privilege manager
		mockPM := privtesting.NewMockPrivilegeManager(true)

		// VerifyAndReadWithPrivileges should return the content that matches the hash
		content, err := validator.VerifyAndReadWithPrivileges(testFile, mockPM)
		assert.NoError(t, err, "VerifyAndReadWithPrivileges should succeed")
		assert.Equal(t, testContent, string(content), "Content should match original")
	})

	t.Run("verify read consistency", func(t *testing.T) {
		// This test verifies that both methods read and verify the same content
		// to prevent TOCTOU attacks where the file could be modified between
		// reading and verification
		testFile := createTestFile(t, testContent)
		_, err = validator.Record(testFile, false)
		require.NoError(t, err, "Failed to record hash")

		// Test VerifyAndRead
		content1, err := validator.VerifyAndRead(testFile)
		require.NoError(t, err, "VerifyAndRead should succeed")

		// Test VerifyAndReadWithPrivileges
		mockPM := privtesting.NewMockPrivilegeManager(true)
		content2, err := validator.VerifyAndReadWithPrivileges(testFile, mockPM)
		require.NoError(t, err, "VerifyAndReadWithPrivileges should succeed")

		// Both methods should return the same content
		assert.Equal(t, content1, content2, "Both methods should return identical content")
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

	// Verify that GetStore returns a non-nil store
	store := validator.GetStore()
	require.NotNil(t, store, "GetStore should return non-nil store")

	// Create a test file
	testContent := "test content for analysis store"
	testFilePath := filepath.Join(tempDir, "test.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0o644)
	require.NoError(t, err, "Failed to create test file")

	t.Run("Record with analysis store format", func(t *testing.T) {
		// Record the hash
		_, err = validator.Record(testFilePath, false)
		require.NoError(t, err, "Failed to record hash")

		// Load record directly from store to verify format
		record, err := store.Load(common.ResolvedPath(testFilePath))
		require.NoError(t, err, "Failed to load record from store")

		// Verify the record fields
		assert.Equal(t, testFilePath, record.FilePath, "FilePath should match")
		assert.True(t, strings.HasPrefix(record.ContentHash, "sha256:"), "ContentHash should have sha256 prefix")
		assert.False(t, record.UpdatedAt.IsZero(), "UpdatedAt should be set")
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

	store := validator.GetStore()
	require.NotNil(t, store, "GetStore should return non-nil store")

	// Create a test file
	testContent := "test content for preserving fields"
	testFilePath := filepath.Join(tempDir, "test_preserve.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// First, save a record with SyscallAnalysis directly via store
	err = store.Save(common.ResolvedPath(testFilePath), &fileanalysis.Record{
		ContentHash: "sha256:old_hash",
		SyscallAnalysis: &fileanalysis.SyscallAnalysisData{
			Architecture:       "x86_64",
			HasUnknownSyscalls: true,
			HighRiskReasons:    []string{"test reason"},
		},
	})
	require.NoError(t, err, "Failed to save initial record")

	// Now use validator.Record with force to update the content hash
	_, err = validator.Record(testFilePath, true)
	require.NoError(t, err, "Failed to record hash with force")

	// Load the record and verify SyscallAnalysis is preserved
	record, err := store.Load(common.ResolvedPath(testFilePath))
	require.NoError(t, err, "Failed to load updated record")

	// Verify the content hash was updated
	assert.True(t, strings.HasPrefix(record.ContentHash, "sha256:"), "ContentHash should have sha256 prefix")
	assert.NotEqual(t, "sha256:old_hash", record.ContentHash, "ContentHash should be updated")

	// Verify the SyscallAnalysis was preserved
	require.NotNil(t, record.SyscallAnalysis, "SyscallAnalysis should be preserved")
	assert.Equal(t, "x86_64", record.SyscallAnalysis.Architecture, "Architecture should be preserved")
	assert.True(t, record.SyscallAnalysis.HasUnknownSyscalls, "HasUnknownSyscalls should be preserved")
	require.Len(t, record.SyscallAnalysis.HighRiskReasons, 1, "HighRiskReasons should be preserved")
	assert.Equal(t, "test reason", record.SyscallAnalysis.HighRiskReasons[0], "HighRiskReason content should be preserved")
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
	store := validator.GetStore()
	require.NotNil(t, store, "GetStore should return non-nil store")
}

// collidingHashFilePathGetter always maps every file path to the same
// record file, simulating a hash file path collision.
type collidingHashFilePathGetter struct {
	fixedName string
}

func (c *collidingHashFilePathGetter) GetHashFilePath(hashDir string, _ common.ResolvedPath) (string, error) {
	return filepath.Join(hashDir, c.fixedName), nil
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

	v, err := newValidator(&SHA256{}, hashDir, getter)
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

	// Record first file — should succeed
	_, err := v.Record(file1, false)
	require.NoError(t, err, "first Record should succeed")

	// Record second file (different path, same record file) — should fail with collision
	_, err = v.Record(file2, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHashFilePathCollision,
		"second Record should detect collision")

	// Even with force=true, collision should still be detected
	_, err = v.Record(file2, true)
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

	// Record first file
	_, err := v.Record(file1, false)
	require.NoError(t, err)

	// Verify second file — record belongs to file1, should detect collision
	err = v.Verify(file2)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHashFilePathCollision,
		"Verify should detect collision when record belongs to a different file")
}
