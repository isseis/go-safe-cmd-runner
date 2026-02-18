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

	targetPath, err := validatePath(symlinkPath)
	assert.NoError(t, err, "validatePath failed")

	recordedPath, expectedHash, err := validator.readAndParseHashFile(targetPath)
	assert.NoError(t, err, "readAndParseHashFile failed")
	assert.Equal(t, expectedPath, recordedPath, "Expected recorded path '%s', got '%s'", expectedPath, recordedPath)
	assert.NotEmpty(t, expectedHash, "Expected non-empty hash, got empty hash")
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

type CollidingHashFilePathGetter struct{}

// GetHashFilePath always returns the same path, so it simulates a hash collision.
func (t *CollidingHashFilePathGetter) GetHashFilePath(hashDir string, _ common.ResolvedPath) (string, error) {
	return filepath.Join(hashDir, "test.json"), nil
}

func TestValidator_HashCollision(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")

	// Create the hash directory
	require.NoError(t, os.MkdirAll(hashDir, 0o755), "Failed to create hash directory")

	// Create two different test files that will have the same hash with our test algorithm
	file1Path := filepath.Join(tempDir, "file1.txt")
	file2Path := filepath.Join(tempDir, "file2.txt")

	// Create the files with different content but same hash (due to our test algorithm)
	require.NoError(t, os.WriteFile(file1Path, []byte("test content 1"), 0o644), "Failed to create test file 1")

	require.NoError(t, os.WriteFile(file2Path, []byte("test content 2"), 0o644), "Failed to create test file 2")

	// Create a validator with a colliding hash algorithm
	// This algorithm will return the same hash for any input
	fixedHash := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	validator, err := newValidator(NewCollidingHashAlgorithm(fixedHash), hashDir, &CollidingHashFilePathGetter{})
	require.NoError(t, err, "Failed to create validator")

	// Record the first file - should succeed
	t.Run("Record first file", func(t *testing.T) {
		_, err := validator.Record(file1Path, false)
		require.NoError(t, err, "Failed to record first file")
		// Verify the hash file was created with the correct content
		hashFilePath := filepath.Join(hashDir, "test.json")
		_, err = testSafeReadFile(hashDir, hashFilePath)
		require.NoError(t, err, "Failed to read hash file")
	})

	// Verify the first file - should succeed
	t.Run("Verify first file", func(t *testing.T) {
		// The first file was recorded, so verification should succeed
		err := validator.Verify(file1Path)
		assert.NoError(t, err, "Failed to verify first file")
	})

	// Record the second file - should fail with hash collision
	t.Run("Record second file with collision", func(t *testing.T) {
		_, err := validator.Record(file2Path, false)
		assert.Error(t, err, "Expected error when recording second file with same hash")
		assert.ErrorIs(t, err, ErrHashCollision, "Expected ErrHashCollision")
	})

	// Now test the Verify function with a hash collision
	t.Run("Verify with hash collision", func(t *testing.T) {
		// Create a new test file that will have the same hash as file1
		file3Path := filepath.Join(tempDir, "file3.txt")
		require.NoError(t, os.WriteFile(file3Path, []byte("different content"), 0o644), "Failed to create test file 3")

		// Get the hash file path for file1
		hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(file1Path))
		require.NoError(t, err, "Failed to get hash file path")

		// Verify the hash file exists
		_, err = os.Lstat(hashFilePath)
		require.NoError(t, err, "Hash file should exist: %s", hashFilePath)

		// Read the original hash file content using test-safe function
		originalContent, err := testSafeReadFile(hashDir, hashFilePath)
		require.NoError(t, err, "Failed to read hash file")

		// Restore the original content after the test
		defer func() {
			if err := os.WriteFile(hashFilePath, originalContent, 0o644); err != nil {
				t.Logf("Warning: failed to restore original hash file: %v", err)
			}
		}()

		// Create a modified hash manifest with file3's path but file1's hash
		modifiedHashManifest := HashManifest{
			Version: HashManifestVersion,
			Format:  HashManifestFormat,
			File: FileInfo{
				Path: file3Path,
				Hash: HashInfo{
					Algorithm: "sha256",
					Value:     fixedHash,
				},
			},
		}

		// Write the modified hash manifest
		jsonData, err := json.MarshalIndent(modifiedHashManifest, "", "  ")
		require.NoError(t, err, "Failed to marshal manifest")
		jsonData = append(jsonData, '\n')

		require.NoError(t, os.WriteFile(hashFilePath, jsonData, 0o644), "Failed to modify hash file")

		// Now try to verify file1 - it should detect the path mismatch
		err = validator.Verify(file1Path)
		assert.Error(t, err, "Expected error when verifying with hash collision")
		assert.ErrorIs(t, err, ErrHashCollision, "Expected ErrHashCollision")
	})
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

	// Create an empty hash file
	require.NoError(t, os.WriteFile(hashFilePath, []byte(""), 0o640), "Failed to create empty hash file")

	// Test Record with empty hash file - this should return ErrInvalidManifestFormat
	_, err = validator.Record(testFilePath, false)
	assert.Error(t, err, "Expected error with empty hash file")
	assert.ErrorIs(t, err, ErrInvalidManifestFormat, "Expected ErrInvalidManifestFormat")
}

