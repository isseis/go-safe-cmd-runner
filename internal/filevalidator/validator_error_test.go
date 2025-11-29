//go:build linux || freebsd || openbsd || netbsd

package filevalidator

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorCases tests various error conditions and their messages
func TestErrorCases(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir)
	assert.NoError(t, err, "Failed to create validator")

	tests := []struct {
		name        string
		setup       func() (string, error)
		wantErr     error
		errContains string
	}{
		{
			name: "non-existent file",
			setup: func() (string, error) {
				return filepath.Join(tempDir, "nonexistent.txt"), nil
			},
			wantErr:     os.ErrNotExist,
			errContains: "no such file or directory",
		},
		{
			name: "empty file path",
			setup: func() (string, error) {
				return "", nil
			},
			wantErr:     safefileio.ErrInvalidFilePath,
			errContains: "invalid file path",
		},
		{
			name: "permission denied",
			setup: func() (string, error) {
				// Create a directory with no read permissions
				dirPath := filepath.Join(tempDir, "restricted")
				if err := os.Mkdir(dirPath, 0o000); err != nil {
					return "", err
				}
				t.Cleanup(func() { _ = os.Chmod(dirPath, 0o755) }) // Ensure cleanup

				// Create a file in the restricted directory
				filePath := filepath.Join(dirPath, "test.txt")
				if err := os.WriteFile(filePath, []byte("test"), 0o400); err == nil {
					return "", safefileio.ErrInvalidFilePath
				}

				return filePath, nil
			},
			wantErr:     os.ErrPermission,
			errContains: "permission denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath, err := tt.setup()
			assert.NoError(t, err, "Setup failed")

			// Test Record
			_, err = validator.Record(filePath, false)
			if tt.wantErr != nil {
				assert.Error(t, err, "Expected error")
				assert.ErrorIs(t, err, tt.wantErr, "Expected specific error type")
			} else {
				assert.NoError(t, err, "Unexpected error")
			}

			// Test Verify
			err = validator.Verify(filePath)
			if tt.wantErr != nil {
				assert.Error(t, err, "Expected error")
				assert.ErrorIs(t, err, tt.wantErr, "Expected specific error type")
			} else {
				assert.NoError(t, err, "Unexpected error")
			}
		})
	}
}

// TestFilesystemEdgeCases tests various edge cases related to filesystem operations
func TestFilesystemEdgeCases(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir)
	assert.NoError(t, err, "Failed to create validator")

	t.Run("deleted file", func(t *testing.T) {
		// Create and record a file
		filePath := filepath.Join(tempDir, "deleted.txt")
		assert.NoError(t, os.WriteFile(filePath, []byte("test"), 0o644), "Failed to create test file")
		_, err := validator.Record(filePath, false)
		assert.NoError(t, err, "Failed to record file")

		// Delete the file
		assert.NoError(t, os.Remove(filePath), "Failed to delete test file")

		// Verify should fail with file not found
		err = validator.Verify(filePath)
		assert.Error(t, err, "Expected error for deleted file")
		// Check the error type
		assert.ErrorIs(t, err, os.ErrNotExist, "Expected file not found error")
	})

	t.Run("directory instead of file", func(t *testing.T) {
		dirPath := filepath.Join(tempDir, "subdir")
		assert.NoError(t, os.Mkdir(dirPath, 0o755), "Failed to create directory")

		_, err := validator.Record(dirPath, false)
		assert.Error(t, err, "Expected error for directory")
		assert.ErrorIs(t, err, safefileio.ErrInvalidFilePath, "Expected invalid file path error")
	})

	t.Run("unreadable directory", func(t *testing.T) {
		// Create a directory with no read permissions
		dirPath := filepath.Join(tempDir, "noreaddir")
		assert.NoError(t, os.Mkdir(dirPath, 0o700), "Failed to create directory")

		// Create a file in the directory first
		filePath := filepath.Join(dirPath, "test.txt")
		assert.NoError(t, os.WriteFile(filePath, []byte("test"), 0o600), "Failed to create test file")

		// Make the directory unreadable
		assert.NoError(t, os.Chmod(dirPath, 0o000), "Failed to change directory permissions")
		t.Cleanup(func() { _ = os.Chmod(dirPath, 0o700) })

		err := validator.Verify(filePath)
		assert.Error(t, err, "Expected error for unreadable directory")
		// Check for permission error in the error chain
		var perr *os.PathError
		assert.ErrorAs(t, err, &perr)
		assert.True(t, os.IsPermission(perr), "Expected permission error, got: %v", err)
	})

	// This test requires root privileges to create a read-only mount
	// Skipping by default, uncomment if running in a suitable environment
	t.Run("read-only filesystem", func(t *testing.T) {
		t.Skip("Skipping read-only filesystem test as it requires root privileges")

		// This test requires root privileges to create a read-only mount
		roDir := filepath.Join(tempDir, "ro")
		assert.NoError(t, os.Mkdir(roDir, 0o755), "Failed to create directory")

		// Try to make directory read-only (this will only work as root)
		if err := syscall.Mount("tmpfs", roDir, "tmpfs", syscall.MS_RDONLY, ""); err != nil {
			t.Skipf("Skipping read-only filesystem test: %v", err)
		}
		defer syscall.Unmount(roDir, 0)

		filePath := filepath.Join(roDir, "test.txt")
		assert.NoError(t, os.WriteFile(filePath, []byte("test"), 0o644), "Failed to create test file")

		_, err := validator.Record(filePath, false)
		assert.Error(t, err, "Expected error for read-only filesystem")
		assert.ErrorIs(t, err, os.ErrPermission, "Expected permission error")
	})
}

// TestErrorMessages verifies that error messages are clear and helpful
func TestErrorMessages(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir)
	assert.NoError(t, err, "Failed to create validator")

	tests := []struct {
		name        string
		filePath    string
		expectedErr error
		errContains string
		skipVerify  bool // Skip Verify test for certain cases
	}{
		{
			name:        "empty path",
			filePath:    "",
			expectedErr: safefileio.ErrInvalidFilePath,
			errContains: "invalid file path",
		},
		{
			name:        "non-existent file",
			filePath:    filepath.Join(tempDir, "nonexistent.txt"),
			expectedErr: os.ErrNotExist,
			errContains: "no such file or directory",
			skipVerify:  true, // Skip verify as it's the same as Record for non-existent files
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test Record
			_, err := validator.Record(tt.filePath, false)
			require.Error(t, err, "Expected error, got nil")

			// Check error type if expectedErr is set
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			}

			// Skip Verify test if specified
			if tt.skipVerify {
				return
			}

			// Test Verify
			err = validator.Verify(tt.filePath)
			require.Error(t, err, "Expected error for Verify, got nil")

			// Check error type for Verify
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			}
		})
	}
}
