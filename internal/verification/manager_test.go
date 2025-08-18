package verification

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewManager(t *testing.T) {
	testCases := []struct {
		name        string
		hashDir     string
		expectError bool
		expectedErr error
	}{
		{
			name:        "valid hash directory",
			hashDir:     "/usr/local/etc/go-safe-cmd-runner/hashes",
			expectError: false,
		},
		{
			name:        "invalid hash directory",
			hashDir:     "", // empty directory
			expectError: true,
			expectedErr: ErrHashDirectoryEmpty,
		},
		{
			name:        "relative hash directory",
			hashDir:     "relative/path/hashes",
			expectError: false, // NewManager doesn't validate path format, only emptiness
		},
		{
			name:        "dot relative hash directory",
			hashDir:     "./hashes",
			expectError: false, // NewManager doesn't validate path format, only emptiness
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockFS := common.NewMockFileSystem()
			manager, err := NewManagerWithOpts(tc.hashDir, withFS(mockFS), withFileValidatorDisabled())

			if tc.expectError {
				require.Error(t, err)
				assert.Nil(t, manager)
				if tc.expectedErr != nil {
					assert.ErrorIs(t, err, tc.expectedErr)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, manager)
				// The manager may normalize the path, so we don't assert exact equality for relative paths
				if tc.hashDir == "./hashes" {
					// "./hashes" gets normalized to "hashes"
					assert.Equal(t, "hashes", manager.hashDir)
				} else {
					assert.Equal(t, tc.hashDir, manager.hashDir)
				}
				assert.Equal(t, mockFS, manager.fs)
			}
		})
	}
}

// TestManager_ValidateHashDirectory_NoSecurityValidator tests that hash directory validation fails when no security validator is set

func TestManager_ValidateHashDirectory_NoSecurityValidator(t *testing.T) {
	manager := &Manager{
		hashDir:  "/usr/local/etc/go-safe-cmd-runner/hashes",
		security: nil, // No security validator
	}

	err := manager.ValidateHashDirectory()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSecurityValidatorNotInitialized)
}

func TestManager_ValidateHashDirectory_RelativePath(t *testing.T) {
	testCases := []struct {
		name        string
		hashDir     string
		expectError bool
	}{
		{
			name:        "absolute path should succeed (if security validator passes)",
			hashDir:     "/usr/local/etc/go-safe-cmd-runner/hashes",
			expectError: false,
		},
		{
			name:        "relative path should be rejected by security validator",
			hashDir:     "relative/path/hashes",
			expectError: true, // The security validator rejects relative paths
		},
		{
			name:        "dot relative path should be rejected by security validator",
			hashDir:     "./hashes",
			expectError: true, // The security validator rejects relative paths
		},
		{
			name:        "double dot relative path should be rejected by security validator",
			hashDir:     "../hashes",
			expectError: true, // The security validator rejects relative paths
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a mock filesystem with necessary directory structure
			mockFS := common.NewMockFileSystem()

			// Create the directory in the mock filesystem to satisfy the security validator
			if tc.hashDir != "" {
				// Create the target directory
				mockFS.AddDir(tc.hashDir, 0o755)

				// For absolute paths, also create parent directories to ensure proper path validation
				if tc.hashDir == "/usr/local/etc/go-safe-cmd-runner/hashes" {
					// Create parent directories
					mockFS.AddDir("/", 0o755)
					mockFS.AddDir("/usr", 0o755)
					mockFS.AddDir("/usr/local", 0o755)
					mockFS.AddDir("/usr/local/etc", 0o755)
					mockFS.AddDir("/usr/local/etc/go-safe-cmd-runner", 0o755)
				}
			}

			manager, err := NewManagerWithOpts(tc.hashDir, withFS(mockFS), withFileValidatorDisabled())
			require.NoError(t, err)

			// The ValidateHashDirectory method delegates to the security validator
			// Since we're using a mock security validator that checks the filesystem,
			// and we've added the directory to the mock filesystem, this should pass
			err = manager.ValidateHashDirectory()

			if tc.expectError {
				assert.Error(t, err)
			} else {
				// With proper mock filesystem setup, validation should succeed
				assert.NoError(t, err, "expected no error for valid hash directory")
			}
		})
	}
}

func TestManager_VerifyConfigFile_Integration(t *testing.T) {
	// This test requires more complex setup with mock filevalidator and security validator
	// For now, we'll skip this test as it would require significant mocking infrastructure
	t.Skip("Integration test requires complex mocking setup")
}

// Test error wrapping in VerifyConfigFile
func TestManager_VerifyConfigFile_ErrorWrapping(t *testing.T) {
	// Create manager with mocked components that will fail
	mockFS := common.NewMockFileSystem()
	manager := &Manager{
		hashDir: "/usr/local/etc/go-safe-cmd-runner/hashes",
		fs:      mockFS,
		// Leave validator and security nil to trigger errors
	}

	err := manager.VerifyConfigFile("/path/to/config.toml")
	assert.Error(t, err)

	// Check that error is properly wrapped
	var verificationErr *Error
	assert.True(t, errors.As(err, &verificationErr))
	assert.Equal(t, "ValidateHashDirectory", verificationErr.Op)
}
