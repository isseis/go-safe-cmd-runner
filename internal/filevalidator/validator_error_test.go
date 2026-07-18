//go:build linux || freebsd || openbsd || netbsd

package filevalidator

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestErrorCases tests various error conditions and their messages
func TestErrorCases(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir, ValidatorConfig{})
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
			wantErr:     common.ErrEmptyPath,
			errContains: "path cannot be empty",
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

			// Test SaveRecord
			_, _, err = validator.SaveRecord(filePath, false)
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
	validator, err := New(&SHA256{}, tempDir, ValidatorConfig{})
	assert.NoError(t, err, "Failed to create validator")

	t.Run("deleted file", func(t *testing.T) {
		// Create and record a file
		filePath := filepath.Join(tempDir, "deleted.txt")
		assert.NoError(t, os.WriteFile(filePath, []byte("test"), 0o644), "Failed to create test file")
		_, _, err := validator.SaveRecord(filePath, false)
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

		_, _, err := validator.SaveRecord(dirPath, false)
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

		_, _, err := validator.SaveRecord(filePath, false)
		assert.Error(t, err, "Expected error for read-only filesystem")
		assert.ErrorIs(t, err, os.ErrPermission, "Expected permission error")
	})
}

// TestNewReadOnly_ParentUnreadable_DeferredPermissionError tests that when
// hashDir does not exist and its parent directory cannot be traversed
// (Lstat returns a permission error rather than IsNotExist), NewReadOnly
// still succeeds construction and defers the permission error to Verify,
// per 02_architecture.md Q-03 (the successor to the removed dry-run
// os.ErrPermission fallback).
//
// Like the "unreadable directory" subtest above, this test is meaningless
// when run as root, since chmod 0o000 does not deny access to root. This is
// an existing constraint of this file's permission tests, not one newly
// introduced here.
func TestNewReadOnly_ParentUnreadable_DeferredPermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping privilege test when running as root")
	}
	tempDir := safeTempDir(t)
	restrictedDir := filepath.Join(tempDir, "restricted")
	require.NoError(t, os.Mkdir(restrictedDir, 0o755))

	// hashDir is a path under restrictedDir that is never created.
	hashDir := filepath.Join(restrictedDir, "hashes")

	require.NoError(t, os.Chmod(restrictedDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(restrictedDir, 0o755) })

	validator, err := NewReadOnly(&SHA256{}, hashDir, ValidatorConfig{})
	require.NoError(t, err, "NewReadOnly should succeed even when Lstat fails with a permission error")
	require.NotNil(t, validator)
	assert.False(t, validator.HashDirAvailable())

	verifyErr := validator.Verify(filepath.Join(tempDir, "any_file.txt"))
	assert.ErrorIs(t, verifyErr, os.ErrPermission)
}

// TestNewReadOnly_ExistingDirUnresolvable_DeferredPermissionError tests the
// err == nil && info.IsDir() branch of NewReadOnly: hashDir itself exists and
// is Lstat-able (via a relative path, which the kernel resolves against the
// already-open cwd without re-checking ancestor search permissions), but
// resolving its absolute, symlink-free path via fileanalysis.NewStoreReadOnly
// / common.NewResolvedPath fails with EACCES because an ancestor directory of
// cwd is unreadable. NewReadOnly must defer this error like the other
// access-error branches instead of failing construction outright.
func TestNewReadOnly_ExistingDirUnresolvable_DeferredPermissionError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping privilege test when running as root")
	}
	tempDir := safeTempDir(t)
	restrictedDir := filepath.Join(tempDir, "restricted")
	workDir := filepath.Join(restrictedDir, "workdir")
	hashDir := filepath.Join(workDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o755))

	t.Chdir(workDir)

	require.NoError(t, os.Chmod(restrictedDir, 0o000))
	t.Cleanup(func() { _ = os.Chmod(restrictedDir, 0o755) })

	// Relative path: os.Lstat resolves it against the process's cwd directly,
	// so it succeeds even though "restricted" (an ancestor of cwd) is unreadable.
	validator, err := NewReadOnly(&SHA256{}, "hashes", ValidatorConfig{})
	require.NoError(t, err, "NewReadOnly should succeed even when the hash directory's absolute path cannot be resolved")
	require.NotNil(t, validator)
	assert.False(t, validator.HashDirAvailable())

	verifyErr := validator.Verify(filepath.Join(workDir, "any_file.txt"))
	assert.ErrorIs(t, verifyErr, os.ErrPermission)
}

// TestErrorMessages verifies that error messages are clear and helpful
func TestErrorMessages(t *testing.T) {
	tempDir := safeTempDir(t)
	validator, err := New(&SHA256{}, tempDir, ValidatorConfig{})
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
			expectedErr: common.ErrEmptyPath,
			errContains: "path cannot be empty",
		},
		{
			name:        "non-existent file",
			filePath:    filepath.Join(tempDir, "nonexistent.txt"),
			expectedErr: os.ErrNotExist,
			errContains: "no such file or directory",
			skipVerify:  true, // Skip verify as it's the same as SaveRecord for non-existent files
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test SaveRecord
			_, _, err := validator.SaveRecord(tt.filePath, false)
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
