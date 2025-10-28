package safefileio

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
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
		{
			name: "write with group writable permissions should succeed for owned file",
			setup: func(t *testing.T) (string, []byte, os.FileMode) {
				tempDir := safeTempDir(t)
				filePath := filepath.Join(tempDir, "group_writable_new.txt")
				content := []byte("test content")
				// Use group writable permissions
				return filePath, content, 0o664
			},
			wantErr: false,
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
				err := os.WriteFile(filePath, content, 0o600)
				require.NoError(t, err, "Failed to create test file")
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
		{
			name: "world writable file should fail",
			setup: func(t *testing.T) string {
				tempDir := safeTempDir(t)
				filePath := filepath.Join(tempDir, "world_writable.txt")

				// Create file with world writable permissions (666)
				require.NoError(t, os.WriteFile(filePath, []byte("test content"), 0o666), "Failed to create test file")

				// Explicitly set world writable permissions to bypass umask
				require.NoError(t, os.Chmod(filePath, 0o666), "Failed to set world writable permissions")

				return filePath
			},
			wantErr: true,
			errType: groupmembership.ErrFileWorldWritable,
		},
		{
			name: "group writable file owned by current user should succeed",
			setup: func(t *testing.T) string {
				tempDir := safeTempDir(t)
				filePath := filepath.Join(tempDir, "group_writable.txt")

				// Create file with group writable permissions (664)
				// Since the test creates the file, the current user will be the owner
				// and will be in the file's group
				require.NoError(t, os.WriteFile(filePath, []byte("test content"), 0o664), "Failed to create test file")

				return filePath
			},
			want:    []byte("test content"),
			wantErr: false,
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

func TestValidateFilePermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions os.FileMode
		operation   groupmembership.FileOperation
		expectError bool
		errorType   error
	}{
		{
			name:        "normal permissions (644) for read",
			permissions: 0o644,
			operation:   groupmembership.FileOpRead,
			expectError: false,
		},
		{
			name:        "normal permissions (644) for write",
			permissions: 0o644,
			operation:   groupmembership.FileOpWrite,
			expectError: false,
		},
		{
			name:        "executable permissions (755) for read",
			permissions: 0o755,
			operation:   groupmembership.FileOpRead,
			expectError: false,
		},
		{
			name:        "executable permissions (755) for write - should fail",
			permissions: 0o755,
			operation:   groupmembership.FileOpWrite,
			expectError: true,
			errorType:   groupmembership.ErrPermissionsExceedMaximum,
		},
		{
			name:        "setuid permissions (4755) for read",
			permissions: 0o4755,
			operation:   groupmembership.FileOpRead,
			expectError: false,
		},
		{
			name:        "setuid permissions (4755) for write - should fail",
			permissions: 0o4755,
			operation:   groupmembership.FileOpWrite,
			expectError: true,
			errorType:   groupmembership.ErrPermissionsExceedMaximum,
		},
		{
			name:        "normal permissions (600) for read",
			permissions: 0o600,
			operation:   groupmembership.FileOpRead,
			expectError: false,
		},
		{
			name:        "normal permissions (600) for write",
			permissions: 0o600,
			operation:   groupmembership.FileOpWrite,
			expectError: false,
		},
		{
			name:        "group writable (664) should succeed for read when user is in group",
			permissions: 0o664,
			operation:   groupmembership.FileOpRead,
			expectError: false,
		},
		{
			name:        "group writable (664) for write should succeed if user is only group member",
			permissions: 0o664,
			operation:   groupmembership.FileOpWrite,
			expectError: false,
		},
		{
			name:        "world writable (666) should fail for read",
			permissions: 0o666,
			operation:   groupmembership.FileOpRead,
			expectError: true,
			errorType:   groupmembership.ErrFileWorldWritable,
		},
		{
			name:        "world writable (666) should fail for write",
			permissions: 0o666,
			operation:   groupmembership.FileOpWrite,
			expectError: true,
			errorType:   groupmembership.ErrPermissionsExceedMaximum,
		},
		{
			name:        "world writable and executable (777) should fail for read",
			permissions: 0o777,
			operation:   groupmembership.FileOpRead,
			expectError: true,
			errorType:   groupmembership.ErrFileWorldWritable,
		},
		{
			name:        "world writable and executable (777) should fail for write",
			permissions: 0o777,
			operation:   groupmembership.FileOpWrite,
			expectError: true,
			errorType:   groupmembership.ErrPermissionsExceedMaximum,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := safeTempDir(t)
			filePath := filepath.Join(tempDir, "test_permissions.txt")

			// Create file with specified permissions
			require.NoError(t, os.WriteFile(filePath, []byte("test content"), tt.permissions), "Failed to create test file")

			// For world writable tests, explicitly set the permissions using chmod
			// to bypass umask restrictions
			const worldWritePermission = 0o002
			if tt.permissions&worldWritePermission != 0 {
				require.NoError(t, os.Chmod(filePath, tt.permissions), "Failed to set world writable permissions")
			}

			// Try to test the operation based on the test case
			var err error
			switch tt.operation {
			case groupmembership.FileOpRead:
				_, err = SafeReadFile(filePath)
			case groupmembership.FileOpWrite:
				err = SafeWriteFileOverwrite(filePath, []byte("new content"), tt.permissions)
			}

			if tt.expectError {
				assert.Error(t, err, "Expected error for permissions %o with operation %v", tt.permissions, tt.operation)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType, "Expected specific error type for permissions %o with operation %v", tt.permissions, tt.operation)
				}
			} else {
				assert.NoError(t, err, "Expected no error for permissions %o with operation %v", tt.permissions, tt.operation)
			}
		})
	}
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

