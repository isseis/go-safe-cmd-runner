package security

import (
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestShouldSkipHashValidation(t *testing.T) {
	testCases := []struct {
		name         string
		cmdPath      string
		globalConfig *runnertypes.GlobalConfig
		expectedSkip bool
	}{
		{
			name:         "nil config should not skip",
			cmdPath:      "/bin/ls",
			globalConfig: nil,
			expectedSkip: false,
		},
		{
			name:    "SkipStandardPaths=false should not skip standard directory",
			cmdPath: "/bin/ls",
			globalConfig: &runnertypes.GlobalConfig{
				SkipStandardPaths: false,
			},
			expectedSkip: false,
		},
		{
			name:    "SkipStandardPaths=false should not skip non-standard directory",
			cmdPath: "/home/user/script",
			globalConfig: &runnertypes.GlobalConfig{
				SkipStandardPaths: false,
			},
			expectedSkip: false,
		},
		{
			name:    "SkipStandardPaths=true should skip standard directory",
			cmdPath: "/bin/ls",
			globalConfig: &runnertypes.GlobalConfig{
				SkipStandardPaths: true,
			},
			expectedSkip: true,
		},
		{
			name:    "SkipStandardPaths=true should not skip non-standard directory",
			cmdPath: "/home/user/script",
			globalConfig: &runnertypes.GlobalConfig{
				SkipStandardPaths: true,
			},
			expectedSkip: false,
		},
		{
			name:    "SkipStandardPaths=true should skip usr/bin",
			cmdPath: "/usr/bin/cat",
			globalConfig: &runnertypes.GlobalConfig{
				SkipStandardPaths: true,
			},
			expectedSkip: true,
		},
		{
			name:    "SkipStandardPaths=true should skip usr/sbin",
			cmdPath: "/usr/sbin/systemctl",
			globalConfig: &runnertypes.GlobalConfig{
				SkipStandardPaths: true,
			},
			expectedSkip: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Convert globalConfig to skipStandardPaths boolean
			skipStandardPaths := tc.globalConfig != nil && tc.globalConfig.SkipStandardPaths
			result := shouldSkipHashValidation(tc.cmdPath, skipStandardPaths)
			assert.Equal(t, tc.expectedSkip, result)
		})
	}
}

func TestValidateFileHash(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("non-existent file should fail", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")
		config := DefaultConfig()

		err := validateFileHash(nonExistentFile, tmpDir, config)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrHashValidationFailed)
		// The error message will include "no such file or directory" for file system errors
		assert.Contains(t, err.Error(), "no such file or directory")
	})

	t.Run("testSkipHashValidation should skip validation", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")
		config := NewTestConfigWithSkipHashValidation()

		err := validateFileHash(nonExistentFile, tmpDir, config)
		assert.NoError(t, err, "hash validation should be skipped when testSkipHashValidation is true")
	})

	t.Run("nil config should perform validation", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")

		err := validateFileHash(nonExistentFile, tmpDir, nil)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrHashValidationFailed)
	})
}
