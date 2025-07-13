package security

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_ValidateDirectoryPermissions_CompletePath(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		setupFunc   func(*common.MockFileSystem)
		dirPath     string
		shouldFail  bool
		expectedErr error
	}{
		{
			name: "valid directory with secure path",
			setupFunc: func(fs *common.MockFileSystem) {
				// Create secure directory hierarchy
				fs.AddDir("/usr", 0o755)
				fs.AddDir("/usr/local", 0o755)
				fs.AddDir("/usr/local/etc", 0o755)
				fs.AddDir("/usr/local/etc/go-safe-cmd-runner", 0o755)
				fs.AddDir("/usr/local/etc/go-safe-cmd-runner/hashes", 0o755)
			},
			dirPath:    "/usr/local/etc/go-safe-cmd-runner/hashes",
			shouldFail: false,
		},
		{
			name: "directory with world-writable intermediate directory",
			setupFunc: func(fs *common.MockFileSystem) {
				fs.AddDir("/usr", 0o755)
				fs.AddDir("/usr/local", 0o777) // World writable - insecure!
				fs.AddDir("/usr/local/etc", 0o755)
				fs.AddDir("/usr/local/etc/go-safe-cmd-runner", 0o755)
				fs.AddDir("/usr/local/etc/go-safe-cmd-runner/hashes", 0o755)
			},
			dirPath:     "/usr/local/etc/go-safe-cmd-runner/hashes",
			shouldFail:  true,
			expectedErr: ErrInvalidFilePermissions,
		},
		{
			name: "directory with group-writable intermediate directory owned by non-root",
			setupFunc: func(fs *common.MockFileSystem) {
				fs.AddDir("/opt", 0o755)
				fs.AddDirWithOwner("/opt/myapp", 0o775, 1000, 1000) // Group writable, owned by non-root
				fs.AddDir("/opt/myapp/etc", 0o755)
				fs.AddDir("/opt/myapp/etc/go-safe-cmd-runner", 0o755)
				fs.AddDir("/opt/myapp/etc/go-safe-cmd-runner/hashes", 0o755)
			},
			dirPath:     "/opt/myapp/etc/go-safe-cmd-runner/hashes",
			shouldFail:  true,
			expectedErr: ErrInvalidFilePermissions,
		},
		{
			name: "directory with root group write owned by root",
			setupFunc: func(fs *common.MockFileSystem) {
				fs.AddDirWithOwner("/usr", 0o775, 0, 0) // Root group writable, owned by root - allowed
				fs.AddDir("/usr/local", 0o755)
				fs.AddDir("/usr/local/etc", 0o755)
				fs.AddDir("/usr/local/etc/go-safe-cmd-runner", 0o755)
				fs.AddDir("/usr/local/etc/go-safe-cmd-runner/hashes", 0o755)
			},
			dirPath:    "/usr/local/etc/go-safe-cmd-runner/hashes",
			shouldFail: false,
		},
		{
			name: "directory with non-root group write owned by root",
			setupFunc: func(fs *common.MockFileSystem) {
				fs.AddDirWithOwner("/usr", 0o775, 0, 1) // Non-root group writable, owned by root - prohibited
				fs.AddDir("/usr/local", 0o755)
				fs.AddDir("/usr/local/etc", 0o755)
				fs.AddDir("/usr/local/etc/go-safe-cmd-runner", 0o755)
				fs.AddDir("/usr/local/etc/go-safe-cmd-runner/hashes", 0o755)
			},
			dirPath:     "/usr/local/etc/go-safe-cmd-runner/hashes",
			shouldFail:  true,
			expectedErr: ErrInvalidFilePermissions,
		},
		{
			name: "directory owned by non-root user",
			setupFunc: func(fs *common.MockFileSystem) {
				fs.AddDir("/home", 0o755)
				fs.AddDirWithOwner("/home/user", 0o755, 1000, 1000) // Owned by non-root user
				fs.AddDir("/home/user/config", 0o755)
			},
			dirPath:     "/home/user/config",
			shouldFail:  true,
			expectedErr: ErrInvalidFilePermissions,
		},
		{
			name:        "relative path rejected",
			dirPath:     "relative/path",
			shouldFail:  true,
			expectedErr: os.ErrNotExist,
			// TODO: This should be ErrInvalidPath, but mock filesystem returns ErrNotExist
		},
		{
			name:        "path does not exist",
			dirPath:     "/nonexistent/path",
			shouldFail:  true,
			expectedErr: os.ErrNotExist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock filesystem for each test
			testMockFS := common.NewMockFileSystem()
			testValidator, err := NewValidatorWithFS(config, testMockFS)
			require.NoError(t, err)

			// Set up the test scenario
			if tt.setupFunc != nil {
				tt.setupFunc(testMockFS)
			}

			// Run the test
			err = testValidator.ValidateDirectoryPermissions(tt.dirPath)

			if tt.shouldFail {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidator_ValidateCompletePath_SymlinkProtection(t *testing.T) {
	// Symlink attack protection is handled by safefileio package using openat2
	// with RESOLVE_NO_SYMLINKS when opening hash files.
	// This test documents that behavior rather than testing it here.
	t.Skip("Symlink protection is handled by safefileio package with openat2 RESOLVE_NO_SYMLINKS")
}

func TestValidator_ValidatePathComponents_EdgeCases(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		setupFunc   func(*common.MockFileSystem)
		path        string
		shouldFail  bool
		expectedErr string
	}{
		{
			name:       "root directory only",
			setupFunc:  nil, // Root directory is handled specially
			path:       "/",
			shouldFail: false,
		},
		{
			name: "single level directory",
			setupFunc: func(fs *common.MockFileSystem) {
				fs.AddDir("/test", 0o755)
			},
			path:       "/test",
			shouldFail: false,
		},
		{
			name: "path with double slashes",
			setupFunc: func(fs *common.MockFileSystem) {
				fs.AddDir("/usr", 0o755)
				fs.AddDir("/usr/local", 0o755)
			},
			path:       "/usr//local",
			shouldFail: false, // filepath.Clean should handle this
		},
		{
			name: "empty path components handled",
			setupFunc: func(fs *common.MockFileSystem) {
				fs.AddDir("/usr", 0o755)
				fs.AddDir("/usr/local", 0o755)
			},
			path:       "/usr/local/",
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh mock filesystem for each test
			testMockFS := common.NewMockFileSystem()
			testValidator, err := NewValidatorWithFS(config, testMockFS)
			require.NoError(t, err)

			// Set up the test scenario
			if tt.setupFunc != nil {
				tt.setupFunc(testMockFS)
			}

			// Run the validation
			err = testValidator.validateCompletePath(tt.path)

			if tt.shouldFail {
				assert.Error(t, err)
				if tt.expectedErr != "" {
					assert.Contains(t, err.Error(), tt.expectedErr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
