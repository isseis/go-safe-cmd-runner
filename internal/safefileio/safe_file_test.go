package safefileio

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// mustResolvedPath converts a string path to common.ResolvedPath using NewResolvedPathParentOnly.
// It fails the test if path resolution fails.
func mustResolvedPath(t *testing.T, path string) common.ResolvedPath {
	t.Helper()
	rp, err := common.NewResolvedPathParentOnly(path)
	require.NoError(t, err, "mustResolvedPath: failed to create ResolvedPath for %s", path)
	return rp
}

// openRoute names one of the two paths SafeOpenFile can take. Tests that must
// hold on both run their table once per route.
type openRoute struct {
	name           string
	config         FileSystemConfig
	requireOpenat2 bool
}

var openRoutes = []openRoute{
	{name: "openat2", config: FileSystemConfig{}, requireOpenat2: true},
	{name: "fallback", config: FileSystemConfig{DisableOpenat2: true}},
}

// newFileSystemForRoute builds the FileSystem for a route. The openat2 route
// insists that openat2 really is available, because NewFileSystem falls back
// silently when it is not (Linux 5.5 and older, non-Linux, or a container
// seccomp profile that blocks the call), which would leave a table claiming to
// cover both routes running the fallback path twice.
func newFileSystemForRoute(t *testing.T, route openRoute) FileSystem {
	t.Helper()
	fs := NewFileSystem(route.config)
	if route.requireOpenat2 {
		osfs, ok := fs.(*osFS)
		require.True(t, ok, "NewFileSystem must return *osFS")
		if !osfs.IsOpenat2Available() {
			t.Skip("openat2 is unavailable here, so the openat2 route cannot be exercised")
		}
	}
	return fs
}

func dirFdForTest(t *testing.T, fs FileSystem, dir string) int {
	t.Helper()
	osfs, ok := fs.(*osFS)
	require.True(t, ok, "NewFileSystem must return *osFS")
	fd, err := osfs.openDirNoSymlinks(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })
	return fd
}

// TestSafeOpenFile_RejectsNonPermissionModeBits also asserts that nothing was
// created, since rejecting only after the side effect would be no better than
// accepting.
func TestSafeOpenFile_RejectsNonPermissionModeBits(t *testing.T) {
	perms := []struct {
		name string
		perm os.FileMode
	}{
		{name: "setuid", perm: os.ModeSetuid | 0o644},
		{name: "setgid", perm: os.ModeSetgid | 0o644},
		{name: "sticky", perm: os.ModeSticky | 0o644},
		{name: "dir", perm: os.ModeDir | 0o644},
		{name: "append", perm: os.ModeAppend | 0o644},
	}

	for _, route := range openRoutes {
		t.Run(route.name, func(t *testing.T) {
			fs := newFileSystemForRoute(t, route)
			for _, p := range perms {
				t.Run(p.name, func(t *testing.T) {
					filePath := filepath.Join(tu.SafeTempDir(t), "created.txt")
					_, err := fs.SafeOpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, p.perm)
					require.ErrorIs(t, err, ErrUnsupportedFileMode)
					assert.NoFileExists(t, filePath, "rejection must happen before the file is created")
				})
			}
		})
	}
}

// TestSafeWriteFileOverwrite_RejectsNonPermissionModeBits is a tripwire: the
// write path reaches validateOpenPerm only by going through SafeOpenFile, so
// this fails loudly if a later change reroutes it without carrying the check
// across. ValidateRequestedPermissions cannot stand in, because it masks with
// 0o7777 and so sees plain 0o600 where os.ModeSetuid (1<<23) was requested.
func TestSafeWriteFileOverwrite_RejectsNonPermissionModeBits(t *testing.T) {
	filePath := filepath.Join(tu.SafeTempDir(t), "target.txt")

	err := SafeWriteFileOverwrite(mustResolvedPath(t, filePath), []byte("content"), os.ModeSetuid|0o600)
	require.ErrorIs(t, err, ErrUnsupportedFileMode)
	assert.NoFileExists(t, filePath, "rejection must happen before anything is written")
}

// TestSafeOpenFile_ReadOpenPermIgnoredOnBothPaths covers the divergence this
// task removed: the openat2 route used to fail a non-creating open carrying a
// non-zero perm with EINVAL, while the fallback route succeeded.
func TestSafeOpenFile_ReadOpenPermIgnoredOnBothPaths(t *testing.T) {
	const content = "read-open-content"

	for _, route := range openRoutes {
		t.Run(route.name, func(t *testing.T) {
			fs := newFileSystemForRoute(t, route)
			filePath := filepath.Join(tu.SafeTempDir(t), "existing.txt")
			require.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))

			file, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0o644)
			require.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })

			got, err := io.ReadAll(file)
			require.NoError(t, err)
			assert.Equal(t, content, string(got))
		})
	}
}

// TestSafeOpenFile_CreatePermUnchanged holds the creating open to what the
// kernel always did: perm reduced by the process umask.
func TestSafeOpenFile_CreatePermUnchanged(t *testing.T) {
	// The umask must actually remove a bit of the requested perm, and the
	// expected result must not be a value the package uses as a default
	// anywhere -- otherwise an implementation that ignored perm and hardcoded
	// 0o600 would pass. 0o646 &^ 0o002 == 0o644 satisfies both.
	const fixedUmask = 0o002
	const requestedPerm = os.FileMode(0o646)
	wantPerm := requestedPerm &^ os.FileMode(fixedUmask)

	// Umask is process-wide and this package's tests never call t.Parallel();
	// failing to restore it would quietly break later tests.
	previousUmask := syscall.Umask(fixedUmask)
	t.Cleanup(func() { syscall.Umask(previousUmask) })

	for _, route := range openRoutes {
		t.Run(route.name, func(t *testing.T) {
			fs := newFileSystemForRoute(t, route)
			filePath := filepath.Join(tu.SafeTempDir(t), "created.txt")

			file, err := fs.SafeOpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, requestedPerm)
			require.NoError(t, err)
			require.NoError(t, file.Close())

			info, err := os.Lstat(filePath)
			require.NoError(t, err)
			assert.Equal(t, wantPerm, info.Mode().Perm())
		})
	}
}

