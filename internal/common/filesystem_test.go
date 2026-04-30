//go:build test

//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultFileSystem_TempDir(t *testing.T) {
	fs := NewDefaultFileSystem()

	// Test getting the temporary directory
	tempDir := fs.TempDir()
	assert.NotEmpty(t, tempDir, "TempDir should return a non-empty path")

	// Resolve symlinks (e.g., /tmp -> /private/tmp on macOS)
	resolvedTempDir, err := filepath.EvalSymlinks(tempDir)
	if err == nil {
		tempDir = resolvedTempDir
	}

	// Verify the temporary directory exists
	exists, err := fs.FileExists(tempDir)
	assert.NoError(t, err, "FileExists failed for temp directory")
	assert.True(t, exists, "Temporary directory does not exist")

	// Verify it's a directory (IsDir uses Lstat, so path must be resolved)
	isDir, err := fs.IsDir(tempDir)
	assert.NoError(t, err, "IsDir failed for temp directory")
	assert.True(t, isDir, "Temporary directory path is not a directory")
}

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
	// Create a real temp dir to test with existing paths
	tmpDir := t.TempDir()

	// Create a real file inside tmpDir
	realFile := filepath.Join(tmpDir, "testfile.txt")
	if err := os.WriteFile(realFile, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Resolve tmpDir itself (handles macOS /tmp -> /private/tmp symlinks)
	resolvedDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	resolvedFile := filepath.Join(resolvedDir, "testfile.txt")

	tests := []struct {
		name        string
		path        string
		expectError bool
		expectPath  string
	}{
		{
			name:        "existing file returns resolved absolute path",
			path:        realFile,
			expectError: false,
			expectPath:  resolvedFile,
		},
		{
			name:        "empty path should fail",
			path:        "",
			expectError: true,
		},
		{
			name:        "non-existent path should fail",
			path:        filepath.Join(tmpDir, "does_not_exist.txt"),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewResolvedPath(tt.path)

			if tt.expectError {
				assert.Error(t, err, "Expected error but got none")
				assert.Empty(t, result.String(), "Expected empty result but got %s", result.String())
			} else {
				assert.NoError(t, err, "Unexpected error")
				assert.Equal(t, tt.expectPath, result.String(), "Expected %s but got %s", tt.expectPath, result.String())
			}
		})
	}
}

func TestNewResolvedPathParentOnly(t *testing.T) {
	tmpDir := t.TempDir()

	resolvedDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	existingFile := filepath.Join(tmpDir, "existing.txt")
	if err := os.WriteFile(existingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(existingFile, symlinkPath); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		path        string
		expectError bool
		expectErr   error
		expectPath  string
	}{
		{
			name:       "new file in existing dir",
			path:       filepath.Join(tmpDir, "newfile.txt"),
			expectPath: filepath.Join(resolvedDir, "newfile.txt"),
		},
		{
			name:      "empty path should fail",
			expectErr: ErrEmptyPath,
		},
		{
			name:        "non-existent parent dir should fail",
			path:        filepath.Join(tmpDir, "nosuchdir", "file.txt"),
			expectError: true,
		},
		{
			// AC-4: existing leaf must not prevent success
			name:       "existing file should succeed",
			path:       existingFile,
			expectPath: filepath.Join(resolvedDir, "existing.txt"),
		},
		{
			// AC-4: existing symlink leaf must not prevent success
			name:       "existing symlink should succeed",
			path:       symlinkPath,
			expectPath: filepath.Join(resolvedDir, "link.txt"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewResolvedPathParentOnly(tt.path)

			switch {
			case tt.expectErr != nil:
				assert.ErrorIs(t, err, tt.expectErr)
				assert.Empty(t, result.String())
			case tt.expectError:
				assert.Error(t, err)
				assert.Empty(t, result.String())
			default:
				assert.NoError(t, err)
				assert.Equal(t, tt.expectPath, result.String())
			}
		})
	}
}

func TestNewResolvedPathForNew(t *testing.T) {
	tmpDir := t.TempDir()

	resolvedDir, err := filepath.EvalSymlinks(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	existingFile := filepath.Join(tmpDir, "existing.txt")
	if err := os.WriteFile(existingFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	symlinkPath := filepath.Join(tmpDir, "link.txt")
	if err := os.Symlink(existingFile, symlinkPath); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		path        string
		expectError bool
		expectPath  string
	}{
		{
			name:       "new file in existing dir",
			path:       filepath.Join(tmpDir, "newfile.txt"),
			expectPath: filepath.Join(resolvedDir, "newfile.txt"),
		},
		{
			name:        "empty path should fail",
			expectError: true,
		},
		{
			name:        "non-existent parent dir should fail",
			path:        filepath.Join(tmpDir, "nosuchdir", "file.txt"),
			expectError: true,
		},
		{
			name:        "existing file should fail",
			path:        existingFile,
			expectError: true,
		},
		{
			name:        "existing symlink should fail",
			path:        symlinkPath,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ResolvedPath{}
			rp, err := NewResolvedPathParentOnly(tt.path)
			if err == nil {
				if _, lerr := os.Lstat(rp.String()); lerr == nil {
					err = os.ErrExist
				} else if !os.IsNotExist(lerr) {
					err = lerr
				} else {
					result = rp
				}
			}

			switch {
			case tt.expectError:
				assert.Error(t, err)
				assert.Empty(t, result.String())
			default:
				assert.NoError(t, err)
				assert.Equal(t, tt.expectPath, result.String())
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
			assert.Equal(t, tt.want, got, "ContainsPathTraversalSegment(%q) = %v; want %v", tt.path, got, tt.want)
		})
	}
}
