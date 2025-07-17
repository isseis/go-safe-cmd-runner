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
				assert.Equal(t, tc.hashDir, manager.hashDir)
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