func TestSetuidSetgidBehavior(t *testing.T) {
	t.Run("SafeReadFile allows reading file with setuid/setgid bits", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "setuid_setgid_read.txt")

		// Create file normally first to avoid umask surprises, then chmod explicitly
		content := []byte("read-ok")
		require.NoError(t, os.WriteFile(filePath, content, 0o644), "failed to create file")

		// Explicitly set setuid and setgid bits; avoid umask by chmod after creation
		require.NoError(t, os.Chmod(filePath, 0o6755), "failed to chmod setuid/setgid")

		got, err := SafeReadFile(filePath)
		assert.NoError(t, err, "SafeReadFile should allow reading file with setuid/setgid bits")
		assert.Equal(t, string(content), string(got))
	})

	t.Run("SafeWriteFile forbids creating file with setuid/setgid bits", func(t *testing.T) {
		tempDir := safeTempDir(t)
		filePath := filepath.Join(tempDir, "setuid_setgid_create.txt")

		// Try to create a new file with setuid/setgid bits in requested perm
		err := SafeWriteFile(filePath, []byte("deny"), 0o6755)
		assert.Error(t, err, "SafeWriteFile should reject setuid/setgid perms on creation")
		assert.ErrorIs(t, err, groupmembership.ErrPermissionsExceedMaximum)
		// Note: depending on the kernel/filesystem, the file may have been created
		// before validation failed. We don't assert non-existence to avoid flakiness.
		// Cleanup if it exists.
		if _, statErr := os.Lstat(filePath); statErr == nil {
			_ = os.Remove(filePath)
		}
	})
}

// TestValidateFileOperationDifferences tests that read and write operations have different permission requirements
func TestValidateFileOperationDifferences(t *testing.T) {
	tempDir := safeTempDir(t)

	// Test executable file - should be allowed for read but not for write
	execFilePath := filepath.Join(tempDir, "executable_file.txt")
	require.NoError(t, os.WriteFile(execFilePath, []byte("executable content"), 0o755))

	// Read should succeed
	_, err := SafeReadFile(execFilePath)
	assert.NoError(t, err, "Reading executable file should succeed")

	// Write should fail
	err = SafeWriteFileOverwrite(execFilePath, []byte("new content"), 0o755)
	assert.Error(t, err, "Writing to executable file should fail")
	assert.ErrorIs(t, err, groupmembership.ErrPermissionsExceedMaximum, "Should fail with permission error")

	// Test setuid file - should be allowed for read but not for write
	setuidFilePath := filepath.Join(tempDir, "setuid_file.txt")
	require.NoError(t, os.WriteFile(setuidFilePath, []byte("setuid content"), 0o644))
	// Explicitly set setuid bit after file creation
	require.NoError(t, os.Chmod(setuidFilePath, 0o4644))

	// Read should succeed
	_, err = SafeReadFile(setuidFilePath)
	assert.NoError(t, err, "Reading setuid file should succeed")

	// Write should fail - try to create a new file with setuid permissions
	newSetuidFilePath := filepath.Join(tempDir, "new_setuid_file.txt")
	err = SafeWriteFile(newSetuidFilePath, []byte("new content"), 0o4644)
	assert.Error(t, err, "Creating a file with setuid permissions should fail")
	assert.ErrorIs(t, err, groupmembership.ErrPermissionsExceedMaximum, "Should fail with permission error")
}

