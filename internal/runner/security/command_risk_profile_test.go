package security

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
)

func TestCommandRiskProfile_BaseRiskLevel(t *testing.T) {
	tests := []struct {
		name    string
		profile CommandRiskProfile
		want    runnertypes.RiskLevel
	}{
		{
			name: "all unknown",
			profile: CommandRiskProfile{
				PrivilegeRisk:   RiskFactor{Level: runnertypes.RiskLevelUnknown},
				NetworkRisk:     RiskFactor{Level: runnertypes.RiskLevelUnknown},
				DestructionRisk: RiskFactor{Level: runnertypes.RiskLevelUnknown},
				DataExfilRisk:   RiskFactor{Level: runnertypes.RiskLevelUnknown},
				SystemModRisk:   RiskFactor{Level: runnertypes.RiskLevelUnknown},
			},
			want: runnertypes.RiskLevelUnknown,
		},
		{
			name: "single medium risk",
			profile: CommandRiskProfile{
				NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelMedium},
			},
			want: runnertypes.RiskLevelMedium,
		},
		{
			name: "multiple risks - max is high",
			profile: CommandRiskProfile{
				NetworkRisk:   RiskFactor{Level: runnertypes.RiskLevelMedium},
				DataExfilRisk: RiskFactor{Level: runnertypes.RiskLevelHigh},
			},
			want: runnertypes.RiskLevelHigh,
		},
		{
			name: "critical privilege risk",
			profile: CommandRiskProfile{
				PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelCritical},
				NetworkRisk:   RiskFactor{Level: runnertypes.RiskLevelMedium},
			},
			want: runnertypes.RiskLevelCritical,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.profile.BaseRiskLevel())
		})
	}
}

func TestCommandRiskProfile_GetRiskReasons(t *testing.T) {
	tests := []struct {
		name    string
		profile CommandRiskProfile
		want    []string
	}{
		{
			name: "no risks",
			profile: CommandRiskProfile{
				PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelUnknown},
			},
			want: []string{},
		},
		{
			name: "single risk",
			profile: CommandRiskProfile{
				NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network access"},
			},
			want: []string{"Network access"},
		},
		{
			name: "multiple risks",
			profile: CommandRiskProfile{
				NetworkRisk:   RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network access"},
				DataExfilRisk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
			},
			want: []string{"Network access", "Data exfiltration"},
		},
		{
			name: "all risk types",
			profile: CommandRiskProfile{
				PrivilegeRisk:   RiskFactor{Level: runnertypes.RiskLevelCritical, Reason: "Privilege escalation"},
				NetworkRisk:     RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network access"},
				DestructionRisk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "File deletion"},
				DataExfilRisk:   RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
				SystemModRisk:   RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "System modification"},
			},
			want: []string{
				"Privilege escalation",
				"Network access",
				"File deletion",
				"Data exfiltration",
				"System modification",
			},
		},
		{
			name: "empty reason is excluded",
			profile: CommandRiskProfile{
				NetworkRisk:   RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: ""},
				DataExfilRisk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
			},
			want: []string{"Data exfiltration"},
		},
		{
			name: "mixed empty and non-empty reasons",
			profile: CommandRiskProfile{
				PrivilegeRisk:   RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: ""},
				NetworkRisk:     RiskFactor{Level: runnertypes.RiskLevelMedium, Reason: "Network access"},
				DestructionRisk: RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: ""},
				DataExfilRisk:   RiskFactor{Level: runnertypes.RiskLevelHigh, Reason: "Data exfiltration"},
			},
			want: []string{"Network access", "Data exfiltration"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.profile.GetRiskReasons()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCommandRiskProfile_IsPrivilege(t *testing.T) {
	tests := []struct {
		name    string
		profile CommandRiskProfile
		want    bool
	}{
		{
			name: "critical privilege risk is privilege",
			profile: CommandRiskProfile{
				PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelCritical},
			},
			want: true,
		},
		{
			name: "high privilege risk is privilege",
			profile: CommandRiskProfile{
				PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelHigh},
			},
			want: true,
		},
		{
			name: "medium privilege risk is not privilege",
			profile: CommandRiskProfile{
				PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelMedium},
			},
			want: false,
		},
		{
			name: "low privilege risk is not privilege",
			profile: CommandRiskProfile{
				PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelLow},
			},
			want: false,
		},
		{
			name: "unknown privilege risk is not privilege",
			profile: CommandRiskProfile{
				PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelUnknown},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.profile.IsPrivilege())
		})
	}
}

func TestCommandRiskProfile_Validate(t *testing.T) {
	tests := []struct {
		name    string
		profile CommandRiskProfile
		wantErr error
	}{
		{
			name: "valid profile - all unknown",
			profile: CommandRiskProfile{
				NetworkType: NetworkTypeNone,
			},
			wantErr: nil,
		},
		{
			name: "valid profile - privilege escalation",
			profile: CommandRiskProfile{
				PrivilegeRisk: RiskFactor{Level: runnertypes.RiskLevelCritical},
				NetworkType:   NetworkTypeNone,
			},
			wantErr: nil,
		},
		{
			name: "valid profile - always network",
			profile: CommandRiskProfile{
				NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelMedium},
				NetworkType: NetworkTypeAlways,
			},
			wantErr: nil,
		},
		{
			name: "valid profile - conditional network",
			profile: CommandRiskProfile{
				NetworkRisk:        RiskFactor{Level: runnertypes.RiskLevelMedium},
				NetworkType:        NetworkTypeConditional,
				NetworkSubcommands: []string{"clone", "fetch"},
			},
			wantErr: nil,
		},
		{
			name: "invalid - NetworkTypeAlways with low NetworkRisk",
			profile: CommandRiskProfile{
				NetworkRisk: RiskFactor{Level: runnertypes.RiskLevelLow},
				NetworkType: NetworkTypeAlways,
			},
			wantErr: ErrNetworkAlwaysRequiresMediumRisk,
		},
		{
			name: "invalid - NetworkSubcommands without Conditional",
			profile: CommandRiskProfile{
				NetworkType:        NetworkTypeNone,
				NetworkSubcommands: []string{"clone"},
			},
			wantErr: ErrNetworkSubcommandsOnlyForConditional,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.profile.Validate()
			if tt.wantErr != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
