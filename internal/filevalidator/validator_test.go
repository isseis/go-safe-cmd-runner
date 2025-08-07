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
	if err != nil {
		t.Fatalf("Failed to resolve symlinks in temp dir: %v", err)
	}
	return realPath
}

func TestValidator_RecordAndVerify(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Resolve any symlinks in the test file path
	testFilePath, err := filepath.EvalSymlinks(testFilePath)
	if err != nil {
		t.Fatalf("Failed to resolve symlinks in test file path: %v", err)
	}

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Test Record
	t.Run("Record", func(t *testing.T) {
		if _, err := validator.Record(testFilePath, false); err != nil {
			t.Fatalf("Record failed: %v", err)
		}

		// Verify the hash file exists
		hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(testFilePath))
		if err != nil {
			t.Fatalf("GetHashFilePath failed: %v", err)
		}

		_, err = os.Lstat(hashFilePath)
		assert.False(t, os.IsNotExist(err), "Hash file was not created")
	})

	// Test Verify with unmodified file
	t.Run("Verify unmodified", func(t *testing.T) {
		if err = validator.Verify(testFilePath); err != nil {
			t.Errorf("Verify failed with unmodified file: %v", err)
		}
	})

	// Test Verify with modified file
	t.Run("Verify modified", func(t *testing.T) {
		// Modify the file
		if err := os.WriteFile(testFilePath, []byte("modified content"), 0o644); err != nil {
			t.Fatalf("Failed to modify test file: %v", err)
		}

		err := validator.Verify(testFilePath)
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
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidator_Record_Symlink(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get the real path (resolving symlinks)
	resolvedTestFilePath, err := filepath.EvalSymlinks(testFilePath)
	if err != nil {
		t.Fatalf("Failed to resolve symlinks in test file path: %v", err)
	}

	// Create a symlink to the test file
	symlinkPath := filepath.Join(tempDir, "symlink.txt")
	if err := os.Symlink(resolvedTestFilePath, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Resolve the symlink to get the expected path
	resolvedSymlinkPath, err := filepath.EvalSymlinks(symlinkPath)
	if err != nil {
		t.Fatalf("Failed to resolve symlink: %v", err)
	}
	expectedPath := resolvedSymlinkPath

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Test Record with symlink
	// Symlinks are resolved before writing the hash file
	_, err = validator.Record(symlinkPath, false)
	if err != nil {
		t.Errorf("Record failed: %v", err)
	}

	targetPath, err := validatePath(symlinkPath)
	if err != nil {
		t.Errorf("validatePath failed: %v", err)
	}

	recordedPath, expectedHash, err := validator.readAndParseHashFile(targetPath)
	if err != nil {
		t.Errorf("readAndParseHashFile failed: %v", err)
	}
	if recordedPath != expectedPath {
		t.Errorf("Expected recorded path '%s', got '%s'", expectedPath, recordedPath)
	}
	if expectedHash == "" {
		t.Errorf("Expected non-empty hash, got empty hash")
	}
}

func TestValidator_Verify_Symlink(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Resolve any symlinks in the test file path
	testFilePath, err := filepath.EvalSymlinks(testFilePath)
	if err != nil {
		t.Fatalf("Failed to resolve symlinks in test file path: %v", err)
	}

	// Create a validator and record the original file
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	if _, err := validator.Record(testFilePath, false); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// Create a symlink to the test file
	symlinkPath := filepath.Join(tempDir, "symlink.txt")
	if err := os.Symlink(testFilePath, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Test Verify with symlink
	err = validator.Verify(symlinkPath)
	if err != nil {
		t.Errorf("Verify failed: %v", err)
	}
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
	if err := os.MkdirAll(hashDir, 0o755); err != nil {
		t.Fatalf("Failed to create hash directory: %v", err)
	}

	// Create two different test files that will have the same hash with our test algorithm
	file1Path := filepath.Join(tempDir, "file1.txt")
	file2Path := filepath.Join(tempDir, "file2.txt")

	// Create the files with different content but same hash (due to our test algorithm)
	if err := os.WriteFile(file1Path, []byte("test content 1"), 0o644); err != nil {
		t.Fatalf("Failed to create test file 1: %v", err)
	}

	if err := os.WriteFile(file2Path, []byte("test content 2"), 0o644); err != nil {
		t.Fatalf("Failed to create test file 2: %v", err)
	}

	// Create a validator with a colliding hash algorithm
	// This algorithm will return the same hash for any input
	fixedHash := "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
	validator, err := newValidator(NewCollidingHashAlgorithm(fixedHash), hashDir, &CollidingHashFilePathGetter{})
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Record the first file - should succeed
	t.Run("Record first file", func(t *testing.T) {
		if _, err := validator.Record(file1Path, false); err != nil {
			t.Fatalf("Failed to record first file: %v", err)
		}
		// Verify the hash file was created with the correct content
		hashFilePath := filepath.Join(hashDir, "test.json")
		_, err := testSafeReadFile(hashDir, hashFilePath)
		if err != nil {
			t.Fatalf("Failed to read hash file: %v", err)
		}
	})

	// Verify the first file - should succeed
	t.Run("Verify first file", func(t *testing.T) {
		// The first file was recorded, so verification should succeed
		if err := validator.Verify(file1Path); err != nil {
			t.Errorf("Failed to verify first file: %v", err)
		}
	})

	// Record the second file - should fail with hash collision
	t.Run("Record second file with collision", func(t *testing.T) {
		_, err := validator.Record(file2Path, false)
		if err == nil {
			t.Fatal("Expected error when recording second file with same hash, got nil")
		}
		if !errors.Is(err, ErrHashCollision) {
			t.Errorf("Expected ErrHashCollision, got %v", err)
		}
	})

	// Now test the Verify function with a hash collision
	t.Run("Verify with hash collision", func(t *testing.T) {
		// Create a new test file that will have the same hash as file1
		file3Path := filepath.Join(tempDir, "file3.txt")
		if err := os.WriteFile(file3Path, []byte("different content"), 0o644); err != nil {
			t.Fatalf("Failed to create test file 3: %v", err)
		}

		// Get the hash file path for file1
		hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(file1Path))
		if err != nil {
			t.Fatalf("Failed to get hash file path: %v", err)
		}

		// Verify the hash file exists
		if _, err := os.Lstat(hashFilePath); os.IsNotExist(err) {
			t.Fatalf("Hash file does not exist: %s", hashFilePath)
		}

		// Read the original hash file content using test-safe function
		originalContent, err := testSafeReadFile(hashDir, hashFilePath)
		if err != nil {
			t.Fatalf("Failed to read hash file: %v", err)
		}

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
		if err != nil {
			t.Fatalf("Failed to marshal manifest: %v", err)
		}
		jsonData = append(jsonData, '\n')

		if err := os.WriteFile(hashFilePath, jsonData, 0o644); err != nil {
			t.Fatalf("Failed to modify hash file: %v", err)
		}

		// Now try to verify file1 - it should detect the path mismatch
		err = validator.Verify(file1Path)
		if err == nil {
			t.Fatal("Expected error when verifying with hash collision, got nil")
		}
		if !errors.Is(err, ErrHashCollision) {
			t.Errorf("Expected ErrHashCollision, got %v", err)
		}
	})
}

func TestValidator_Record_EmptyHashFile(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Get the hash file path
	hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(testFilePath))
	if err != nil {
		t.Fatalf("GetHashFilePath failed: %v", err)
	}

	// Create the hash directory
	if err := os.MkdirAll(filepath.Dir(hashFilePath), 0o750); err != nil {
		t.Fatalf("Failed to create hash directory: %v", err)
	}

	// Create an empty hash file
	if err := os.WriteFile(hashFilePath, []byte(""), 0o640); err != nil {
		t.Fatalf("Failed to create empty hash file: %v", err)
	}

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
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Record the file
	_, err = validator.Record(testFilePath, false)
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// Get the hash file path
	hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(testFilePath))
	if err != nil {
		t.Fatalf("GetHashFilePath failed: %v", err)
	}

	// Read the hash file content
	content, err := os.ReadFile(hashFilePath)
	if err != nil {
		t.Fatalf("Failed to read hash file: %v", err)
	}

	// Parse and validate the manifest content
	var manifest HashManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	// Verify the manifest structure
	if manifest.Version != HashManifestVersion {
		t.Errorf("Expected version %s, got %s", HashManifestVersion, manifest.Version)
	}
	if manifest.Format != HashManifestFormat {
		t.Errorf("Expected format %s, got %s", HashManifestFormat, manifest.Format)
	}
	if manifest.File.Path == "" {
		t.Error("File path is empty")
	}
	if manifest.File.Hash.Algorithm != "sha256" {
		t.Errorf("Expected algorithm sha256, got %s", manifest.File.Hash.Algorithm)
	}
	if manifest.File.Hash.Value == "" {
		t.Error("Hash value is empty")
	}
}

func TestValidator_InvalidTimestamp(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	hashFilePath, err := validator.GetHashFilePath(common.ResolvedPath(testFilePath))
	if err != nil {
		t.Fatalf("GetHashFilePath failed: %v", err)
	}

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
		if err != nil {
			t.Fatalf("Failed to marshal manifest: %v", err)
		}
		jsonData = append(jsonData, '\n')
		if err := os.WriteFile(hashFilePath, jsonData, 0o644); err != nil {
			t.Fatalf("Failed to write hash file: %v", err)
		}
		err = validator.Verify(testFilePath)
		if !errors.Is(err, ErrInvalidTimestamp) {
			t.Errorf("Expected ErrInvalidTimestamp, got %v", err)
		}
	})
}