func TestSafeAtomicMoveFile(t *testing.T) {
	t.Run("successful atomic move with permission setting", func(t *testing.T) {
		tempDir := safeTempDir(t)
		srcPath := filepath.Join(tempDir, "source.txt")
		dstPath := filepath.Join(tempDir, "destination.txt")
		content := []byte("test content for atomic move")

		// Create source file with loose permissions
		require.NoError(t, os.WriteFile(srcPath, content, 0o644))

		// Move with secure permissions
		err := SafeAtomicMoveFile(srcPath, dstPath, 0o600)
		assert.NoError(t, err, "SafeAtomicMoveFile should succeed")

		// Verify source file is gone
		_, err = os.Stat(srcPath)
		assert.True(t, os.IsNotExist(err), "Source file should not exist after move")

		// Verify destination file exists with correct content and permissions
		stat, err := os.Stat(dstPath)
		require.NoError(t, err, "Destination file should exist")
		assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm(), "Destination should have 0600 permissions")

		gotContent, err := os.ReadFile(dstPath)
		require.NoError(t, err, "Should be able to read destination file")
		assert.Equal(t, content, gotContent, "Content should match")
	})

	t.Run("move to existing file overwrites", func(t *testing.T) {
		tempDir := safeTempDir(t)
		srcPath := filepath.Join(tempDir, "source.txt")
		dstPath := filepath.Join(tempDir, "destination.txt")
		srcContent := []byte("new content")
		oldContent := []byte("old content")

		// Create source and destination files
		require.NoError(t, os.WriteFile(srcPath, srcContent, 0o600))
		require.NoError(t, os.WriteFile(dstPath, oldContent, 0o600))

		// Move should overwrite destination
		err := SafeAtomicMoveFile(srcPath, dstPath, 0o600)
		assert.NoError(t, err, "SafeAtomicMoveFile should succeed with overwrite")

		// Verify content was overwritten
		gotContent, err := os.ReadFile(dstPath)
		require.NoError(t, err, "Should be able to read destination file")
		assert.Equal(t, srcContent, gotContent, "Content should be from source file")
	})

	t.Run("fails with invalid permissions", func(t *testing.T) {
		tempDir := safeTempDir(t)
		srcPath := filepath.Join(tempDir, "source.txt")
		dstPath := filepath.Join(tempDir, "destination.txt")

		require.NoError(t, os.WriteFile(srcPath, []byte("test"), 0o600))

		// Try to move with invalid permissions (too permissive for write operation)
		err := SafeAtomicMoveFile(srcPath, dstPath, 0o755)
		assert.Error(t, err, "Should fail with overly permissive permissions")
		assert.ErrorIs(t, err, groupmembership.ErrPermissionsExceedMaximum)
	})

	t.Run("fails when source does not exist", func(t *testing.T) {
		tempDir := safeTempDir(t)
		srcPath := filepath.Join(tempDir, "nonexistent.txt")
		dstPath := filepath.Join(tempDir, "destination.txt")

		err := SafeAtomicMoveFile(srcPath, dstPath, 0o600)
		assert.Error(t, err, "Should fail when source file does not exist")
	})

	t.Run("creates destination directory structure", func(t *testing.T) {
		tempDir := safeTempDir(t)
		srcPath := filepath.Join(tempDir, "source.txt")
		dstPath := filepath.Join(tempDir, "subdir", "destination.txt")
		content := []byte("test content")

		require.NoError(t, os.WriteFile(srcPath, content, 0o600))
		require.NoError(t, os.MkdirAll(filepath.Dir(dstPath), 0o750))

		err := SafeAtomicMoveFile(srcPath, dstPath, 0o600)
		assert.NoError(t, err, "Should succeed when destination directory exists")

		// Verify move was successful
		gotContent, err := os.ReadFile(dstPath)
		require.NoError(t, err, "Should be able to read destination file")
		assert.Equal(t, content, gotContent, "Content should match")
	})

	t.Run("prevents file descriptor leakage attack", func(t *testing.T) {
		tempDir := safeTempDir(t)
		srcPath := filepath.Join(tempDir, "source.txt")
		dstPath := filepath.Join(tempDir, "destination.txt")

		// 1. Create initial target file with 0o644 permissions (potentially vulnerable)
		oldContent := []byte("sensitive old content")
		require.NoError(t, os.WriteFile(dstPath, oldContent, 0o644))

		// 2. Simulate attacker opening the file and keeping the descriptor
		attackerFd, err := os.Open(dstPath)
		require.NoError(t, err, "Attacker should be able to open the file")
		defer attackerFd.Close()

		// Verify attacker can read original content
		attackerContent := make([]byte, len(oldContent))
		n, err := attackerFd.Read(attackerContent)
		require.NoError(t, err, "Attacker should be able to read original content")
		assert.Equal(t, oldContent, attackerContent[:n], "Attacker should see original content")

		// 3. Create source file with new sensitive content
		newContent := []byte("new sensitive content that should not leak")
		require.NoError(t, os.WriteFile(srcPath, newContent, 0o600))

		// 4. Use SafeAtomicMoveFile to overwrite the target
		err = SafeAtomicMoveFile(srcPath, dstPath, 0o600)
		assert.NoError(t, err, "SafeAtomicMoveFile should succeed")

		// 5. Verify the file was overwritten with new content
		finalContent, err := os.ReadFile(dstPath)
		require.NoError(t, err, "Should be able to read final file")
		assert.Equal(t, newContent, finalContent, "File should contain new content")

		// 6. Critical security check: Attacker's old file descriptor should NOT see new content
		// Reset file descriptor position and try to read
		_, err = attackerFd.Seek(0, 0)
		require.NoError(t, err, "Should be able to seek to beginning")

		// Try to read new content through old descriptor
		attackerNewRead := make([]byte, len(newContent))
		n, readErr := attackerFd.Read(attackerNewRead)

		// The behavior depends on the filesystem and OS, but we expect one of these outcomes:
		// 1. Read error (file descriptor becomes invalid)
		// 2. Read returns old content or empty (not new content)
		// 3. Read returns fewer bytes than expected

		switch {
		case readErr != nil:
			// Good: File descriptor became invalid (best case)
			t.Logf("Attacker's file descriptor became invalid after atomic move: %v", readErr)
		case n == 0:
			// Good: No content readable
			t.Logf("Attacker's file descriptor returned no content")
		default:
			// Check if attacker can see new content (this would be a security issue)
			attackerReadContent := attackerNewRead[:n]
			if bytes.Equal(attackerReadContent, newContent) {
				t.Errorf("SECURITY ISSUE: Attacker can read new content through old file descriptor")
				t.Errorf("Expected: old content or error, Got: new content")
			} else {
				// Good: Attacker sees old content or garbage, not new content
				t.Logf("Attacker's file descriptor sees different content (safe): %q", attackerReadContent)
			}
		}

		// 7. Verify file permissions are secure (0o600)
		stat, err := os.Stat(dstPath)
		require.NoError(t, err, "Should be able to stat final file")
		assert.Equal(t, os.FileMode(0o600), stat.Mode().Perm(), "Final file should have secure permissions")
	})
}

