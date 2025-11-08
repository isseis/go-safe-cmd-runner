package security

import (
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestShouldPerformHashValidation(t *testing.T) {
	testCases := []struct {
		name            string
		cmdPath         string
		globalConfig    *runnertypes.GlobalSpec
		expectedPerform bool
	}{
		{
			name:            "nil config should perform validation (default: verify standard paths)",
			cmdPath:         "/bin/ls",
			globalConfig:    nil,
			expectedPerform: true, // default behavior is to verify standard paths (VerifyStandardPaths=true)
		},
		{
			name:    "VerifyStandardPaths=false should not perform validation for standard directory",
			cmdPath: "/bin/ls",
			globalConfig: &runnertypes.GlobalSpec{
				VerifyStandardPaths: &[]bool{false}[0], // false means skip verification
			},
			expectedPerform: false,
		},
		{
			name:    "VerifyStandardPaths=false should perform validation for non-standard directory",
			cmdPath: "/home/user/script",
			globalConfig: &runnertypes.GlobalSpec{
				VerifyStandardPaths: &[]bool{false}[0], // false means skip verification
			},
			expectedPerform: true,
		},
		{
			name:    "VerifyStandardPaths=true should perform validation for standard directory",
			cmdPath: "/bin/ls",
			globalConfig: &runnertypes.GlobalSpec{
				VerifyStandardPaths: &[]bool{true}[0], // true means verify (don't skip)
			},
			expectedPerform: true,
		},
		{
			name:    "VerifyStandardPaths=true should perform validation for non-standard directory",
			cmdPath: "/home/user/script",
			globalConfig: &runnertypes.GlobalSpec{
				VerifyStandardPaths: &[]bool{true}[0], // true means verify (don't skip)
			},
			expectedPerform: true,
		},
		{
			name:    "VerifyStandardPaths=true should perform validation for usr/bin",
			cmdPath: "/usr/bin/curl",
			globalConfig: &runnertypes.GlobalSpec{
				VerifyStandardPaths: &[]bool{true}[0], // true means verify (don't skip)
			},
			expectedPerform: true,
		},
		{
			name:    "VerifyStandardPaths=true should perform validation for usr/sbin",
			cmdPath: "/usr/sbin/nginx",
			globalConfig: &runnertypes.GlobalSpec{
				VerifyStandardPaths: &[]bool{true}[0], // true means verify (don't skip)
			},
			expectedPerform: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Determine VerifyStandardPaths from globalConfig
			var verifyStandardPathsPtr *bool
			if tc.globalConfig != nil {
				verifyStandardPathsPtr = tc.globalConfig.VerifyStandardPaths
			}

			verifyStandardPaths := runnertypes.DetermineVerifyStandardPaths(verifyStandardPathsPtr)
			result := shouldPerformHashValidation(tc.cmdPath, verifyStandardPaths)
			assert.Equal(t, tc.expectedPerform, result)
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
	})

	t.Run("testSkipHashValidation should skip validation", func(t *testing.T) {
		nonExistentFile := filepath.Join(tmpDir, "non_existent")
		config := NewSkipHashValidationTestConfig()

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
