package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestGetCommandRiskOverride(t *testing.T) {
	testCases := []struct {
		name          string
		cmdPath       string
		expectedRisk  runnertypes.RiskLevel
		expectedFound bool
	}{
		{
			name:          "sudo command should have critical risk",
			cmdPath:       "/usr/bin/sudo",
			expectedRisk:  runnertypes.RiskLevelCritical,
			expectedFound: true,
		},
		{
			name:          "su command should have critical risk",
			cmdPath:       "/bin/su",
			expectedRisk:  runnertypes.RiskLevelCritical,
			expectedFound: true,
		},
		{
			name:          "curl command should have medium risk",
			cmdPath:       "/usr/bin/curl",
			expectedRisk:  runnertypes.RiskLevelMedium,
			expectedFound: true,
		},
		{
			name:          "wget command should have medium risk",
			cmdPath:       "/usr/bin/wget",
			expectedRisk:  runnertypes.RiskLevelMedium,
			expectedFound: true,
		},
		{
			name:          "systemctl command should have high risk",
			cmdPath:       "/usr/sbin/systemctl",
			expectedRisk:  runnertypes.RiskLevelHigh,
			expectedFound: true,
		},
		{
			name:          "service command should have high risk",
			cmdPath:       "/usr/sbin/service",
			expectedRisk:  runnertypes.RiskLevelHigh,
			expectedFound: true,
		},
		{
			name:          "rm command should have high risk",
			cmdPath:       "/bin/rm",
			expectedRisk:  runnertypes.RiskLevelHigh,
			expectedFound: true,
		},
		{
			name:          "dd command should have high risk",
			cmdPath:       "/usr/bin/dd",
			expectedRisk:  runnertypes.RiskLevelHigh,
			expectedFound: true,
		},
		{
			name:          "ls command should not have override",
			cmdPath:       "/bin/ls",
			expectedRisk:  runnertypes.RiskLevelUnknown,
			expectedFound: false,
		},
		{
			name:          "non-existent command should not have override",
			cmdPath:       "/usr/bin/unknown",
			expectedRisk:  runnertypes.RiskLevelUnknown,
			expectedFound: false,
		},
		{
			name:          "relative path should not match",
			cmdPath:       "sudo",
			expectedRisk:  runnertypes.RiskLevelUnknown,
			expectedFound: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			risk, found := getCommandRiskOverride(tc.cmdPath)
			assert.Equal(t, tc.expectedFound, found)
			if found {
				assert.Equal(t, tc.expectedRisk, risk)
			}
		})
	}
}

func TestCommandRiskOverrides_Completeness(t *testing.T) {
	// Ensure all defined overrides are valid risk levels
	for cmdPath, risk := range CommandRiskOverrides {
		t.Run("validate_"+cmdPath, func(t *testing.T) {
			assert.NotEmpty(t, cmdPath, "command path should not be empty")
			assert.True(t, risk > runnertypes.RiskLevelUnknown && risk <= runnertypes.RiskLevelCritical,
				"risk level should be valid: %d", risk)
		})
	}
}