// TestSafeOpenFileFallback_CreationProbe covers the open itself, not the
// cleanup: splitting an O_CREATE open into an O_EXCL probe and a reopen is only
// acceptable if the caller cannot tell, so each row states what the caller must
// still see.
func TestSafeOpenFileFallback_CreationProbe(t *testing.T) {
	const existingContent = "content that was already there"

	t.Run("creates_when_absent", func(t *testing.T) {
		filePath := filepath.Join(tu.SafeTempDir(t), "created.txt")

		file, err := safeOpenFileFallback(filePath, os.O_CREATE|os.O_WRONLY, 0o600)
		require.NoError(t, err)
		require.NoError(t, file.Close())
		assert.FileExists(t, filePath)
	})

	t.Run("opens_existing_without_reporting_it_exists", func(t *testing.T) {
		filePath := filepath.Join(tu.SafeTempDir(t), "existing.txt")
		require.NoError(t, os.WriteFile(filePath, []byte(existingContent), 0o600))

		// The probe hits EEXIST here. That is the function's own doing, so it
		// must be absorbed: only a caller that asked for O_EXCL may be told the
		// file exists.
		file, err := safeOpenFileFallback(filePath, os.O_CREATE|os.O_RDONLY, 0o600)
		require.NoError(t, err)
		t.Cleanup(func() { _ = file.Close() })

		got, err := io.ReadAll(file)
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(got), "the reopen must land on the existing file")
	})

	t.Run("reports_exists_when_caller_asked_for_o_excl", func(t *testing.T) {
		filePath := filepath.Join(tu.SafeTempDir(t), "existing.txt")
		require.NoError(t, os.WriteFile(filePath, []byte(existingContent), 0o600))

		_, err := safeOpenFileFallback(filePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		require.ErrorIs(t, err, ErrFileExists)
	})

	t.Run("reopen_failure_other_than_enoent_is_not_retried", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("root opens a 0000 file regardless of its mode, so the reopen would not fail")
		}
		dir := tu.SafeTempDir(t)
		filePath := filepath.Join(dir, "unreadable.txt")
		require.NoError(t, os.WriteFile(filePath, []byte(existingContent), 0o600))
		require.NoError(t, os.Chmod(filePath, 0o000))

		// Only ENOENT means "it was deleted under us, try again"; every other
		// reopen failure is the answer and must come straight back.
		//
		// The returned error alone cannot say that: a loop that retried EACCES
		// to exhaustion would hand back the same EACCES wrapped. What separates
		// the two is the give-up warning, which only the exhausted loop emits.
		recorder := captureWarnings(t)

		_, err := safeOpenFileFallback(filePath, os.O_CREATE|os.O_WRONLY, 0o600)
		require.ErrorIs(t, err, os.ErrPermission)
		assert.Empty(t, recorder.FindRecords(slog.LevelWarn, giveUpOpeningMsg),
			"a non-ENOENT reopen failure must be returned at once, not retried to exhaustion")
	})

	t.Run("rejects_leaf_symlink", func(t *testing.T) {
		dir := tu.SafeTempDir(t)
		targetPath := filepath.Join(dir, "target.txt")
		require.NoError(t, os.WriteFile(targetPath, []byte(existingContent), 0o600))
		linkPath := filepath.Join(dir, "link.txt")
		require.NoError(t, os.Symlink(targetPath, linkPath))

		// An O_EXCL open of a symlink fails with EEXIST even when the link
		// dangles, so the probe sends this case down the reopen. The reopen has
		// to keep O_NOFOLLOW, or a symlink at the leaf would be followed
		// instead of rejected.
		_, err := safeOpenFileFallback(linkPath, os.O_CREATE|os.O_WRONLY, 0o600)
		require.ErrorIs(t, err, ErrIsSymlink)

		got, err := os.ReadFile(targetPath)
		require.NoError(t, err)
		assert.Equal(t, existingContent, string(got), "the link target must not have been touched")
	})
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
				tempDir := tu.SafeTempDir(t)
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
				tempDir := tu.SafeTempDir(t)
				return filepath.Join(tempDir, "nonexistent.txt")
			},
			wantErr: true,
		},
		{
			name: "directory instead of file",
			setup: func(t *testing.T) string {
				tempDir := tu.SafeTempDir(t)
				return tempDir
			},
			wantErr: true,
			errType: ErrInvalidFilePath,
		},
		{
			name: "symlink to file",
			setup: func(t *testing.T) string {
				tempDir := tu.SafeTempDir(t)
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
				tempDir := tu.SafeTempDir(t)
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
				tempDir := tu.SafeTempDir(t)
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
				tempDir := tu.SafeTempDir(t)
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

			rp := mustResolvedPath(t, path)
			got, err := SafeReadFile(rp)
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
			tempDir := tu.SafeTempDir(t)
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
			rp := mustResolvedPath(t, filePath)
			var err error
			switch tt.operation {
			case groupmembership.FileOpRead:
				_, err = SafeReadFile(rp)
			case groupmembership.FileOpWrite:
				err = SafeWriteFileOverwrite(rp, []byte("new content"), tt.permissions)
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

func TestSafeWriteFileOverwrite_FileCloseError(t *testing.T) {
	t.Run("close error only", func(t *testing.T) {
		tempDir := tu.SafeTempDir(t)
		filePath := filepath.Join(tempDir, "testfile.txt")

		fs := failingCloseFS{FileSystem: defaultFS}
		err := safeWriteFileOverwriteWithFS(mustResolvedPath(t, filePath), []byte("test"), 0o644, fs)
		assert.Error(t, err, "Expected error when closing file fails")
		assert.ErrorIs(t, err, errSimulatedClose, "Expected specific close error")
	})

	t.Run("write error takes precedence over close error", func(t *testing.T) {
		tempDir := tu.SafeTempDir(t)
		filePath := filepath.Join(tempDir, "testfile.txt")

		fs := failingWriteFS{FileSystem: defaultFS}
		err := safeWriteFileOverwriteWithFS(mustResolvedPath(t, filePath), []byte("test"), 0o644, fs)
		assert.Error(t, err, "Expected error when writing to file")
		assert.ErrorIs(t, err, errSimulatedWrite, "Expected specific write error")
	})
}

func TestSetuidSetgidBehavior(t *testing.T) {
	t.Run("SafeReadFile allows reading file with setuid/setgid bits", func(t *testing.T) {
		tempDir := tu.SafeTempDir(t)
		filePath := filepath.Join(tempDir, "setuid_setgid_read.txt")

		// Create file normally first to avoid umask surprises, then chmod explicitly
		content := []byte("read-ok")
		require.NoError(t, os.WriteFile(filePath, content, 0o644), "failed to create file")

		// Explicitly set setuid and setgid bits; avoid umask by chmod after creation
		require.NoError(t, os.Chmod(filePath, 0o6755), "failed to chmod setuid/setgid")

		got, err := SafeReadFile(mustResolvedPath(t, filePath))
		assert.NoError(t, err, "SafeReadFile should allow reading file with setuid/setgid bits")
		assert.Equal(t, string(content), string(got))
	})
}

// TestValidateFileOperationDifferences tests that read and write operations have different permission requirements
func TestValidateFileOperationDifferences(t *testing.T) {
	tempDir := tu.SafeTempDir(t)

	// Test executable file - should be allowed for read but not for write
	execFilePath := filepath.Join(tempDir, "executable_file.txt")
	require.NoError(t, os.WriteFile(execFilePath, []byte("executable content"), 0o755))

	// Read should succeed
	_, err := SafeReadFile(mustResolvedPath(t, execFilePath))
	assert.NoError(t, err, "Reading executable file should succeed")

	// Write should fail
	err = SafeWriteFileOverwrite(mustResolvedPath(t, execFilePath), []byte("new content"), 0o755)
	assert.Error(t, err, "Writing to executable file should fail")
	assert.ErrorIs(t, err, groupmembership.ErrPermissionsExceedMaximum, "Should fail with permission error")

	// Test setuid file - should be allowed for read but not for write
	setuidFilePath := filepath.Join(tempDir, "setuid_file.txt")
	require.NoError(t, os.WriteFile(setuidFilePath, []byte("setuid content"), 0o644))
	// Explicitly set setuid bit after file creation
	require.NoError(t, os.Chmod(setuidFilePath, 0o4644))

	// Read should succeed
	_, err = SafeReadFile(mustResolvedPath(t, setuidFilePath))
	assert.NoError(t, err, "Reading setuid file should succeed")
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
			tempDir := tu.SafeTempDir(t)
			filePath := filepath.Join(tempDir, fmt.Sprintf("test_%s.txt", tt.name))

			// Create test file with specified permissions
			require.NoError(t, os.WriteFile(filePath, []byte("test content"), tt.permissions), "Failed to create test file")

			if tt.permissions&0o002 != 0 {
				// For world writable test, need to explicitly chmod after creation
				require.NoError(t, os.Chmod(filePath, tt.permissions), "Failed to set world writable permissions")
			}

			// Test the unified security validation function through the high-level API
			// This will internally use the CanSafelyWriteToFile function via validateFile
			rp := mustResolvedPath(t, filePath)
			var err error
			switch tt.operation {
			case groupmembership.FileOpRead:
				_, err = SafeReadFile(rp)
			case groupmembership.FileOpWrite:
				err = SafeWriteFileOverwrite(rp, []byte("new content"), tt.permissions)
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
			tmpDir := tu.SafeTempDir(t)
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
		tmpDir := tu.SafeTempDir(t)
		filePath := filepath.Join(tmpDir, "group_writable_file")
		content := []byte("test content for group writable file")

		// Create test file with group writable permissions (0o664)
		require.NoError(t, os.WriteFile(filePath, content, 0o664), "Failed to create test file")

		// Test that SafeReadFile now succeeds with the new read-specific validation
		result, err := SafeReadFile(mustResolvedPath(t, filePath))
		assert.NoError(t, err, "SafeReadFile should succeed with group writable file using new read permissions")
		assert.Equal(t, content, result, "File content should match")
	})

	t.Run("SafeReadFile should still fail with world writable file", func(t *testing.T) {
		// Create temporary file with world writable permissions
		tmpDir := tu.SafeTempDir(t)
		filePath := filepath.Join(tmpDir, "world_writable_file")
		content := []byte("test content for world writable file")

		// Create test file with world writable permissions (0o666)
		require.NoError(t, os.WriteFile(filePath, content, 0o666), "Failed to create test file")
		require.NoError(t, os.Chmod(filePath, 0o666), "Failed to set world writable permissions")

		// Test that SafeReadFile still fails with world writable files
		_, err := SafeReadFile(mustResolvedPath(t, filePath))
		assert.Error(t, err, "SafeReadFile should fail with world writable file")
		assert.ErrorIs(t, err, groupmembership.ErrFileWorldWritable)
	})
}

// TestResolvedPathModeEnforcement verifies that passing a ResolvedPath created with
// NewResolvedPath (resolveModeFull) to write functions returns ErrInvalidFilePath.
func TestResolvedPathModeEnforcement(t *testing.T) {
	tempDir := tu.SafeTempDir(t)

	// Create an existing file so that NewResolvedPath succeeds.
	existingFile := filepath.Join(tempDir, "existing.txt")
	require.NoError(t, os.WriteFile(existingFile, []byte("data"), 0o644))

	fullRP, err := common.NewResolvedPath(existingFile)
	require.NoError(t, err, "NewResolvedPath should succeed for existing file")

	t.Run("SafeWriteFileOverwrite rejects resolveModeFull", func(t *testing.T) {
		err := SafeWriteFileOverwrite(fullRP, []byte("new"), 0o644)
		assert.ErrorIs(t, err, ErrInvalidFilePath,
			"SafeWriteFileOverwrite must reject ResolvedPath created with NewResolvedPath")
	})
}

// TestValidateOpenAtName covers the guard every directory-fd-anchored operation
// depends on. Anything but a single component either escapes the directory the
// caller checked or, when absolute, makes the *at call ignore the fd entirely.
func TestValidateOpenAtName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "plain name", input: "file.txt"},
		{name: "dotfile", input: ".safefileio-move-abc"},
		{name: "empty", input: "", wantErr: true},
		{name: "dot", input: ".", wantErr: true},
		{name: "dotdot", input: "..", wantErr: true},
		{name: "root", input: "/", wantErr: true},
		{name: "absolute path", input: "/etc/passwd", wantErr: true},
		{name: "relative path", input: "sub/file.txt", wantErr: true},
		{name: "parent traversal", input: "../file.txt", wantErr: true},
		{name: "trailing separator", input: "sub/", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOpenAtName(tt.input)
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrInvalidFilePath)
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestVerifySameFileAt covers the check that stands between a name and the
// operation about to be applied to it -- on Linux the removal of the source
// after a move, and on other platforms the rename itself, which is the only
// thing keeping a substituted source from being moved there.
func TestVerifySameFileAt(t *testing.T) {
	setup := func(t *testing.T) (dirFd int, file File, dir string) {
		t.Helper()
		dir = tu.SafeTempDir(t)
		path := filepath.Join(dir, "target.txt")
		require.NoError(t, os.WriteFile(path, []byte("content"), 0o600))

		fs := NewFileSystem(FileSystemConfig{})
		file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
		require.NoError(t, err)
		t.Cleanup(func() { _ = file.Close() })

		return dirFdForTest(t, fs, dir), file, dir
	}

	t.Run("accepts the entry the fd was opened from", func(t *testing.T) {
		dirFd, file, _ := setup(t)
		assert.NoError(t, verifySameFileAt(file, dirFd, "target.txt"))
	})

	t.Run("rejects an entry replaced by another file", func(t *testing.T) {
		dirFd, file, dir := setup(t)
		path := filepath.Join(dir, "target.txt")
		require.NoError(t, os.Remove(path))
		require.NoError(t, os.WriteFile(path, []byte("replacement"), 0o600))

		assert.ErrorIs(t, verifySameFileAt(file, dirFd, "target.txt"), ErrSourceIdentityMismatch)
	})

	t.Run("rejects an entry replaced by a symlink to it", func(t *testing.T) {
		dirFd, file, dir := setup(t)
		path := filepath.Join(dir, "target.txt")
		moved := filepath.Join(dir, "moved.txt")
		require.NoError(t, os.Rename(path, moved))
		// The symlink resolves to the very inode the fd holds, so only a stat
		// that refuses to follow it reports a mismatch.
		require.NoError(t, os.Symlink(moved, path))

		assert.ErrorIs(t, verifySameFileAt(file, dirFd, "target.txt"), ErrSourceIdentityMismatch)
	})

	t.Run("rejects an entry that is not a plain name", func(t *testing.T) {
		dirFd, file, _ := setup(t)
		assert.ErrorIs(t, verifySameFileAt(file, dirFd, "sub/target.txt"), ErrInvalidFilePath)
	})

	t.Run("reports a missing entry as an error, not a match", func(t *testing.T) {
		dirFd, file, dir := setup(t)
		require.NoError(t, os.Remove(filepath.Join(dir, "target.txt")))

		err := verifySameFileAt(file, dirFd, "target.txt")
		require.Error(t, err)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})
}

// allowOSManagedSymlink adds path to the OS-managed symlink allowlist for the
// duration of the test.
//
// Without it these tests could not run anywhere but macOS:
// common.IsAllowedOSManagedSymlink returns false unconditionally on every other
// platform, so the branch that follows a symlink -- and with it the obligation
// on ensureDirNoSymlinks to report where it led -- would be unreachable on the
// platform this project targets.
func allowOSManagedSymlink(t *testing.T, path string) {
	t.Helper()
	orig := isAllowedOSManagedSymlinkOverride
	isAllowedOSManagedSymlinkOverride = func(candidate string) bool {
		return candidate == path || orig(candidate)
	}
	t.Cleanup(func() { isAllowedOSManagedSymlinkOverride = orig })
}

// symlinkedDirFixture builds <tmp>/real/sub, plus <tmp>/link -> <tmp>/real, and
// returns the link and the real directory.
func symlinkedDirFixture(t *testing.T) (linkDir, realDir string) {
	t.Helper()
	tmpDir := tu.SafeTempDir(t)
	realDir = filepath.Join(tmpDir, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(realDir, "sub"), 0o750))
	linkDir = filepath.Join(tmpDir, "link")
	require.NoError(t, os.Symlink(realDir, linkDir))
	return linkDir, realDir
}

