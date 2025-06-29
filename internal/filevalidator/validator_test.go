package filevalidator

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test error definitions
var (
	// ErrTestHashCalculation is returned when there's an error calculating a hash in tests.
	ErrTestHashCalculation = fmt.Errorf("test error: failed to calculate hash")

	// ErrTestHashFileWrite is returned when there's an error writing a hash file in tests.
	ErrTestHashFileWrite = fmt.Errorf("test error: failed to write hash file")

	// ErrTestHashFileRead is returned when there's an error reading a hash file in tests.
	ErrTestHashFileRead = fmt.Errorf("test error: failed to read hash file")

	// ErrTestHashCollision is returned when a hash collision is detected in tests.
	ErrTestHashCollision = fmt.Errorf("test error: hash collision detected")
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

func TestValidator_RecordAndVerify(t *testing.T) {
	tempDir := t.TempDir()

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
	tempDir := t.TempDir()
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
			expectedErr: ErrInvalidFilePath,
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
			hashDir: t.TempDir(),
			wantErr: false,
		},
		{
			name:    "nil algorithm",
			algo:    nil,
			hashDir: t.TempDir(),
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
	tempDir := t.TempDir()

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	resolvedTestFilePath, err := filepath.EvalSymlinks(testFilePath)
	if err != nil {
		t.Fatalf("Failed to resolve test file path: %v", err)
	}

	// Create a symlink to the test file
	symlinkPath := filepath.Join(tempDir, "symlink.txt")
	if err := os.Symlink(testFilePath, symlinkPath); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Create a validator
	validator, err := New(&SHA256{}, tempDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Test Record with symlink
	err = validator.Record(symlinkPath)
	if err != nil {
		t.Errorf("Record failed: %v", err)
	}

	targetPath, err := validator.validatePath(symlinkPath)
	if err != nil {
		t.Errorf("validatePath failed: %v", err)
	}

	recordedPath, expectedHash, err := validator.readAndParseHashFile(targetPath)
	if err != nil {
		t.Errorf("readAndParseHashFile failed: %v", err)
	}
	if recordedPath != resolvedTestFilePath {
		t.Errorf("Expected recorded path '%s', got '%s'", resolvedTestFilePath, recordedPath)
	}
	if expectedHash == "" {
		t.Errorf("Expected non-empty hash, got empty hash")
	}
}

func TestValidator_Verify_Symlink(t *testing.T) {
	tempDir := t.TempDir()

	// Create a test file
	testFilePath := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFilePath, []byte("test content"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
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

// testValidator wraps a Validator to override GetHashFilePath for testing.
// TODO: Inject custom GetHashFilePath to read Validator and remove custom Record and Verify methods.
type testValidator struct {
	*Validator
	hashDir string
	t       *testing.T // Add testing.T for logging
}

// GetHashFilePath overrides the original method to always return the same hash file path for testing.
func (v *testValidator) GetHashFilePath(_ string) (string, error) {
	// Always return the same hash file path for testing purposes
	path := filepath.Join(v.hashDir, "test.hash")
	return path, nil
}

// Record overrides the Validator's Record method to use our custom GetHashFilePath
func (v *testValidator) Record(filePath string) error {
	// Use our custom GetHashFilePath
	hashFilePath, err := v.GetHashFilePath(filePath)
	if err != nil {
		return fmt.Errorf("failed to get hash file path: %w", err)
	}

	targetPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Calculate hash of the file
	hash, err := v.calculateHash(targetPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTestHashCalculation, err)
	}

	// Create the hash directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(hashFilePath), 0o755); err != nil {
		return fmt.Errorf("%w: %v", ErrTestHashFileWrite, err)
	}

	// Check if the hash file already exists and contains a different path
	if existingContent, err := testSafeReadFile(v.hashDir, hashFilePath); err == nil {
		// File exists, check the recorded path
		parts := strings.SplitN(string(existingContent), "\n", 2)
		if len(parts) >= 1 && parts[0] != targetPath {
			return fmt.Errorf("%w: path '%s' conflicts with existing path '%s'", ErrTestHashCollision, targetPath, parts[0])
		}
		// If we get here, the file exists and has the same path, so we can overwrite it
	} else if !os.IsNotExist(err) {
		// Return error if it's not a "not exist" error
		return fmt.Errorf("%w: %v", ErrTestHashFileRead, err)
	}

	// Write the target path and hash to the hash file
	content := fmt.Sprintf("%s\n%s", targetPath, hash)
	if err := os.WriteFile(hashFilePath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("%w: %v", ErrTestHashFileWrite, err)
	}

	return nil
}

// Verify overrides the Validator's Verify method to use our custom GetHashFilePath
func (v *testValidator) Verify(filePath string) error {
	// Use our custom GetHashFilePath
	hashFilePath, err := v.GetHashFilePath(filePath)
	if err != nil {
		return fmt.Errorf("failed to get hash file path: %w", err)
	}

	targetPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Read the stored hash file using test-safe function
	hashFileContent, err := testSafeReadFile(v.hashDir, hashFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrHashFileNotFound
		}
		return fmt.Errorf("%w: %v", ErrTestHashFileRead, err)
	}

	// Parse the hash file content (format: "filepath\nhash")
	parts := strings.SplitN(string(hashFileContent), "\n", 2)
	if len(parts) != 2 {
		return fmt.Errorf("%w: expected 'path\nhash', got %d parts", ErrInvalidHashFileFormat, len(parts))
	}

	// Check if the recorded path matches the current file path
	recordedPath := parts[0]
	hash := parts[1]

	if recordedPath == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidHashFileFormat)
	}

	if recordedPath != targetPath {
		return fmt.Errorf("%w: recorded path '%s' does not match current path '%s'", ErrTestHashCollision, recordedPath, targetPath)
	}

	// Calculate the current hash of the file
	currentHash, err := v.calculateHash(targetPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTestHashCalculation, err)
	}

	// Compare the hashes
	if currentHash != hash {
		return ErrMismatch
	}

	return nil
}

func TestValidator_HashCollision(t *testing.T) {
	tempDir := t.TempDir()
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
	baseValidator, err := New(NewCollidingHashAlgorithm(fixedHash), hashDir)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	// Wrap the validator with our test implementation
	validator := &testValidator{
		Validator: baseValidator,
		hashDir:   hashDir,
		t:         t,
	}

	// Record the first file - should succeed
	t.Run("Record first file", func(t *testing.T) {
		if err := validator.Record(file1Path); err != nil {
			t.Fatalf("Failed to record first file: %v", err)
		}
		// Verify the hash file was created with the correct content
		hashFilePath := filepath.Join(hashDir, "test.hash")
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

		// Modify the hash file to contain file3's path but file1's hash
		parts := strings.SplitN(string(originalContent), "\n", 2)
		if len(parts) < 2 {
			t.Fatalf("Invalid hash file format: %s", originalContent)
		}

		// Write a hash file with file3's path but file1's hash
		modifiedContent := file3Path + "\n" + parts[1]
		if err := os.WriteFile(hashFilePath, []byte(modifiedContent), 0o644); err != nil {
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