func TestValidator_Record(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create test files
	testFile1Path := filepath.Join(tempDir, "test1.txt")
	testFile2Path := filepath.Join(tempDir, "test2.txt")

	if err := os.WriteFile(testFile1Path, []byte("content1"), 0o644); err != nil {
		t.Fatalf("Failed to create test file 1: %v", err)
	}
	if err := os.WriteFile(testFile2Path, []byte("content2"), 0o644); err != nil {
		t.Fatalf("Failed to create test file 2: %v", err)
	}

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
		if err != nil {
			t.Fatalf("Failed to record hash without force: %v", err)
		}

		// Verify the hash file was created
		if _, err := os.Stat(hashFile); os.IsNotExist(err) {
			t.Errorf("Hash file was not created: %s", hashFile)
		}
	})

	t.Run("Record without force on existing file with different path should fail", func(t *testing.T) {
		// Try to record the second file which will have the same hash file path
		_, err := testValidator.Record(testFile2Path, false)
		if err == nil {
			t.Fatal("Expected error for hash collision, but got nil")
		}

		if !errors.Is(err, ErrHashCollision) {
			t.Errorf("Expected ErrHashCollision, got: %v", err)
		}
	})

	t.Run("Record with force on existing file with different path should still fail", func(t *testing.T) {
		// Try to record the second file with force=true - should still fail due to hash collision
		_, err := testValidator.Record(testFile2Path, true)
		if err == nil {
			t.Fatal("Expected error for hash collision even with force=true, but got nil")
		}

		if !errors.Is(err, ErrHashCollision) {
			t.Errorf("Expected ErrHashCollision, got: %v", err)
		}
	})

	t.Run("Record with force on same file should succeed", func(t *testing.T) {
		// Try to record the same file again with force=true - should succeed
		hashFile, err := testValidator.Record(testFile1Path, true)
		if err != nil {
			t.Fatalf("Failed to record hash with force for same file: %v", err)
		}

		// Verify the hash file was updated
		if _, err := os.Stat(hashFile); os.IsNotExist(err) {
			t.Errorf("Hash file was not created: %s", hashFile)
		}

		// Verify the content still has the correct file path
		content, err := os.ReadFile(hashFile)
		if err != nil {
			t.Fatalf("Failed to read hash file: %v", err)
		}

		var manifest HashManifest
		if err := json.Unmarshal(content, &manifest); err != nil {
			t.Fatalf("Failed to unmarshal hash file: %v", err)
		}

		if manifest.File.Path != testFile1Path {
			t.Errorf("Expected path %s, got %s", testFile1Path, manifest.File.Path)
		}
	})

	t.Run("Record without force on same file should fail", func(t *testing.T) {
		// Try to record the same file again without force - should fail because file exists
		_, err := testValidator.Record(testFile1Path, false)
		if err == nil {
			t.Fatal("Expected error when recording same file without force, but got nil")
		}

		// The error should be file exists error, not hash collision
		if errors.Is(err, ErrHashCollision) {
			t.Error("Got ErrHashCollision for same file, expected file exists error")
		}
	})
}