// TestCanSafelyWriteToFile tests the new unified security validation function
func TestCanSafelyWriteToFile(t *testing.T) {
	tests := []struct {
		name        string
		permissions os.FileMode
		operation   groupmembership.FileOperation
		expectError bool
		errorType   error
	}{
		{
			name:        "read regular file with 0o644 should succeed",
			permissions: 0o644,
			operation:   groupmembership.FileOpRead,
			expectError: false,
		},
		{
			name:        "write regular file with 0o644 should succeed",
			permissions: 0o644,
			operation:   groupmembership.FileOpWrite,
			expectError: false,
		},
		{
			name:        "read file with world writable permissions should fail",
			permissions: 0o666,
			operation:   groupmembership.FileOpRead,
			expectError: true,
			errorType:   groupmembership.ErrFileWorldWritable,
		},
		{
			name:        "write file with world writable permissions should fail",
			permissions: 0o666,
			operation:   groupmembership.FileOpWrite,
			expectError: true,
			errorType:   groupmembership.ErrPermissionsExceedMaximum,
		},
		{
			name:        "read file with excessive permissions should fail",
			permissions: 0o777,
			operation:   groupmembership.FileOpRead,
			expectError: true,
			errorType:   groupmembership.ErrFileWorldWritable,
		},
		{
			name:        "write file with excessive permissions should fail",
			permissions: 0o777,
			operation:   groupmembership.FileOpWrite,
			expectError: true,
			errorType:   groupmembership.ErrPermissionsExceedMaximum,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := safeTempDir(t)
			filePath := filepath.Join(tempDir, fmt.Sprintf("test_%s.txt", tt.name))

			// Create test file with specified permissions
			require.NoError(t, os.WriteFile(filePath, []byte("test content"), tt.permissions), "Failed to create test file")

			if tt.permissions&0o002 != 0 {
				// For world writable test, need to explicitly chmod after creation
				require.NoError(t, os.Chmod(filePath, tt.permissions), "Failed to set world writable permissions")
			}

			// Test the unified security validation function through the high-level API
			// This will internally use the CanSafelyWriteToFile function via validateFile
			var err error
			switch tt.operation {
			case groupmembership.FileOpRead:
				_, err = SafeReadFile(filePath)
			case groupmembership.FileOpWrite:
				err = SafeWriteFileOverwrite(filePath, []byte("new content"), tt.permissions)
			}

			if tt.expectError {
				assert.Error(t, err, "Expected error for permissions %o with operation %v", tt.permissions, tt.operation)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType, "Expected specific error type for permissions %o with operation %v", tt.permissions, tt.operation)
				}
			} else {
				assert.NoError(t, err, "Expected no error for permissions %o with operation %v", tt.permissions, tt.operation)
			}
		})
	}
}

