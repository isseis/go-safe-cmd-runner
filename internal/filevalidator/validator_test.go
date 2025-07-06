package filevalidator

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
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
		if err := validator.Record(testFilePath); err != nil {
			t.Fatalf("Record failed: %v", err)
		}

		// Verify the hash file exists
		hashFilePath, err := validator.GetHashFilePath(testFilePath)
		if err != nil {
			t.Fatalf("GetHashFilePath failed: %v", err)
		}

		if _, err := os.Stat(hashFilePath); os.IsNotExist(err) {
			t.Error("Hash file was not created")
		}
	})

	// Test Verify with unmodified file
	t.Run("Verify unmodified", func(t *testing.T) {
		if err := validator.Verify(testFilePath); err != nil {
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
		if err == nil {
			t.Error("Expected error with modified file, got nil")
		} else if !errors.Is(err, ErrMismatch) {
			t.Errorf("Expected ErrMismatch, got %v", err)
		}
	})

	// Test Verify with non-existent file
	t.Run("Verify non-existent", func(t *testing.T) {
		err := validator.Verify(filepath.Join(tempDir, "nonexistent.txt"))
		if err == nil {
			t.Error("Expected an error for non-existent file, got nil")
		}
	})
}

func TestValidator_GetHashFilePath(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Create test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	tests := []struct {
		name        string
		filePath    string
		expectedErr error
	}{
		{
			name:        "valid path",
			filePath:    testFilePath,
			expectedErr: nil,
		},
		{
			name:        "empty path",
			filePath:    "",
			expectedErr: safefileio.ErrInvalidFilePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validator.GetHashFilePath(tt.filePath)
			if (err != nil) != (tt.expectedErr != nil) || (err != nil && !errors.Is(err, tt.expectedErr)) {
				t.Errorf("GetHashFilePath() error = %v, want %v", err, tt.expectedErr)
			}
		})
	}
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
	err = validator.Record(symlinkPath)
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

	if err := validator.Record(testFilePath); err != nil {
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
func (t *CollidingHashFilePathGetter) GetHashFilePath(_ HashAlgorithm, hashDir string, _ string) (string, error) {
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
		if err := validator.Record(file1Path); err != nil {
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
		err := validator.Record(file2Path)
		if err == nil {
			t.Fatal("Expected error when recording second file with same hash, got nil")
		}
		expectedErr := "hash collision detected"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("Expected error to contain '%s', got: %v", expectedErr, err)
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
		hashFilePath, err := validator.GetHashFilePath(file1Path)
		if err != nil {
			t.Fatalf("Failed to get hash file path: %v", err)
		}

		// Verify the hash file exists
		if _, err := os.Stat(hashFilePath); os.IsNotExist(err) {
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

		// Create a modified JSON format hash file with file3's path but file1's hash
		modifiedHashManifest := HashManifest{
			Version:   "1.0",
			Format:    "file-hash",
			Timestamp: time.Now().UTC(),
			File: FileInfo{
				Path: file3Path,
				Hash: HashInfo{
					Algorithm: "sha256",
					Value:     fixedHash,
				},
			},
		}

		// Write the modified JSON format hash file
		jsonData, err := json.MarshalIndent(modifiedHashManifest, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
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
		expectedErr := "hash collision detected"
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("Expected error to contain '%s', got: %v", expectedErr, err)
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
	hashFilePath, err := validator.GetHashFilePath(testFilePath)
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

	// Test Record with empty hash file - this should return ErrInvalidJSONFormat
	err = validator.Record(testFilePath)
	if err == nil {
		t.Error("Expected error with empty hash file, got nil")
	} else if !errors.Is(err, ErrInvalidJSONFormat) {
		t.Errorf("Expected ErrInvalidJSONFormat, got %v", err)
	}
}

// TestValidator_JSONFormat tests that hash files are created in JSON format
func TestValidator_JSONFormat(t *testing.T) {
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
	if err := validator.Record(testFilePath); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	// Get the hash file path
	hashFilePath, err := validator.GetHashFilePath(testFilePath)
	if err != nil {
		t.Fatalf("GetHashFilePath failed: %v", err)
	}

	// Read the hash file content
	content, err := os.ReadFile(hashFilePath)
	if err != nil {
		t.Fatalf("Failed to read hash file: %v", err)
	}

	// Parse and validate the JSON content
	var format HashManifest
	if err := json.Unmarshal(content, &format); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Verify the JSON structure
	if format.Version != "1.0" {
		t.Errorf("Expected version 1.0, got %s", format.Version)
	}
	if format.Format != "file-hash" {
		t.Errorf("Expected format 'file-hash', got %s", format.Format)
	}
	if format.File.Path == "" {
		t.Error("File path is empty")
	}
	if format.File.Hash.Algorithm != "sha256" {
		t.Errorf("Expected algorithm sha256, got %s", format.File.Hash.Algorithm)
	}
	if format.File.Hash.Value == "" {
		t.Error("Hash value is empty")
	}
}

// TestValidator_LegacyFormatError tests that legacy format files are rejected
func TestValidator_LegacyFormatError(t *testing.T) {
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
	hashFilePath, err := validator.GetHashFilePath(testFilePath)
	if err != nil {
		t.Fatalf("GetHashFilePath failed: %v", err)
	}

	// Create the hash directory
	if err := os.MkdirAll(filepath.Dir(hashFilePath), 0o750); err != nil {
		t.Fatalf("Failed to create hash directory: %v", err)
	}

	// Create a legacy format hash file
	legacyContent := testFilePath + "\nabc123def456..."
	if err := os.WriteFile(hashFilePath, []byte(legacyContent), 0o644); err != nil {
		t.Fatalf("Failed to create legacy hash file: %v", err)
	}

	// Test Verify with legacy format (should fail)
	err = validator.Verify(testFilePath)
	if err == nil {
		t.Error("Expected error with legacy format, got nil")
	} else if !errors.Is(err, ErrInvalidJSONFormat) {
		t.Errorf("Expected ErrInvalidJSONFormat, got %v", err)
	}

	// Test Record with existing legacy format (should fail)
	err = validator.Record(testFilePath)
	if err == nil {
		t.Error("Expected error with existing legacy format, got nil")
	} else if !errors.Is(err, ErrInvalidJSONFormat) {
		t.Errorf("Expected ErrInvalidJSONFormat, got %v", err)
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

	hashFilePath, err := validator.GetHashFilePath(testFilePath)
	if err != nil {
		t.Fatalf("GetHashFilePath failed: %v", err)
	}

	t.Run("Zero timestamp", func(t *testing.T) {
		format := HashManifest{
			Version:   "1.0",
			Format:    "file-hash",
			Timestamp: time.Time{}, // zero value
			File: FileInfo{
				Path: testFilePath,
				Hash: HashInfo{
					Algorithm: "sha256",
					Value:     "dummyhash",
				},
			},
		}
		jsonData, err := json.MarshalIndent(format, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
		}
		jsonData = append(jsonData, '\n')
		if err := os.WriteFile(hashFilePath, jsonData, 0o644); err != nil {
			t.Fatalf("Failed to write hash file: %v", err)
		}
		err = validator.Verify(testFilePath)
		if err == nil || !strings.Contains(err.Error(), "invalid timestamp") {
			t.Errorf("Expected invalid timestamp error, got: %v", err)
		}
	})
}
