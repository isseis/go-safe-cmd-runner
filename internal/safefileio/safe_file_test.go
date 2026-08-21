package safefileio

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// newFileSystemForRoute builds the FileSystem for a route. For the openat2
// route it insists that openat2 really is available, because NewFileSystem
// falls back silently when it is not (Linux 5.5 and older, non-Linux, or a
// container seccomp profile that blocks the call). Without this check a table
// claiming to cover both routes would run the fallback path twice.
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

// TestSafeOpenFile_RejectsNonPermissionModeBits verifies that a perm carrying
// any bit outside os.ModePerm is rejected with the same sentinel on both
// routes, before anything is created.
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

// TestSafeWriteFileOverwrite_RejectsNonPermissionModeBits verifies that the
// public write entry point rejects the same mode bits SafeOpenFile does.
//
// It reaches validateOpenPerm through SafeOpenFile today, and the existing
// ValidateRequestedPermissions cannot stand in for it: that check masks with
// 0o7777, so os.ModeSetuid (1<<23) is invisible to it and 0o600 is all it
// sees. This test therefore fails loudly if a later change reroutes the write
// path around SafeOpenFile without carrying the check across.
func TestSafeWriteFileOverwrite_RejectsNonPermissionModeBits(t *testing.T) {
	filePath := filepath.Join(tu.SafeTempDir(t), "target.txt")

	err := SafeWriteFileOverwrite(mustResolvedPath(t, filePath), []byte("content"), os.ModeSetuid|0o600)
	require.ErrorIs(t, err, ErrUnsupportedFileMode)
	assert.NoFileExists(t, filePath, "rejection must happen before anything is written")
}

// TestSafeOpenFile_ReadOpenPermIgnoredOnBothPaths verifies that a non-zero
// perm on an open without O_CREATE is ignored rather than rejected, and that
// both routes agree. Before this task the openat2 route failed such a call
// with EINVAL while the fallback route succeeded.
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

// TestSafeOpenFile_CreatePermUnchanged verifies that a creating open still
// applies perm as the kernel always did, i.e. reduced by the process umask.
// The umask is chosen so that it actually removes a bit of the requested
// perm; an implementation that applied perm verbatim would fail here.
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
// This corresponds to AC-14, AC-15, AC-16.
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
// created with either NewResolvedPath or NewResolvedPathParentOnly (AC-17).
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
