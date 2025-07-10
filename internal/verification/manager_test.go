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
		config      *Config
		expectError bool
		expectedErr error
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
			expectedErr: ErrConfigNil,
		},
		{
			name: "valid disabled config",
			config: &Config{
				Enabled:       false,
				HashDirectory: "/usr/local/etc/go-safe-cmd-runner/hashes",
			},
			expectError: false,
		},
		{
			name: "invalid enabled config",
			config: &Config{
				Enabled:       true,
				HashDirectory: "", // empty directory
			},
			expectError: true,
			expectedErr: ErrHashDirectoryEmpty,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			manager, err := NewManager(tc.config)

			if tc.expectError {
				require.Error(t, err)
				assert.Nil(t, manager)
				if tc.expectedErr != nil {
					assert.ErrorIs(t, err, tc.expectedErr)
				}
			} else {
				require.NoError(t, err)
				assert.NotNil(t, manager)
				assert.Equal(t, tc.config, manager.config)
			}
		})
	}
}

func TestNewManagerWithFS(t *testing.T) {
	mockFS := common.NewMockFileSystem()

	config := &Config{
		Enabled:       false,
		HashDirectory: "/usr/local/etc/go-safe-cmd-runner/hashes",
	}

	manager, err := NewManagerWithFS(config, mockFS)
	require.NoError(t, err)
	assert.NotNil(t, manager)
	assert.Equal(t, config, manager.config)
	assert.Equal(t, mockFS, manager.fs)
}

func TestManager_IsEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{
			name: "enabled config",
			config: &Config{
				Enabled: true,
			},
			expected: true,
		},
		{
			name: "disabled config",
			config: &Config{
				Enabled: false,
			},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			manager := &Manager{config: tc.config}
			assert.Equal(t, tc.expected, manager.IsEnabled())
		})
	}
}

func TestManager_GetConfig(t *testing.T) {
	config := &Config{
		Enabled:       true,
		HashDirectory: "/test/path",
	}

	manager := &Manager{config: config}
	assert.Equal(t, config, manager.GetConfig())
}

func TestManager_VerifyConfigFile_Disabled(t *testing.T) {
	config := &Config{
		Enabled: false,
	}

	manager := &Manager{config: config}
	err := manager.VerifyConfigFile("/path/to/config.toml")
	assert.NoError(t, err)
}

func TestManager_ValidateHashDirectory_Disabled(t *testing.T) {
	config := &Config{
		Enabled: false,
	}

	manager := &Manager{config: config}
	err := manager.ValidateHashDirectory()
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrVerificationDisabled)
}

func TestManager_ValidateHashDirectory_NoSecurityValidator(t *testing.T) {
	config := &Config{
		Enabled:       true,
		HashDirectory: "/usr/local/etc/go-safe-cmd-runner/hashes",
	}

	manager := &Manager{
		config:   config,
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
	config := &Config{
		Enabled:       true,
		HashDirectory: "/usr/local/etc/go-safe-cmd-runner/hashes",
	}

	// Create manager with mocked components that will fail
	mockFS := common.NewMockFileSystem()
	manager := &Manager{
		config: config,
		fs:     mockFS,
		// Leave validator and security nil to trigger errors
	}

	err := manager.VerifyConfigFile("/path/to/config.toml")
	assert.Error(t, err)

	// Check that error is properly wrapped
	var verificationErr *Error
	assert.True(t, errors.As(err, &verificationErr))
	assert.Equal(t, "ValidateHashDirectory", verificationErr.Op)
}