// TestEnsureDirNoSymlinks_ReturnsResolvedPath pins the return value itself, not
// just the accept/reject verdict: a caller that opens the path it was given
// rather than the one that comes back would fail on exactly the layout the
// allowlist exists to support. The symlink here is mid-path, which the
// fallback-route test below cannot reach -- it can only place one at the leaf.
//
// Rejection of a symlink that is not on the allowlist is covered by
// TestEnsureParentDirsNoSymlinks.
func TestEnsureDirNoSymlinks_ReturnsResolvedPath(t *testing.T) {
	linkDir, realDir := symlinkedDirFixture(t)
	allowOSManagedSymlink(t, linkDir)

	resolved, err := ensureDirNoSymlinks(filepath.Join(linkDir, "sub"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(realDir, "sub"), resolved,
		"the followed symlink must be replaced by its target in the returned path")
}

// TestOpenDirNoSymlinksFallback_OpensResolvedPath covers the consequence of the
// above for the fallback route: when the directory being opened is itself an
// allowlisted symlink, opening the path as given would fail, so the resolved
// path is what must reach open(2).
func TestOpenDirNoSymlinksFallback_OpensResolvedPath(t *testing.T) {
	linkDir, realDir := symlinkedDirFixture(t)
	allowOSManagedSymlink(t, linkDir)

	// The premise: opening linkDir as given, the way a naive implementation
	// would, fails. Without this the test could pass on an implementation that
	// never resolved anything.
	// Which errno that is depends on the platform -- Linux reports ENOTDIR when
	// O_DIRECTORY and O_NOFOLLOW meet a symlink, others report ELOOP -- so only
	// the failure itself is asserted.
	rawFd, rawErr := unix.Open(linkDir, unix.O_DIRECTORY|unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if rawErr == nil {
		_ = unix.Close(rawFd)
		t.Fatal("opening the unresolved path was expected to fail with O_NOFOLLOW")
	}

	fd, err := openDirNoSymlinksFallback(linkDir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = unix.Close(fd) })

	// The fd is the target directory, not merely some directory.
	var fdStat, realStat unix.Stat_t
	require.NoError(t, unix.Fstat(fd, &fdStat))
	require.NoError(t, unix.Stat(realDir, &realStat))
	assert.Equal(t, realStat.Ino, fdStat.Ino)
	assert.Equal(t, realStat.Dev, fdStat.Dev)
}

// stubEnsureDirAfterOpen replaces the second directory check for the duration of
// the test. Only that second call goes through the override, so the walk and the
// open that precede it still run unmodified against the real path.
func stubEnsureDirAfterOpen(t *testing.T, stub func(dir string) (string, error)) {
	t.Helper()
	orig := ensureDirAfterOpenOverride
	t.Cleanup(func() { ensureDirAfterOpenOverride = orig })
	ensureDirAfterOpenOverride = stub
}

// TestOpenDirNoSymlinksFallback_AbandonsOpenWhenDirChanges covers the second
// phase of the fallback directory open, which is what keeps this route as
// symlink-safe as the file open next to it: the walk and the open are separate
// system calls, and unix.Open refuses a symlink only at the leaf, so a component
// replaced in between is invisible to everything else here.
//
// The two subtests are the two ways that shows up once the re-walk itself
// accepts the path: it resolves somewhere else, or it is perfectly happy because
// the directory was replaced by another real directory -- which only the inode
// comparison can see. The third way, the re-walk refusing the path outright, is
// covered by TestOpenDirNoSymlinksFallback_ClosesFDWhenPostCheckFails.
func TestOpenDirNoSymlinksFallback_AbandonsOpenWhenDirChanges(t *testing.T) {
	t.Run("second walk resolves elsewhere", func(t *testing.T) {
		tmpDir := tu.SafeTempDir(t)
		dir := filepath.Join(tmpDir, "target")
		elsewhere := filepath.Join(tmpDir, "elsewhere")
		require.NoError(t, os.Mkdir(dir, 0o750))
		require.NoError(t, os.Mkdir(elsewhere, 0o750))

		// A component of dir turned into an allowlisted symlink after the first
		// walk: the path is still accepted, but it now leads somewhere else.
		stubEnsureDirAfterOpen(t, func(string) (string, error) { return elsewhere, nil })

		fd, err := openDirNoSymlinksFallback(dir)
		require.ErrorIs(t, err, ErrDirChangedDuringOpen)
		assert.Equal(t, -1, fd)
	})

	t.Run("directory replaced by another directory", func(t *testing.T) {
		tmpDir := tu.SafeTempDir(t)
		dir := filepath.Join(tmpDir, "target")
		attacker := filepath.Join(tmpDir, "attacker")
		require.NoError(t, os.Mkdir(dir, 0o750))
		require.NoError(t, os.Mkdir(attacker, 0o750))

		// Swap a different real directory into the path between the open and the
		// check, then run the genuine walk over it. No symlink is involved at any
		// point, so the walk has nothing to report and the path it returns is
		// unchanged: the mismatch is only visible in the inode.
		var walkResult string
		var walkErr error
		stubEnsureDirAfterOpen(t, func(d string) (string, error) {
			require.NoError(t, os.Rename(dir, filepath.Join(tmpDir, "displaced")))
			require.NoError(t, os.Rename(attacker, dir))
			walkResult, walkErr = ensureDirNoSymlinks(d)
			return walkResult, walkErr
		})

		fd, err := openDirNoSymlinksFallback(dir)
		require.ErrorIs(t, err, ErrDirChangedDuringOpen)
		assert.Equal(t, -1, fd)

		// The negative control for the assertion above: the walk alone accepted
		// the swapped-in directory, so the failure came from the inode comparison
		// and from nothing else.
		require.NoError(t, walkErr)
		assert.Equal(t, dir, walkResult)
	})
}

// writeFileWithPerm creates a file whose permissions are exactly perm. Passing
// perm to os.WriteFile is not enough: the process umask removes bits from it,
// and the modes these tests turn on -- group and world write -- are the ones a
// default umask removes.
func writeFileWithPerm(t *testing.T, path string, content []byte, perm os.FileMode) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, content, 0o600))
	require.NoError(t, os.Chmod(path, perm))
}