// TestValidator_ManifestFormat tests that hash files are created in manifest format
func TestValidator_ManifestFormat(t *testing.T) {
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

	// Get the hash file path
	hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(testFilePath))
	require.NoError(t, err, "GetHashFilePath failed")

	// Read the hash file content
	content, err := os.ReadFile(hashFilePath)
	require.NoError(t, err, "Failed to read hash file")

	// Parse and validate the manifest content
	var manifest HashManifest
	require.NoError(t, json.Unmarshal(content, &manifest), "Failed to parse manifest")

	// Verify the manifest structure
	assert.Equal(t, HashManifestVersion, manifest.Version, "Expected version %s, got %s", HashManifestVersion, manifest.Version)
	assert.Equal(t, HashManifestFormat, manifest.Format, "Expected format %s, got %s", HashManifestFormat, manifest.Format)
	assert.NotEmpty(t, manifest.File.Path, "File path is empty")
	assert.Equal(t, "sha256", manifest.File.Hash.Algorithm, "Expected algorithm sha256, got %s", manifest.File.Hash.Algorithm)
	assert.NotEmpty(t, manifest.File.Hash.Value, "Hash value is empty")
}

func TestValidator_Record(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create test files
	testFile1Path := filepath.Join(tempDir, "test1.txt")
	testFile2Path := filepath.Join(tempDir, "test2.txt")

	require.NoError(t, os.WriteFile(testFile1Path, []byte("content1"), 0o644), "Failed to create test file 1")
	require.NoError(t, os.WriteFile(testFile2Path, []byte("content2"), 0o644), "Failed to create test file 2")

	// Create mock hash file path getter to simulate hash collision
	mockGetter := &MockHashFilePathGetter{
		filePath: filepath.Join(tempDir, "collision.json"),
	}

	testValidator := &Validator{
		algorithm:          &SHA256{},
		hashDir:            tempDir,
		hashFilePathGetter: mockGetter,
	}

	t.Run("Record without force on new file", func(t *testing.T) {
		hashFile, err := testValidator.Record(testFile1Path, false)
		require.NoError(t, err, "Failed to record hash without force")

		// Verify the hash file was created
		_, err = os.Stat(hashFile)
		assert.NoError(t, err, "Hash file should exist: %s", hashFile)
	})

	t.Run("Record without force on existing file with different path should fail", func(t *testing.T) {
		// Try to record the second file which will have the same hash file path
		_, err := testValidator.Record(testFile2Path, false)
		assert.Error(t, err, "Expected error for hash collision")
		assert.ErrorIs(t, err, ErrHashCollision, "Expected ErrHashCollision")
	})

	t.Run("Record with force on existing file with different path should still fail", func(t *testing.T) {
		// Try to record the second file with force=true - should still fail due to hash collision
		_, err := testValidator.Record(testFile2Path, true)
		assert.Error(t, err, "Expected error for hash collision even with force=true")
		assert.ErrorIs(t, err, ErrHashCollision, "Expected ErrHashCollision")
	})

	t.Run("Record with force on same file should succeed", func(t *testing.T) {
		// Try to record the same file again with force=true - should succeed
		hashFile, err := testValidator.Record(testFile1Path, true)
		require.NoError(t, err, "Failed to record hash with force for same file")

		// Verify the hash file was updated
		_, err = os.Stat(hashFile)
		assert.NoError(t, err, "Hash file should exist: %s", hashFile)

		// Verify the content still has the correct file path
		content, err := os.ReadFile(hashFile)
		require.NoError(t, err, "Failed to read hash file")

		var manifest HashManifest
		require.NoError(t, json.Unmarshal(content, &manifest), "Failed to unmarshal hash file")

		assert.Equal(t, testFile1Path, manifest.File.Path, "Expected path %s, got %s", testFile1Path, manifest.File.Path)
	})

	t.Run("Record without force on same file should fail", func(t *testing.T) {
		// Try to record the same file again without force - should fail because file exists
		_, err := testValidator.Record(testFile1Path, false)
		assert.Error(t, err, "Expected error when recording same file without force")

		// The error should be file exists error, not hash collision
		assert.NotErrorIs(t, err, ErrHashCollision, "Got ErrHashCollision for same file, expected file exists error")
	})
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

// MockHashFilePathGetter is a mock implementation for testing hash collisions
type MockHashFilePathGetter struct {
	filePath string
}

func (m *MockHashFilePathGetter) GetHashFilePath(_ string, _ common.ResolvedPath) (string, error) {
	return m.filePath, nil
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

	// Additional verification: Check that the hash file contains the expected algorithm name and hash
	hashFileContent, err := testSafeReadFile(tempDir, hashFilePath)
	require.NoError(t, err, "Failed to read hash file")

	var manifest map[string]any
	err = json.Unmarshal(hashFileContent, &manifest)
	require.NoError(t, err, "Failed to unmarshal hash file")

	// Verify the file structure: manifest.file.hash.algorithm and manifest.file.hash.value
	require.Contains(t, manifest, "file", "Hash file should contain 'file' section")
	fileSection, ok := manifest["file"].(map[string]any)
	require.True(t, ok, "File section should be a map")

	require.Contains(t, fileSection, "hash", "File section should contain 'hash'")
	hashSection, ok := fileSection["hash"].(map[string]any)
	require.True(t, ok, "Hash section should be a map")

	// Verify the algorithm name is correctly stored
	require.Contains(t, hashSection, "algorithm", "Hash section should contain 'algorithm'")
	assert.Equal(t, "mock", hashSection["algorithm"], "Hash file should contain the correct algorithm name")

	// Verify the hash value is what MockHashAlgorithm would produce
	expectedHash, err := mockAlgo.Sum(strings.NewReader(testContent))
	require.NoError(t, err, "Failed to calculate expected hash")

	require.Contains(t, hashSection, "value", "Hash section should contain 'value'")
	assert.Equal(t, expectedHash, hashSection["value"], "Hash file should contain the hash from MockHashAlgorithm")
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

	// This should fail due to algorithm mismatch
	err = sha256Validator.Verify(testFilePath)
	assert.Error(t, err, "Cross-algorithm verification should fail")
	assert.Contains(t, err.Error(), "algorithm mismatch", "Error should indicate algorithm mismatch")
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

		hashFilePath, err := validator.GetHashFilePath(restrictedPath)
		require.NoError(t, err, "Failed to get hash file path")

		// Create the hash directory
		err = os.MkdirAll(filepath.Dir(hashFilePath), 0o750)
		require.NoError(t, err, "Failed to create hash directory")

		// Create a mock hash manifest for the restricted file
		manifest := createHashManifest(restrictedPath, "mock_hash_for_restricted_file", "SHA256")
		err = validator.writeHashManifest(hashFilePath, manifest, false)
		require.NoError(t, err, "Failed to write hash manifest")

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

		// Create hash file for this restricted path (simulating it was recorded when accessible)
		restrictedPath, err := common.NewResolvedPath(restrictedFile)
		require.NoError(t, err, "Failed to create resolved path")

		hashFilePath, err := validator.GetHashFilePath(restrictedPath)
		require.NoError(t, err, "Failed to get hash file path")

		// Create the hash directory
		err = os.MkdirAll(filepath.Dir(hashFilePath), 0o750)
		require.NoError(t, err, "Failed to create hash directory")

		// Create a hash manifest
		manifest := createHashManifest(restrictedPath, "some_hash_value", "SHA256")
		err = validator.writeHashManifest(hashFilePath, manifest, false)
		require.NoError(t, err, "Failed to write hash manifest")

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

// TestNewWithAnalysisStore_RecordAndVerify tests that NewWithAnalysisStore creates a validator
// that uses the FileAnalysisRecord format and preserves existing fields.
func TestNewWithAnalysisStore_RecordAndVerify(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator with analysis store support
	validator, err := NewWithAnalysisStore(&SHA256{}, tempDir)
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
		record, err := store.Load(testFilePath)
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

// TestNewWithAnalysisStore_PreservesExistingFields tests that updating a record
// preserves existing fields like SyscallAnalysis.
func TestNewWithAnalysisStore_PreservesExistingFields(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator with analysis store support
	validator, err := NewWithAnalysisStore(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator with analysis store")

	store := validator.GetStore()
	require.NotNil(t, store, "GetStore should return non-nil store")

	// Create a test file
	testContent := "test content for preserving fields"
	testFilePath := filepath.Join(tempDir, "test_preserve.txt")
	err = os.WriteFile(testFilePath, []byte(testContent), 0o644)
	require.NoError(t, err, "Failed to create test file")

	// First, save a record with SyscallAnalysis directly via store
	err = store.Save(testFilePath, &fileanalysis.Record{
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
	record, err := store.Load(testFilePath)
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

// TestNewWithAnalysisStore_CreatesDirectory tests that NewWithAnalysisStore
// automatically creates the hash directory if it doesn't exist.
// This verifies the fix for the ordering bug where New() was called before
// NewStore(), causing a failure because newValidator() requires the directory
// to already exist, while NewStore() is the one that creates it.
func TestNewWithAnalysisStore_CreatesDirectory(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "nonexistent_subdir")

	// Verify directory does not exist yet
	_, err := os.Stat(hashDir)
	require.True(t, os.IsNotExist(err), "hashDir should not exist before calling NewWithAnalysisStore")

	// NewWithAnalysisStore should succeed even though hashDir doesn't exist
	validator, err := NewWithAnalysisStore(&SHA256{}, hashDir)
	require.NoError(t, err, "NewWithAnalysisStore should create the directory automatically")
	require.NotNil(t, validator)

	// Verify directory was created
	info, err := os.Stat(hashDir)
	require.NoError(t, err, "hashDir should exist after NewWithAnalysisStore")
	assert.True(t, info.IsDir(), "hashDir should be a directory")

	// Verify the store is usable
	store := validator.GetStore()
	require.NotNil(t, store, "GetStore should return non-nil store")
}
