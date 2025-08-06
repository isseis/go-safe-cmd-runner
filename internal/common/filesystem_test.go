//nolint:revive // common is an appropriate name for shared utilities package
package common

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultFileSystem_CreateTempDir(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Test creating a temporary directory
	dir, err := fs.CreateTempDir("", "test-")
	if err != nil {
		t.Fatalf("CreateTempDir failed: %v", err)
	}
	defer os.RemoveAll(dir)

	// Verify the directory exists
	exists, err := fs.FileExists(dir)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if !exists {
		t.Error("Created directory does not exist")
	}

	// Verify it's a directory
	isDir, err := fs.IsDir(dir)
	if err != nil {
		t.Fatalf("IsDir failed: %v", err)
	}
	if !isDir {
		t.Error("Created path is not a directory")
	}
}

func TestDefaultFileSystem_FileExists(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Test with non-existent file
	exists, err := fs.FileExists("/non/existent/path")
	if err != nil {
		t.Fatalf("FileExists failed for non-existent path: %v", err)
	}
	if exists {
		t.Error("Non-existent file reported as existing")
	}

	// Test with existing file
	tempDir, err := fs.CreateTempDir("", "test-exists-")
	if err != nil {
		t.Fatalf("CreateTempDir failed: %v", err)
	}
	defer fs.RemoveAll(tempDir)

	exists, err = fs.FileExists(tempDir)
	if err != nil {
		t.Fatalf("FileExists failed for existing path: %v", err)
	}
	if !exists {
		t.Error("Existing file reported as non-existent")
	}
}

func TestDefaultFileSystem_Remove(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Create a temporary directory
	tempDir, err := fs.CreateTempDir("", "test-remove-")
	if err != nil {
		t.Fatalf("CreateTempDir failed: %v", err)
	}

	// Create a file in the directory
	filePath := filepath.Join(tempDir, "test.txt")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	file.Close()

	// Remove the file
	err = fs.Remove(filePath)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify the file no longer exists
	exists, err := fs.FileExists(filePath)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Error("File still exists after removal")
	}

	// Clean up
	fs.RemoveAll(tempDir)
}

func TestDefaultFileSystem_RemoveAll(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Create a temporary directory with nested structure
	tempDir, err := fs.CreateTempDir("", "test-removeall-")
	if err != nil {
		t.Fatalf("CreateTempDir failed: %v", err)
	}

	nestedDir := filepath.Join(tempDir, "nested")
	err = os.MkdirAll(nestedDir, 0o755)
	if err != nil {
		t.Fatalf("os.MkdirAll failed: %v", err)
	}

	// Create a file in the nested directory
	filePath := filepath.Join(nestedDir, "test.txt")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	file.Close()

	// Remove all
	err = fs.RemoveAll(tempDir)
	if err != nil {
		t.Fatalf("RemoveAll failed: %v", err)
	}

	// Verify the directory no longer exists
	exists, err := fs.FileExists(tempDir)
	if err != nil {
		t.Fatalf("FileExists failed: %v", err)
	}
	if exists {
		t.Error("Directory still exists after RemoveAll")
	}
}

func TestDefaultFileSystem_Lstat(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Create a temporary directory
	tempDir, err := fs.CreateTempDir("", "test-lstat-")
	if err != nil {
		t.Fatalf("CreateTempDir failed: %v", err)
	}
	defer fs.RemoveAll(tempDir)

	// Test Lstat on the directory
	info, err := fs.Lstat(tempDir)
	if err != nil {
		t.Fatalf("Lstat failed: %v", err)
	}

	if !info.IsDir() {
		t.Error("Lstat reported directory as not a directory")
	}

	// Test Lstat on non-existent path
	_, err = fs.Lstat("/non/existent/path")
	if err == nil {
		t.Error("Lstat should fail for non-existent path")
	}
}

func TestDefaultFileSystem_IsDir(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Create a temporary directory
	tempDir, err := fs.CreateTempDir("", "test-isdir-")
	if err != nil {
		t.Fatalf("CreateTempDir failed: %v", err)
	}
	defer fs.RemoveAll(tempDir)

	// Test with directory
	isDir, err := fs.IsDir(tempDir)
	if err != nil {
		t.Fatalf("IsDir failed: %v", err)
	}
	if !isDir {
		t.Error("Directory reported as not a directory")
	}

	// Create a file
	filePath := filepath.Join(tempDir, "test.txt")
	file, err := os.Create(filePath)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	file.Close()

	// Test with file
	isDir, err = fs.IsDir(filePath)
	if err != nil {
		t.Fatalf("IsDir failed for file: %v", err)
	}
	if isDir {
		t.Error("File reported as directory")
	}

	// Test with non-existent path
	_, err = fs.IsDir("/non/existent/path")
	if err == nil {
		t.Error("IsDir should fail for non-existent path")
	}
}

func TestNewResolvedPath(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name:        "valid path",
			path:        "/tmp/test",
			expectError: false,
		},
		{
			name:        "empty path should fail",
			path:        "",
			expectError: true,
		},
		{
			name:        "relative path",
			path:        "./test",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewResolvedPath(tt.path)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if result != "" {
					t.Errorf("Expected empty result but got %s", result)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result.String() != tt.path {
					t.Errorf("Expected %s but got %s", tt.path, result.String())
				}
			}
		})
	}
}