// TestAtomicMoveFile_ValidatesSourceBeforeChmod pins the order of the source
// validation and the fchmod. With the fchmod first, a world-writable source is
// narrowed to requiredPerm and then passes a check that would have refused what
// the caller actually pointed at; a refused move must instead leave the source
// as it was found.
func TestAtomicMoveFile_ValidatesSourceBeforeChmod(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	content := []byte("world-writable content")
	writeFileWithPerm(t, srcPath, content, 0o666)

	fs := NewFileSystem(FileSystemConfig{})
	err := fs.AtomicMoveFile(srcPath, dstPath, 0o600)
	require.ErrorIs(t, err, ErrInvalidFilePermissions)

	info, err := os.Lstat(srcPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o666), info.Mode().Perm(),
		"a refused move must not have narrowed the source's permissions first")

	got, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, content, got, "the source must still be where it was")
	assert.NoFileExists(t, dstPath)
}

// gidOutsideOwnGroups returns a gid the current user does not belong to, or
// reports that none was found.
func gidOutsideOwnGroups(t *testing.T) (gid int, ok bool) {
	t.Helper()
	own, err := os.Getgroups()
	require.NoError(t, err)
	owned := make(map[int]struct{}, len(own)+1)
	owned[os.Getgid()] = struct{}{}
	for _, g := range own {
		owned[g] = struct{}{}
	}
	// Low gids are the ones a system is sure to have (root, daemon, bin, ...).
	for candidate := range 64 {
		if _, mine := owned[candidate]; !mine {
			return candidate, true
		}
	}
	return 0, false
}

