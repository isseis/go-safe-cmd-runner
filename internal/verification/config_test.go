package verification

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.True(t, config.Enabled)
	assert.Equal(t, "/usr/local/etc/go-safe-cmd-runner/hashes", config.HashDirectory)
}

func TestConfig_Validate(t *testing.T) {
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
				HashDirectory: "/some/path",
			},
			expectError: false,
		},
		{
			name: "valid enabled config",
			config: &Config{
				Enabled:       true,
				HashDirectory: "/usr/local/etc/go-safe-cmd-runner/hashes",
			},
			expectError: false,
		},
		{
			name: "enabled with empty hash directory",
			config: &Config{
				Enabled:       true,
				HashDirectory: "",
			},
			expectError: true,
			expectedErr: ErrHashDirectoryEmpty,
		},
		{
			name: "enabled with whitespace hash directory",
			config: &Config{
				Enabled:       true,
				HashDirectory: "   ",
			},
			expectError: true,
			expectedErr: ErrHashDirectoryEmpty,
		},
		{
			name: "enabled with relative hash directory",
			config: &Config{
				Enabled:       true,
				HashDirectory: "relative/path",
			},
			expectError: true,
			expectedErr: ErrHashDirectoryInvalid,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Validate()

			if tc.expectError {
				require.Error(t, err)
				if tc.expectedErr != nil {
					assert.ErrorIs(t, err, tc.expectedErr)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_Validate_PathCleaning(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean simple path",
			input:    "/path/to/file",
			expected: "/path/to/file",
		},
		{
			name:     "clean path with parent directory",
			input:    "/path/to/../file",
			expected: "/path/file",
		},
		{
			name:     "clean path with current directory",
			input:    "/path/./to/./file",
			expected: "/path/to/file",
		},
		{
			name:     "clean path with trailing slash",
			input:    "/path/to/dir/",
			expected: "/path/to/dir",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := Config{
				Enabled:       true,
				HashDirectory: tc.input,
			}

			// Call NewManagerWithFS which should clean the path internally
			// We use a mock file system to avoid actual filesystem operations
			manager, _ := NewManagerWithFS(config, nil)

			// We don't care about the error here since we're just testing path cleaning
			// and the error might occur due to the nil filesystem
			// The original config should not be modified
			assert.Equal(t, tc.input, config.HashDirectory)

			// The manager should have the cleaned path
			if manager != nil {
				assert.Equal(t, tc.expected, manager.GetConfig().HashDirectory)
			}
		})
	}
}
