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
	assert.True(t, config.IsEnabled())
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

func TestConfig_IsEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		config   *Config
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name: "disabled config",
			config: &Config{
				Enabled: false,
			},
			expected: false,
		},
		{
			name: "enabled config",
			config: &Config{
				Enabled: true,
			},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.config.IsEnabled())
		})
	}
}

func TestConfig_Validate_PathCleaning(t *testing.T) {
	config := &Config{
		Enabled:       true,
		HashDirectory: "/path/to/../hashes/./",
	}

	err := config.Validate()
	require.NoError(t, err)

	// Path should be cleaned
	assert.Equal(t, "/path/hashes", config.HashDirectory)
}
