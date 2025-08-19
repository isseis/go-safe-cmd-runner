package safefileio

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// safeTempDir creates a temporary directory and resolves any symlinks in its path
// to ensure consistent behavior across different environments.
func safeTempDir(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	// Resolve any symlinks in the path
	realPath, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err, "Failed to resolve symlinks in temp dir")
	return realPath
}

func TestSafeWriteFile(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (string, []byte, os.FileMode)
		wantErr bool
		errType error
	}{
		{
			name: "write to new file",
			setup: func(t *testing.T) (string, []byte, os.FileMode) {
				tempDir := safeTempDir(t)
				filePath := filepath.Join(tempDir, "testfile.txt")
				content := []byte("test content")
				return filePath, content, 0o644
			},
			wantErr: false,
		},
		{
			name: "write to existing file should fail",
			setup: func(t *testing.T) (string, []byte, os.FileMode) {
				tempDir := safeTempDir(t)
				filePath := filepath.Join(tempDir, "existing.txt")
				// Create a file first with 0600 permissions
				require.NoError(t, os.WriteFile(filePath, []byte("old content"), 0o600), "Failed to create test file")
				// Note: safeWriteFile will preserve the original file's permissions
				// rather than using the provided permissions when the file exists
				return filePath, []byte("new content"), 0o600
			},
			wantErr: true,
			errType: ErrFileExists,
		},
		{
			name: "write to directory should fail",
			setup: func(t *testing.T) (string, []byte, os.FileMode) {
				tempDir := safeTempDir(t)
				return tempDir, []byte("should fail"), 0o644
			},
			wantErr: true,
			// The actual error will be from the OS about not being able to write to a directory
			errType: nil,
		},
		{
			name: "write to path containing symlink should fail with ErrIsSymlink",
			setup: func(t *testing.T) (string, []byte, os.FileMode) {
				tempDir := safeTempDir(t)

				// Create a target directory
				targetDir := filepath.Join(tempDir, "target")
				require.NoError(t, os.MkdirAll(targetDir, 0o755), "Failed to create target directory")

				// Create a directory that will contain our test files
				testDir := filepath.Join(tempDir, "testdir")
				require.NoError(t, os.Mkdir(testDir, 0o755), "Failed to create test directory")

				// Create a symlink inside our test directory
				symlinkPath := filepath.Join(testDir, "symlink")
				require.NoError(t, os.Symlink(targetDir, symlinkPath), "Failed to create symlink")

				// Create a file path that includes the symlink
				filePath := filepath.Join(symlinkPath, "file.txt")
				return filePath, []byte("test content"), 0o644
			},
			wantErr: true,
			errType: ErrIsSymlink,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, content, perm := tt.setup(t)

			err := SafeWriteFile(path, content, perm)
			if tt.wantErr {
				assert.Error(t, err, "SafeWriteFile() should return an error")
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType, "SafeWriteFile() error should be of expected type")
				}
			} else {
				assert.NoError(t, err, "SafeWriteFile() should not return an error")
			}

			if !tt.wantErr {
				// Verify file was created with correct content and permissions
				info, err := os.Lstat(path)
				require.NoError(t, err, "Failed to stat file")

				// On Unix-like systems, the actual permissions might be affected by umask
				// So we'll only check that the file is readable and writable by the owner
				assert.True(t, info.Mode()&0o600 == 0o600, "File should be readable and writable by owner, got permissions %v", info.Mode())

				gotContent, err := os.ReadFile(path)
				require.NoError(t, err, "Failed to read file")

				assert.Equal(t, string(content), string(gotContent), "File content should match")
			}
		})
	}
}

