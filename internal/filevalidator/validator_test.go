package filevalidator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	privtesting "github.com/isseis/go-safe-cmd-runner/internal/runner/privilege/testing"
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
func (t *CollidingHashFilePathGetter) GetHashFilePath(_ HashAlgorithm, hashDir string, _ common.ResolvedPath) (string, error) {
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
			Version:   HashManifestVersion,
			Format:    HashManifestFormat,
			Timestamp: time.Now().UTC(),
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

func TestValidator_InvalidTimestamp(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	require.NoError(t, os.WriteFile(testFilePath, []byte("test content"), 0o644), "Failed to create test file")

	validator, err := New(&SHA256{}, tempDir)
	require.NoError(t, err, "Failed to create validator")

	hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(testFilePath))
	require.NoError(t, err, "GetHashFilePath failed")

	t.Run("Zero timestamp", func(t *testing.T) {
		manifest := HashManifest{
			Version:   HashManifestVersion,
			Format:    HashManifestFormat,
			Timestamp: time.Time{}, // zero value
			File: FileInfo{
				Path: testFilePath,
				Hash: HashInfo{
					Algorithm: "sha256",
					Value:     "dummyhash",
				},
			},
		}
		jsonData, err := json.MarshalIndent(manifest, "", "  ")
		require.NoError(t, err, "Failed to marshal manifest")
		jsonData = append(jsonData, '\n')
		require.NoError(t, os.WriteFile(hashFilePath, jsonData, 0o644), "Failed to write hash file")
		err = validator.Verify(testFilePath)
		assert.ErrorIs(t, err, ErrInvalidTimestamp, "Expected ErrInvalidTimestamp")
	})
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
	defer file.Close()

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
	defer file.Close()

	// Test VerifyFromHandle - should fail with mismatch
	err = validator.VerifyFromHandle(file, common.ResolvedPath(testFile))
	assert.ErrorIs(t, err, ErrMismatch, "Expected ErrMismatch")
}

// MockHashFilePathGetter is a mock implementation for testing hash collisions
type MockHashFilePathGetter struct {
	filePath string
}

func (m *MockHashFilePathGetter) GetHashFilePath(_ HashAlgorithm, _ string, _ common.ResolvedPath) (string, error) {
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