// TestValidator_VerifyFromHandle tests the VerifyFromHandle method
func TestValidator_VerifyFromHandle(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Create a test file
	testFile := createTestFile(t, "test content for VerifyFromHandle")

	// Record the hash
	_, err = validator.Record(testFile, false)
	if err != nil {
		t.Fatalf("Failed to record hash: %v", err)
	}

	// Open the file
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer file.Close()

	// Test VerifyFromHandle
	err = validator.VerifyFromHandle(file, common.ResolvedPath(testFile))
	if err != nil {
		t.Errorf("VerifyFromHandle failed: %v", err)
	}
}

// TestValidator_VerifyFromHandle_Mismatch tests hash mismatch case
func TestValidator_VerifyFromHandle_Mismatch(t *testing.T) {
	tempDir := safeTempDir(t)

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Create a test file
	testFile := createTestFile(t, "test content")

	// Record the hash
	_, err = validator.Record(testFile, false)
	if err != nil {
		t.Fatalf("Failed to record hash: %v", err)
	}

	// Create another file with different content
	testFile2 := createTestFile(t, "different content")

	// Open the second file
	file, err := os.Open(testFile2)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer file.Close()

	// Test VerifyFromHandle - should fail with mismatch
	err = validator.VerifyFromHandle(file, common.ResolvedPath(testFile))
	if !errors.Is(err, ErrMismatch) {
		t.Errorf("Expected ErrMismatch, got: %v", err)
	}
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
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Create a test file
	testFile := createTestFile(t, "test content for VerifyWithPrivileges")

	// Record the hash
	_, err = validator.Record(testFile, false)
	if err != nil {
		t.Fatalf("Failed to record hash: %v", err)
	}

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
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

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
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Create a test file and record its hash first
	testFile := createTestFile(t, "test content for VerifyWithPrivileges")
	_, err = validator.Record(testFile, false)
	if err != nil {
		t.Fatalf("Failed to record hash: %v", err)
	}

	t.Run("privilege manager not supported", func(t *testing.T) {
		mockPM := privtesting.NewMockPrivilegeManager(false)
		err = validator.VerifyWithPrivileges(testFile, mockPM)
		if err == nil {
			t.Error("Expected error with unsupported privilege manager, got nil")
		}
		if !errors.Is(err, ErrPrivilegedExecutionNotSupported) {
			t.Errorf("Expected privileged execution not supported error, got: %v", err)
		}
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
		if err != nil {
			t.Errorf("Expected no error with working privilege manager, got: %v", err)
		}
	})
}