func TestSafeReadFile(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		want    []byte
		wantErr bool
		errType error
	}{
		{
			name: "read existing file",
			setup: func(t *testing.T) string {
				tempDir := safeTempDir(t)
				filePath := filepath.Join(tempDir, "testfile.txt")
				content := []byte("test content")
				if err := os.WriteFile(filePath, content, 0o600); err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return filePath
			},
			want:    []byte("test content"),
			wantErr: false,
		},
		{
			name: "non-existent file",
			setup: func(t *testing.T) string {
				tempDir := safeTempDir(t)
				return filepath.Join(tempDir, "nonexistent.txt")
			},
			wantErr: true,
		},
		{
			name: "directory instead of file",
			setup: func(t *testing.T) string {
				tempDir := safeTempDir(t)
				return tempDir
			},
			wantErr: true,
			errType: ErrInvalidFilePath,
		},
		{
			name: "symlink to file",
			setup: func(t *testing.T) string {
				tempDir := safeTempDir(t)
				targetFile := filepath.Join(tempDir, "target.txt")
				symlink := filepath.Join(tempDir, "symlink.txt")

				// Create target file
				require.NoError(t, os.WriteFile(targetFile, []byte("target content"), 0o600), "Failed to create target file")

				// Create symlink
				require.NoError(t, os.Symlink(targetFile, symlink), "Failed to create symlink")

				return symlink
			},
			wantErr: true,
			errType: ErrIsSymlink,
		},
		{
			name: "file too large",
			setup: func(t *testing.T) string {
				tempDir := safeTempDir(t)
				filePath := filepath.Join(tempDir, "largefile.bin")

				// Create a file that's slightly larger than the max allowed size
				f, err := os.Create(filePath)
				require.NoError(t, err, "Failed to create test file")
				//nolint:errcheck // In test, we don't need to check the error from Close()
				defer f.Close()

				// Set proper permissions before writing content
				err = f.Chmod(0o644)
				require.NoError(t, err, "Failed to set file permissions")

				// Write MaxFileSize + 1 bytes
				_, err = f.Write(make([]byte, MaxFileSize+1))
				require.NoError(t, err, "Failed to write test data")

				return filePath
			},
			wantErr: true,
			errType: ErrFileTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)

			got, err := SafeReadFile(path)
			if tt.wantErr {
				assert.Error(t, err, "SafeReadFile() should return an error")
				if tt.errType != nil {
					assert.ErrorIs(t, err, tt.errType, "SafeReadFile() error should be of expected type")
				}
				return
			}

			assert.NoError(t, err, "SafeReadFile() should not return an error")
			assert.Equal(t, string(tt.want), string(got), "SafeReadFile() content should match")
		})
	}
}

// failingFile is a file that fails on Close
type failingFile struct {
	File
}

var errSimulatedClose = errors.New("simulated close error")

func (f *failingFile) Close() error {
	// Always return an error when closing
	return errSimulatedClose
}

// failingCloseFS is a FileSystem that returns files that fail on Close
type failingCloseFS struct {
	FileSystem
}

func (fs failingCloseFS) SafeOpenFile(name string, flag int, perm os.FileMode) (File, error) {
	f, err := fs.FileSystem.SafeOpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &failingFile{File: f}, nil
}

// failingWriteCloseFS is a file that fails on Write and Close
type failingWriteCloseFS struct {
	File
}

var errSimulatedWrite = errors.New("simulated write error")

func (f *failingWriteCloseFS) Write(_ []byte) (n int, err error) {
	return 0, errSimulatedWrite
}

func (f *failingWriteCloseFS) Close() error {
	// Call the original Close to ensure cleanup
	_ = f.File.Close()
	return errSimulatedClose
}

// failingWriteFS is a FileSystem that returns files that fail on Write and Close
type failingWriteFS struct {
	FileSystem
}

func (fs failingWriteFS) SafeOpenFile(name string, flag int, perm os.FileMode) (File, error) {
	f, err := fs.FileSystem.SafeOpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	return &failingWriteCloseFS{File: f}, nil
}

func TestSafeWriteFile_FileCloseError(t *testing.T) {
	t.Run("close error only", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "testfile.txt")

		// Create a test file system that will return failing files
		fs := failingCloseFS{FileSystem: defaultFS}
		err := safeWriteFileWithFS(filePath, []byte("test"), 0o644, fs)
		assert.Error(t, err, "Expected error when closing file fails")

		// The error should be related to file closing
		assert.ErrorIs(t, err, errSimulatedClose, "Expected specific close error")
	})

	t.Run("write error takes precedence over close error", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "testfile.txt")

		// Create a test file system that will return files that fail on both write and close
		fs := failingWriteFS{FileSystem: defaultFS}
		err := safeWriteFileWithFS(filePath, []byte("test"), 0o644, fs)
		assert.Error(t, err, "Expected error when writing to file")

		// The error should be the write error, not the close error
		assert.ErrorIs(t, err, errSimulatedWrite, "Expected specific write error")
	})
}