// TestAtomicMoveFile_RejectsUnsafeSourcePermissions covers the inputs the
// reordered source validation now refuses outright, where before the fchmod
// narrowed them into acceptance.
//
// The read policy is not symmetric with the write policy: a group-writable
// source is refused only when the caller is not in that group, which is why
// there is no world-and-group table here but two separate cases.
//
// The mode-exceeds-maximum rule that the design document lists as a third case
// cannot be reached through a real file: CanCurrentUserSafelyReadFile masks the
// mode with 0o7777, and Go encodes setuid, setgid and sticky above that mask,
// so the only bit a file can carry that MaxAllowedReadPerms (0o6775) disallows
// is 0o002 -- which the world-writable rule has already refused. It is
// reachable only through ValidateRequestedPermissions, which takes a mode from
// the caller rather than from a file.
func TestAtomicMoveFile_RejectsUnsafeSourcePermissions(t *testing.T) {
	t.Run("world_writable", func(t *testing.T) {
		dir := tu.SafeTempDir(t)
		srcPath := filepath.Join(dir, "src.txt")
		dstPath := filepath.Join(dir, "dst.txt")
		writeFileWithPerm(t, srcPath, []byte("content"), 0o666)

		fs := NewFileSystem(FileSystemConfig{})
		// Even a requiredPerm that is itself safe does not make the source
		// acceptable; the source is judged as it stands.
		err := fs.AtomicMoveFile(srcPath, dstPath, 0o600)
		require.ErrorIs(t, err, ErrInvalidFilePermissions)
		require.ErrorIs(t, err, groupmembership.ErrFileWorldWritable)
		assert.NoFileExists(t, dstPath)
	})

	t.Run("group_writable_non_member", func(t *testing.T) {
		gid, ok := gidOutsideOwnGroups(t)
		if !ok {
			t.Skip("no gid outside the current user's groups was found, so a non-member group source cannot be built")
		}
		dir := tu.SafeTempDir(t)
		srcPath := filepath.Join(dir, "src.txt")
		dstPath := filepath.Join(dir, "dst.txt")
		writeFileWithPerm(t, srcPath, []byte("content"), 0o660)
		if err := os.Chown(srcPath, -1, gid); err != nil {
			t.Skipf("cannot give the source a group the caller is not in: %v", err)
		}

		fs := NewFileSystem(FileSystemConfig{})
		err := fs.AtomicMoveFile(srcPath, dstPath, 0o600)
		require.ErrorIs(t, err, ErrInvalidFilePermissions)
		require.ErrorIs(t, err, groupmembership.ErrGroupWritableNonMember)
		assert.NoFileExists(t, dstPath)
	})
}

