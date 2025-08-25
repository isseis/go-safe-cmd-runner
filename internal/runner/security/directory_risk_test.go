package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestGetDefaultRiskByDirectory(t *testing.T) {
	testCases := []struct {
		name         string
		cmdPath      string
		expectedRisk runnertypes.RiskLevel
	}{
		{
			name:         "bin directory",
			cmdPath:      "/bin/ls",
			expectedRisk: runnertypes.RiskLevelLow,
		},
		{
			name:         "usr/bin directory",
			cmdPath:      "/usr/bin/cat",
			expectedRisk: runnertypes.RiskLevelLow,
		},
		{
			name:         "usr/local/bin directory",
			cmdPath:      "/usr/local/bin/custom",
			expectedRisk: runnertypes.RiskLevelLow,
		},
		{
			name:         "sbin directory",
			cmdPath:      "/sbin/ifconfig",
			expectedRisk: runnertypes.RiskLevelMedium,
		},
		{
			name:         "usr/sbin directory",
			cmdPath:      "/usr/sbin/systemctl",
			expectedRisk: runnertypes.RiskLevelMedium,
		},
		{
			name:         "usr/local/sbin directory",
			cmdPath:      "/usr/local/sbin/custom",
			expectedRisk: runnertypes.RiskLevelMedium,
		},
		{
			name:         "non-standard directory",
			cmdPath:      "/home/user/script",
			expectedRisk: runnertypes.RiskLevelUnknown,
		},
		{
			name:         "subdirectory of bin",
			cmdPath:      "/bin/subdir/script",
			expectedRisk: runnertypes.RiskLevelLow,
		},
		{
			name:         "subdirectory of usr/sbin",
			cmdPath:      "/usr/sbin/subdir/tool",
			expectedRisk: runnertypes.RiskLevelMedium,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := getDefaultRiskByDirectory(tc.cmdPath)
			assert.Equal(t, tc.expectedRisk, result)
		})
	}
}

func TestIsStandardDirectory(t *testing.T) {
	testCases := []struct {
		name         string
		cmdPath      string
		expectedBool bool
	}{
		{
			name:         "bin directory",
			cmdPath:      "/bin/ls",
			expectedBool: true,
		},
		{
			name:         "usr/bin directory",
			cmdPath:      "/usr/bin/cat",
			expectedBool: true,
		},
		{
			name:         "usr/local/bin directory",
			cmdPath:      "/usr/local/bin/custom",
			expectedBool: true,
		},
		{
			name:         "sbin directory",
			cmdPath:      "/sbin/ifconfig",
			expectedBool: true,
		},
		{
			name:         "usr/sbin directory",
			cmdPath:      "/usr/sbin/systemctl",
			expectedBool: true,
		},
		{
			name:         "usr/local/sbin directory",
			cmdPath:      "/usr/local/sbin/custom",
			expectedBool: true,
		},
		{
			name:         "non-standard directory",
			cmdPath:      "/home/user/script",
			expectedBool: false,
		},
		{
			name:         "subdirectory of bin",
			cmdPath:      "/bin/subdir/script",
			expectedBool: true,
		},
		{
			name:         "subdirectory of usr/sbin",
			cmdPath:      "/usr/sbin/subdir/tool",
			expectedBool: true,
		},
		{
			name:         "root directory",
			cmdPath:      "/script",
			expectedBool: false,
		},
		{
			name:         "partial match should not return true",
			cmdPath:      "/usr/binding/test",
			expectedBool: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isStandardDirectory(tc.cmdPath)
			assert.Equal(t, tc.expectedBool, result)
		})
	}
}
