package runnertypes

import (
	"testing"
)

func TestCommandSpec_GetMaxRiskLevel(t *testing.T) {
	tests := []struct {
		name        string
		cmd         *CommandSpec
		expectErr   bool
		expectLevel RiskLevel
	}{
		{
			name: "low risk level",
			cmd: &CommandSpec{
				MaxRiskLevel: "low",
			},
			expectErr:   false,
			expectLevel: RiskLevelLow,
		},
		{
			name: "medium risk level",
			cmd: &CommandSpec{
				MaxRiskLevel: "medium",
			},
			expectErr:   false,
			expectLevel: RiskLevelMedium,
		},
		{
			name: "high risk level",
			cmd: &CommandSpec{
				MaxRiskLevel: "high",
			},
			expectErr:   false,
			expectLevel: RiskLevelHigh,
		},
		{
			name: "empty string defaults to low",
			cmd: &CommandSpec{
				MaxRiskLevel: "",
			},
			expectErr:   false,
			expectLevel: RiskLevelLow,
		},
		{
			name: "invalid risk level",
			cmd: &CommandSpec{
				MaxRiskLevel: "invalid",
			},
			expectErr:   true,
			expectLevel: RiskLevelUnknown,
		},
		{
			name: "critical risk level is prohibited",
			cmd: &CommandSpec{
				MaxRiskLevel: "critical",
			},
			expectErr:   true,
			expectLevel: RiskLevelUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			level, err := tt.cmd.GetMaxRiskLevel()
			if (err != nil) != tt.expectErr {
				t.Errorf("GetMaxRiskLevel() error = %v, expectErr %v", err, tt.expectErr)
			}
			if level != tt.expectLevel {
				t.Errorf("GetMaxRiskLevel() = %v, want %v", level, tt.expectLevel)
			}
		})
	}
}

func TestCommandSpec_HasUserGroupSpecification(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *CommandSpec
		expected bool
	}{
		{
			name: "both user and group specified",
			cmd: &CommandSpec{
				RunAsUser:  "nobody",
				RunAsGroup: "nogroup",
			},
			expected: true,
		},
		{
			name: "only user specified",
			cmd: &CommandSpec{
				RunAsUser:  "nobody",
				RunAsGroup: "",
			},
			expected: true,
		},
		{
			name: "only group specified",
			cmd: &CommandSpec{
				RunAsUser:  "",
				RunAsGroup: "nogroup",
			},
			expected: true,
		},
		{
			name: "neither user nor group specified",
			cmd: &CommandSpec{
				RunAsUser:  "",
				RunAsGroup: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cmd.HasUserGroupSpecification()
			if result != tt.expected {
				t.Errorf("HasUserGroupSpecification() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestConfigSpec_Structure(t *testing.T) {
	// Test that ConfigSpec can be created with nested Spec types
	config := &ConfigSpec{
		Version: "1.0",
		Global: GlobalSpec{
			Timeout:     30,
			LogLevel:    "info",
			Env:         []string{"KEY=VALUE"},
			VerifyFiles: []string{"/path/to/file"},
		},
		Groups: []GroupSpec{
			{
				Name:     "test-group",
				Priority: 1,
				Commands: []CommandSpec{
					{
						Name:         "test-cmd",
						Cmd:          "/bin/echo",
						Args:         []string{"hello"},
						MaxRiskLevel: "low",
					},
				},
			},
		},
	}

	if config.Version != "1.0" {
		t.Errorf("Version = %v, want 1.0", config.Version)
	}
	if config.Global.Timeout != 30 {
		t.Errorf("Global.Timeout = %v, want 30", config.Global.Timeout)
	}
	if len(config.Groups) != 1 {
		t.Errorf("len(Groups) = %v, want 1", len(config.Groups))
	}
	if config.Groups[0].Name != "test-group" {
		t.Errorf("Groups[0].Name = %v, want test-group", config.Groups[0].Name)
	}
	if len(config.Groups[0].Commands) != 1 {
		t.Errorf("len(Groups[0].Commands) = %v, want 1", len(config.Groups[0].Commands))
	}
}