// TestAtomicMoveFile_SafeSourceStillMoves is the other half of the reordering:
// the sources the read policy accepts must move exactly as they did before,
// with requiredPerm applied at the destination.
func TestAtomicMoveFile_SafeSourceStillMoves(t *testing.T) {
	sources := []struct {
		name string
		perm os.FileMode
	}{
		{name: "owner_only", perm: 0o600},
		// Group-writable, in a group the caller belongs to: the read policy
		// admits this, and the reordering must not turn it into a refusal.
		{name: "group_writable_member", perm: 0o660},
	}

	const requiredPerm = os.FileMode(0o644)
	for _, src := range sources {
		t.Run(src.name, func(t *testing.T) {
			dir := tu.SafeTempDir(t)
			srcPath := filepath.Join(dir, "src.txt")
			dstPath := filepath.Join(dir, "dst.txt")
			content := []byte("moved content")
			writeFileWithPerm(t, srcPath, content, src.perm)

			fs := NewFileSystem(FileSystemConfig{})
			require.NoError(t, fs.AtomicMoveFile(srcPath, dstPath, requiredPerm))

			got, err := os.ReadFile(dstPath)
			require.NoError(t, err)
			assert.Equal(t, content, got)

			info, err := os.Lstat(dstPath)
			require.NoError(t, err)
			assert.Equal(t, requiredPerm, info.Mode().Perm())
			assert.NoFileExists(t, srcPath)
		})
	}
}

