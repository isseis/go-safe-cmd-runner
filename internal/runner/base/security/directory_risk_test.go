package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
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
