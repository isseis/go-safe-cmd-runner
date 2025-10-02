//nolint:revive // common is an appropriate name for shared utilities package
package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultFileSystem_CreateTempDir(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Test creating a temporary directory
	dir, err := fs.CreateTempDir("", "test-")
	assert.NoError(t, err, "CreateTempDir failed")
	defer os.RemoveAll(dir)

	// Verify the directory exists
	exists, err := fs.FileExists(dir)
	assert.NoError(t, err, "FileExists failed")
	assert.True(t, exists, "Created directory does not exist")

	// Verify it's a directory
	isDir, err := fs.IsDir(dir)
	assert.NoError(t, err, "IsDir failed")
	assert.True(t, isDir, "Created path is not a directory")
}

func TestDefaultFileSystem_FileExists(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Test with non-existent file
	exists, err := fs.FileExists("/non/existent/path")
	assert.NoError(t, err, "FileExists failed for non-existent path")
	assert.False(t, exists, "Non-existent file reported as existing")

	// Test with existing file
	tempDir, err := fs.CreateTempDir("", "test-exists-")
	assert.NoError(t, err, "CreateTempDir failed")
	defer fs.RemoveAll(tempDir)

	exists, err = fs.FileExists(tempDir)
	assert.NoError(t, err, "FileExists failed for existing path")
	assert.True(t, exists, "Existing file reported as non-existent")
}

func TestDefaultFileSystem_Remove(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Create a temporary directory
	tempDir, err := fs.CreateTempDir("", "test-remove-")
	assert.NoError(t, err, "CreateTempDir failed")

	// Create a file in the directory
	filePath := filepath.Join(tempDir, "test.txt")
	file, err := os.Create(filePath)
	assert.NoError(t, err, "Failed to create test file")
	file.Close()

	// Remove the file
	err = fs.Remove(filePath)
	assert.NoError(t, err, "Remove failed")

	// Verify the file no longer exists
	exists, err := fs.FileExists(filePath)
	assert.NoError(t, err, "FileExists failed")
	assert.False(t, exists, "File still exists after removal")

	// Clean up
	fs.RemoveAll(tempDir)
}

func TestDefaultFileSystem_RemoveAll(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Create a temporary directory with nested structure
	tempDir, err := fs.CreateTempDir("", "test-removeall-")
	assert.NoError(t, err, "CreateTempDir failed")

	nestedDir := filepath.Join(tempDir, "nested")
	assert.NoError(t, os.MkdirAll(nestedDir, 0o755), "os.MkdirAll failed")

	// Create a file in the nested directory
	filePath := filepath.Join(nestedDir, "test.txt")
	file, err := os.Create(filePath)
	assert.NoError(t, err, "Failed to create test file")
	file.Close()

	// Remove all
	assert.NoError(t, fs.RemoveAll(tempDir), "RemoveAll failed")

	// Verify the directory no longer exists
	exists, err := fs.FileExists(tempDir)
	assert.NoError(t, err, "FileExists failed")
	assert.False(t, exists, "Directory still exists after RemoveAll")
}

func TestDefaultFileSystem_Lstat(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Create a temporary directory
	tempDir, err := fs.CreateTempDir("", "test-lstat-")
	assert.NoError(t, err, "CreateTempDir failed")
	defer fs.RemoveAll(tempDir)

	// Test Lstat on the directory
	info, err := fs.Lstat(tempDir)
	assert.NoError(t, err, "Lstat failed")
	assert.True(t, info.IsDir(), "Lstat reported directory as not a directory")

	// Test Lstat on non-existent path
	_, err = fs.Lstat("/non/existent/path")
	assert.Error(t, err, "Lstat should fail for non-existent path")
}

func TestDefaultFileSystem_IsDir(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Create a temporary directory
	tempDir, err := fs.CreateTempDir("", "test-isdir-")
	assert.NoError(t, err, "CreateTempDir failed")
	defer fs.RemoveAll(tempDir)

	// Test with directory
	isDir, err := fs.IsDir(tempDir)
	assert.NoError(t, err, "IsDir failed")
	assert.True(t, isDir, "Directory reported as not a directory")

	// Create a file
	filePath := filepath.Join(tempDir, "test.txt")
	file, err := os.Create(filePath)
	assert.NoError(t, err, "Failed to create test file")
	file.Close()

	// Test with file
	isDir, err = fs.IsDir(filePath)
	assert.NoError(t, err, "IsDir failed for file")
	assert.False(t, isDir, "File reported as directory")

	// Test with non-existent path
	_, err = fs.IsDir("/non/existent/path")
	assert.Error(t, err, "IsDir should fail for non-existent path")
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
				assert.Error(t, err, "Expected error but got none")
				assert.Empty(t, result, "Expected empty result but got %s", result)
			} else {
				assert.NoError(t, err, "Unexpected error")
				assert.Equal(t, tt.path, result.String(), "Expected %s but got %s", tt.path, result.String())
			}
		})
	}
}

func TestContainsPathTraversalSegment(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"empty", "", false},
		{"single traversal", "..", true},
		{"relative traversal", "../etc/passwd", true},
		{"nested traversal", "a/b/../c", true},
		{"no traversal", "a/b/c.txt", false},
		{"dots in filename", "archive..zip", false},
		{"dots in segment", "a..b/c", false},
		{"leading dotfile", ".hidden/file", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ContainsPathTraversalSegment(tt.path)
			if got != tt.want {
				t.Fatalf("ContainsPathTraversalSegment(%q) = %v; want %v", tt.path, got, tt.want)
			}
		})
	}
}
