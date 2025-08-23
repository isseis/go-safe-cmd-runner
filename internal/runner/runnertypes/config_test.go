package runnertypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCommand_ValidateMaxRiskLevel(t *testing.T) {
	tests := []struct {
		name         string
		maxRiskLevel string
		wantErr      bool
		errorMessage string
	}{
		{
			name:         "empty max risk level",
			maxRiskLevel: "",
			wantErr:      false,
		},
		{
			name:         "valid low risk level",
			maxRiskLevel: "low",
			wantErr:      false,
		},
		{
			name:         "valid medium risk level",
			maxRiskLevel: "medium",
			wantErr:      false,
		},
		{
			name:         "valid high risk level",
			maxRiskLevel: "high",
			wantErr:      false,
		},
		{
			name:         "invalid risk level",
			maxRiskLevel: "invalid",
			wantErr:      true,
			errorMessage: "invalid max_risk_level: 'invalid' must be one of 'low', 'medium', 'high', or empty",
		},
		{
			name:         "invalid case risk level",
			maxRiskLevel: "HIGH",
			wantErr:      true,
			errorMessage: "invalid max_risk_level: 'HIGH' must be one of 'low', 'medium', 'high', or empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{
				MaxRiskLevel: tt.maxRiskLevel,
			}

			err := cmd.ValidateMaxRiskLevel()

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Equal(t, tt.errorMessage, err.Error())
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCommand_GetMaxRiskLevel(t *testing.T) {
	tests := []struct {
		name         string
		maxRiskLevel string
		privileged   bool
		expected     RiskLevel
	}{
		{
			name:         "empty max risk level - non-privileged",
			maxRiskLevel: "",
			privileged:   false,
			expected:     RiskLevelMedium,
		},
		{
			name:         "empty max risk level - privileged",
			maxRiskLevel: "",
			privileged:   true,
			expected:     RiskLevelHigh,
		},
		{
			name:         "explicit low risk level",
			maxRiskLevel: "low",
			privileged:   false,
			expected:     RiskLevelLow,
		},
		{
			name:         "explicit medium risk level",
			maxRiskLevel: "medium",
			privileged:   false,
			expected:     RiskLevelMedium,
		},
		{
			name:         "explicit high risk level",
			maxRiskLevel: "high",
			privileged:   false,
			expected:     RiskLevelHigh,
		},
		{
			name:         "explicit low risk level - privileged ignored",
			maxRiskLevel: "low",
			privileged:   true,
			expected:     RiskLevelLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{
				MaxRiskLevel: tt.maxRiskLevel,
				Privileged:   tt.privileged,
			}

			result := cmd.GetMaxRiskLevel()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCommand_SetDefaults(t *testing.T) {
	cmd := &Command{
		Name: "test",
		Cmd:  "echo",
	}

	// SetDefaults should not modify MaxRiskLevel field directly
	cmd.SetDefaults()
	assert.Equal(t, "", cmd.MaxRiskLevel)
}

func TestInheritanceMode_String(t *testing.T) {
	tests := []struct {
		mode     InheritanceMode
		expected string
	}{
		{InheritanceModeInherit, "inherit"},
		{InheritanceModeExplicit, "explicit"},
		{InheritanceModeReject, "reject"},
		{InheritanceMode(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.mode.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAllowlistResolution_IsAllowed(t *testing.T) {
	tests := []struct {
		name       string
		resolution AllowlistResolution
		variable   string
		expected   bool
	}{
		{
			name: "reject mode - should always return false",
			resolution: AllowlistResolution{
				Mode: InheritanceModeReject,
			},
			variable: "ANY_VAR",
			expected: false,
		},
		{
			name: "explicit mode - variable in group allowlist",
			resolution: AllowlistResolution{
				Mode:           InheritanceModeExplicit,
				GroupAllowlist: []string{"VAR1", "VAR2"},
			},
			variable: "VAR1",
			expected: true,
		},
		{
			name: "explicit mode - variable not in group allowlist",
			resolution: AllowlistResolution{
				Mode:           InheritanceModeExplicit,
				GroupAllowlist: []string{"VAR1", "VAR2"},
			},
			variable: "VAR3",
			expected: false,
		},
		{
			name: "inherit mode - variable in global allowlist",
			resolution: AllowlistResolution{
				Mode:            InheritanceModeInherit,
				GlobalAllowlist: []string{"GLOBAL1", "GLOBAL2"},
			},
			variable: "GLOBAL1",
			expected: true,
		},
		{
			name: "inherit mode - variable not in global allowlist",
			resolution: AllowlistResolution{
				Mode:            InheritanceModeInherit,
				GlobalAllowlist: []string{"GLOBAL1", "GLOBAL2"},
			},
			variable: "OTHER",
			expected: false,
		},
		{
			name: "invalid mode - should return false",
			resolution: AllowlistResolution{
				Mode: InheritanceMode(999),
			},
			variable: "ANY_VAR",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolution.IsAllowed(tt.variable)
			assert.Equal(t, tt.expected, result)
		})
	}
}