func TestCanSafelyReadFromFile(t *testing.T) {
	tests := []struct {
		name        string
		permissions os.FileMode
		expectError bool
		errorType   error
	}{
		{
			name:        "normal permissions (644) for read",
			permissions: 0o644,
			expectError: false,
		},
		{
			name:        "group writable (664) should succeed for read - more permissive than write",
			permissions: 0o664,
			expectError: false,
		},
		{
			name:        "world writable (666) should fail for read",
			permissions: 0o666,
			expectError: true,
			errorType:   groupmembership.ErrFileWorldWritable,
		},
		{
			name:        "setuid permissions (4755) should succeed for read",
			permissions: 0o4755,
			expectError: false,
		},
		{
			name:        "setuid with group writable (4775) should succeed for read",
			permissions: 0o4775,
			expectError: false,
		},
		{
			name:        "executable permissions (755) should succeed for read",
			permissions: 0o755,
			expectError: false,
		},
		{
			name:        "owner only (600) should succeed for read",
			permissions: 0o600,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary file
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "test_file")

			// Create test file with specified permissions
			require.NoError(t, os.WriteFile(filePath, []byte("test content"), tt.permissions), "Failed to create test file")

			if tt.permissions&0o002 != 0 {
				// For world writable test, need to explicitly chmod after creation
				require.NoError(t, os.Chmod(filePath, tt.permissions), "Failed to set world writable permissions")
			}

			// Test the read-specific security validation function
			fs := &osFS{groupMembership: groupmembership.New()}
			file, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
			require.NoError(t, err, "Failed to open file for testing")
			defer func() {
				assert.NoError(t, file.Close(), "Failed to close file")
			}()

			// Test CanSafelyReadFromFile directly
			_, err = canSafelyReadFromFile(file, filePath, fs.GetGroupMembership())

			if tt.expectError {
				assert.Error(t, err, "Expected error for permissions %o", tt.permissions)
				if tt.errorType != nil {
					assert.True(t, errors.Is(err, tt.errorType), "Expected error type %T, got %v", tt.errorType, err)
				}
			} else {
				assert.NoError(t, err, "Expected no error for permissions %o", tt.permissions)
			}
		})
	}
}

func TestSafeReadFileWithRelaxedPermissions(t *testing.T) {
	t.Run("SafeReadFile should succeed with group writable file using new read permissions", func(t *testing.T) {
		// Create temporary file with group writable permissions
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "group_writable_file")
		content := []byte("test content for group writable file")

		// Create test file with group writable permissions (0o664)
		require.NoError(t, os.WriteFile(filePath, content, 0o664), "Failed to create test file")

		// Test that SafeReadFile now succeeds with the new read-specific validation
		result, err := SafeReadFile(filePath)
		assert.NoError(t, err, "SafeReadFile should succeed with group writable file using new read permissions")
		assert.Equal(t, content, result, "File content should match")
	})

	t.Run("SafeReadFile should still fail with world writable file", func(t *testing.T) {
		// Create temporary file with world writable permissions
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "world_writable_file")
		content := []byte("test content for world writable file")

		// Create test file with world writable permissions (0o666)
		require.NoError(t, os.WriteFile(filePath, content, 0o666), "Failed to create test file")
		require.NoError(t, os.Chmod(filePath, 0o666), "Failed to set world writable permissions")

		// Test that SafeReadFile still fails with world writable files
		_, err := SafeReadFile(filePath)
		assert.Error(t, err, "SafeReadFile should fail with world writable file")
		assert.True(t, errors.Is(err, groupmembership.ErrFileWorldWritable), "Error should be ErrFileWorldWritable")
	})
}