// openSourceForMove opens name under dir the way atomicMoveFileCore does, and
// returns the directory fd and the open file for a direct moveOpenFileCore
// call.
func openSourceForMove(t *testing.T, fs FileSystem, dir, name string) (dirFd int, file File) {
	t.Helper()
	osfs, ok := fs.(*osFS)
	require.True(t, ok, "NewFileSystem must return *osFS")
	dirFd = dirFdForTest(t, fs, dir)
	opened, err := osfs.openFileAt(dirFd, name, os.O_RDONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = opened.Close() })
	return dirFd, opened
}

// TestMoveOpenFileCore_RejectsRequiredPermBeforeRename covers the gap between
// the two permission policies: ValidateRequestedPermissions lets requiredPerm
// through at the entry, and CanCurrentUserSafelyWriteFile then refuses the file
// it produces. That used to be found only after the rename, leaving the
// destination replaced and an error returned; it must now be found before.
func TestMoveOpenFileCore_RejectsRequiredPermBeforeRename(t *testing.T) {
	// A mode with no owner-write bit is the one refusal reachable without
	// arranging group memberships: the write policy requires the caller to be
	// able to write the file it is about to accept.
	t.Run("required_perm_not_writable_by_owner", func(t *testing.T) {
		assertRejectedBeforeRename(t, 0o444, func(*testing.T, string) {})
	})

	// The case the design document is about: a group-writable requiredPerm in a
	// group with more than one member. It needs such a group to exist.
	t.Run("required_perm_group_writable_in_shared_group", func(t *testing.T) {
		gid, ok := sharedGroupOfCurrentUser(t)
		if !ok {
			t.Skip("the caller belongs to no group with another member, so a shared-group destination cannot be built")
		}
		assertRejectedBeforeRename(t, 0o660, func(t *testing.T, srcPath string) {
			t.Helper()
			if err := os.Chown(srcPath, -1, gid); err != nil {
				t.Skipf("cannot put the source in the shared group: %v", err)
			}
		})
	})
}

// assertRejectedBeforeRename runs moveOpenFileCore with requiredPerm over a
// source prepared by prepare, and asserts that it refused before touching the
// destination.
func assertRejectedBeforeRename(t *testing.T, requiredPerm os.FileMode, prepare func(*testing.T, string)) {
	t.Helper()
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0o600))
	prepare(t, srcPath)

	fs := NewFileSystem(FileSystemConfig{})
	gm := fs.GetGroupMembership()
	// Without this, the test would not say which of the two policies refused:
	// a requiredPerm the entry check rejects never reaches the one under test.
	require.NoError(t, gm.ValidateRequestedPermissions(requiredPerm, groupmembership.FileOpWrite),
		"requiredPerm must pass the entry check, or this test proves nothing about the later one")

	dirFd, file := openSourceForMove(t, fs, dir, "src.txt")
	err := moveOpenFileCore(file, dirFd, "src.txt", dirFd, "dst.txt", dstPath, requiredPerm, gm)
	require.ErrorIs(t, err, ErrInvalidFilePermissions)
	assert.NotErrorIs(t, err, ErrDestinationCommitted, "the refusal must come before the rename")
	assert.NoFileExists(t, dstPath, "nothing may have been moved")
}

// sharedGroupOfCurrentUser returns a gid the current user belongs to that has
// at least one other member, which is what CanCurrentUserSafelyWriteFile
// refuses a group-writable file for.
func sharedGroupOfCurrentUser(t *testing.T) (gid int, ok bool) {
	t.Helper()
	groups, err := os.Getgroups()
	require.NoError(t, err)
	current, err := user.Current()
	require.NoError(t, err)

	gm := groupmembership.New()
	for _, candidate := range groups {
		// #nosec G115 - a gid from os.Getgroups is a valid gid
		members, err := gm.GetGroupMembers(uint32(candidate))
		if err != nil {
			continue
		}
		if len(members) != 1 || members[0] != current.Username {
			return candidate, true
		}
	}
	return 0, false
}

// TestMoveOpenFileCore_PostMoveIdentityFailureIsDestinationCommitted covers the
// one outcome that leaves the destination changed and still returns an error.
// The caller has to be able to tell it apart from a move that did nothing, and
// an operator has to find it in the log, since the previous content is gone
// either way.
func TestMoveOpenFileCore_PostMoveIdentityFailureIsDestinationCommitted(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	content := []byte("moved content")
	require.NoError(t, os.WriteFile(srcPath, content, 0o600))
	require.NoError(t, os.WriteFile(dstPath, []byte("previous content"), 0o600))

	recorder := captureWarnings(t)
	errIdentity := errors.New("simulated post-move identity failure")
	orig := verifyMovedFileOverride
	t.Cleanup(func() { verifyMovedFileOverride = orig })
	verifyMovedFileOverride = func(File, int, string) error { return errIdentity }

	fs := NewFileSystem(FileSystemConfig{})
	dirFd, file := openSourceForMove(t, fs, dir, "src.txt")
	err := moveOpenFileCore(file, dirFd, "src.txt", dirFd, "dst.txt", dstPath, 0o600, fs.GetGroupMembership())

	require.ErrorIs(t, err, ErrDestinationCommitted)
	require.ErrorIs(t, err, errIdentity)

	got, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, got, "the rename went through, so the destination holds the moved content")

	record := recorder.RequireRecord(t, slog.LevelWarn, destinationCommittedMsg)
	record.AssertAttrs(t, map[string]any{"destination": dstPath})
}

