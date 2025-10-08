package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestRiskFactor(t *testing.T) {
	tests := []struct {
		name       string
		risk       RiskFactor
		wantLevel  runnertypes.RiskLevel
		wantReason string
	}{
		{
			name:       "Unknown risk with empty reason",
			risk:       RiskFactor{Level: runnertypes.RiskLevelUnknown},
			wantLevel:  runnertypes.RiskLevelUnknown,
			wantReason: "",
		},
		{
			name:       "Low risk with reason",
			risk:       RiskFactor{Level: runnertypes.RiskLevelLow, Reason: "Low impact operation"},
			wantLevel:  runnertypes.RiskLevelLow,
			wantReason: "Low impact operation",
		},
		{
			name:       "Medium risk",
			risk:       RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network operation"},
			wantLevel:  runnertypes.RiskLevelMedium,
			wantReason: "Network operation",
		},
		{
			name:       "High risk",
			risk:       RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
			wantLevel:  runnertypes.RiskLevelHigh,
			wantReason: "Data exfiltration",
		},
		{
			name:       "Critical risk",
			risk:       RiskFactor{Level: runnertypes.RiskLevelCritical, Reason: "Privilege escalation"},
			wantLevel:  runnertypes.RiskLevelCritical,
			wantReason: "Privilege escalation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantLevel, tt.risk.Level)
			assert.Equal(t, tt.wantReason, tt.risk.Reason)
		})
	}
}