// TestAtomicMoveFile_WritesUnderOSManagedSymlink is the same obligation seen
// from the public API, on a real OS-managed symlink rather than an allowlisted
// stand-in. It runs only where /tmp is one, which is decided by asking the
// allowlist rather than by testing runtime.GOOS.
func TestAtomicMoveFile_WritesUnderOSManagedSymlink(t *testing.T) {
	if !common.IsAllowedOSManagedSymlink("/tmp") {
		t.Skip("/tmp is not an OS-managed symlink here, so this layout does not exist")
	}

	srcDir := tu.SafeTempDir(t)
	srcPath := filepath.Join(srcDir, "src.txt")
	content := []byte("moved under an OS-managed symlink")
	require.NoError(t, os.WriteFile(srcPath, content, 0o600))

	// The destination is directly under /tmp, which t.TempDir cannot produce:
	// it hands back an already-resolved path, and it is the unresolved parent
	// that this test is about. The name is unique per run and removed
	// afterwards, /tmp being shared.
	name, err := randomTempName(".safefileio-test-")
	require.NoError(t, err)
	dstPath := filepath.Join("/tmp", name)
	t.Cleanup(func() { _ = os.Remove(dstPath) })

	fs := NewFileSystem(FileSystemConfig{})
	require.NoError(t, fs.AtomicMoveFile(srcPath, dstPath, 0o600))

	got, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestEnsureParentDirsNoSymlinks exercises the symlink policy in
// ensureParentDirsNoSymlinks: user-owned symlinks must be rejected, while
// OS-managed symlinks on the explicit allowlist (e.g. /tmp -> /private/tmp)
// must be allowed after target verification.
func TestEnsureParentDirsNoSymlinks(t *testing.T) {
	t.Run("rejects user-owned symlink in parent", func(t *testing.T) {
		// Create:  <tmpDir>/real/   (real directory)
		//          <tmpDir>/link -> <tmpDir>/real  (user-owned symlink)
		// Target path: <tmpDir>/link/file.txt
		// ensureParentDirsNoSymlinks should reject the symlink component.
		tmpDir := tu.SafeTempDir(t)
		realDir := filepath.Join(tmpDir, "real")
		require.NoError(t, os.Mkdir(realDir, 0o750))
		linkDir := filepath.Join(tmpDir, "link")
		require.NoError(t, os.Symlink(realDir, linkDir))

		targetPath := filepath.Join(linkDir, "file.txt")
		err := ensureParentDirsNoSymlinks(targetPath)
		assert.ErrorIs(t, err, ErrIsSymlink, "user-owned symlink in parent must be rejected")
	})

	t.Run("allows regular directory hierarchy", func(t *testing.T) {
		tmpDir := tu.SafeTempDir(t)
		subDir := filepath.Join(tmpDir, "sub")
		require.NoError(t, os.Mkdir(subDir, 0o750))

		targetPath := filepath.Join(subDir, "file.txt")
		err := ensureParentDirsNoSymlinks(targetPath)
		assert.NoError(t, err, "plain directory hierarchy must be accepted")
	})

	t.Run("os.TempDir path is accepted (may traverse OS-managed symlinks)", func(t *testing.T) {
		// os.TempDir() on macOS resolves through /tmp -> /private/tmp.
		// ensureParentDirsNoSymlinks must accept that root-owned symlink so
		// that writing to the system temp directory works correctly.
		subDir := tu.SafeTempDir(t)

		targetPath := filepath.Join(subDir, "file.txt")
		err := ensureParentDirsNoSymlinks(targetPath)
		assert.NoError(t, err, "path under os.TempDir must be accepted")
	})
}

// TestSafeReadFile_AcceptsBothModes verifies that SafeReadFile accepts ResolvedPath values
// created with either NewResolvedPath or NewResolvedPathParentOnly.
func TestSafeReadFile_AcceptsBothModes(t *testing.T) {
	tempDir := tu.SafeTempDir(t)
	filePath := filepath.Join(tempDir, "readable.txt")
	content := []byte("hello")
	require.NoError(t, os.WriteFile(filePath, content, 0o644))

	t.Run("NewResolvedPath accepted by SafeReadFile", func(t *testing.T) {
		rp, err := common.NewResolvedPath(filePath)
		require.NoError(t, err)
		got, err := SafeReadFile(rp)
		assert.NoError(t, err)
		assert.Equal(t, content, got)
	})

	t.Run("NewResolvedPathParentOnly accepted by SafeReadFile", func(t *testing.T) {
		rp, err := common.NewResolvedPathParentOnly(filePath)
		require.NoError(t, err)
		got, err := SafeReadFile(rp)
		assert.NoError(t, err)
		assert.Equal(t, content, got)
	})
}

// TestGroupMembershipFollowsProcessPermissionCheckUIDPolicy verifies that the
// GroupMembership instances created by NewFileSystem and held by the
// defaultFS package variable carry no instance-level permission check UID
// policy and therefore follow the process-wide default policy. This
// mutates the process-wide default policy, so it must not call t.Parallel()
// (see the package-level restriction documented in safe_file_linux.go).
func TestGroupMembershipFollowsProcessPermissionCheckUIDPolicy(t *testing.T) {
	groupMemberships := map[string]*groupmembership.GroupMembership{
		"NewFileSystem": NewFileSystem(FileSystemConfig{}).GetGroupMembership(),
		"defaultFS":     defaultFS.GetGroupMembership(),
	}

	for name, gm := range groupMemberships {
		t.Run(name, func(t *testing.T) {
			t.Run("follows process default RealUIDOnly when unset", func(t *testing.T) {
				t.Cleanup(groupmembership.SwapProcessPermissionCheckUIDPolicy(groupmembership.PolicyUnset))

				assert.Equal(t, groupmembership.RealUIDOnly, gm.EffectivePermissionCheckUIDPolicy())
			})

			t.Run("follows process default SudoUIDAware", func(t *testing.T) {
				t.Cleanup(groupmembership.SwapProcessPermissionCheckUIDPolicy(groupmembership.SudoUIDAware))

				assert.Equal(t, groupmembership.SudoUIDAware, gm.EffectivePermissionCheckUIDPolicy())
			})
		})
	}
}
