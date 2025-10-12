package runnertypes

import (
	"errors"
	"testing"
)

func TestParseRiskLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected RiskLevel
		hasError bool
	}{
		{
			name:     "valid unknown risk",
			input:    "unknown",
			expected: RiskLevelUnknown,
			hasError: false,
		},
		{
			name:     "valid low risk",
			input:    "low",
			expected: RiskLevelLow,
			hasError: false,
		},
		{
			name:     "valid medium risk",
			input:    "medium",
			expected: RiskLevelMedium,
			hasError: false,
		},
		{
			name:     "valid high risk",
			input:    "high",
			expected: RiskLevelHigh,
			hasError: false,
		},
		{
			name:     "critical risk is prohibited in configuration",
			input:    "critical",
			expected: RiskLevelUnknown,
			hasError: true,
		},
		{
			name:     "empty string defaults to low",
			input:    "",
			expected: RiskLevelLow,
			hasError: false,
		},
		{
			name:     "invalid risk level",
			input:    "invalid",
			expected: RiskLevelUnknown,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseRiskLevel(tt.input)

			if tt.hasError && err == nil {
				t.Errorf("expected error but got none")
			}

			if !tt.hasError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRiskLevelString(t *testing.T) {
	tests := []struct {
		level    RiskLevel
		expected string
	}{
		{RiskLevelLow, "low"},
		{RiskLevelMedium, "medium"},
		{RiskLevelHigh, "high"},
		{RiskLevelCritical, "critical"},
		{RiskLevel(999), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.level.String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCommandGetMaxRiskLevel(t *testing.T) {
	tests := []struct {
		name        string
		maxRiskStr  string
		expected    RiskLevel
		expectError bool
	}{
		{
			name:        "valid unknown risk",
			maxRiskStr:  "unknown",
			expected:    RiskLevelUnknown,
			expectError: false,
		},
		{
			name:        "valid low risk",
			maxRiskStr:  "low",
			expected:    RiskLevelLow,
			expectError: false,
		},
		{
			name:        "valid medium risk",
			maxRiskStr:  "medium",
			expected:    RiskLevelMedium,
			expectError: false,
		},
		{
			name:        "valid high risk",
			maxRiskStr:  "high",
			expected:    RiskLevelHigh,
			expectError: false,
		},
		{
			name:        "empty defaults to low",
			maxRiskStr:  "",
			expected:    RiskLevelLow,
			expectError: false,
		},
		{
			name:        "invalid risk level",
			maxRiskStr:  "invalid",
			expected:    RiskLevelUnknown,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{
				MaxRiskLevel: tt.maxRiskStr,
			}

			result, err := cmd.GetMaxRiskLevel()

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCommandHasUserGroupSpecification(t *testing.T) {
	tests := []struct {
		name       string
		runAsUser  string
		runAsGroup string
		expected   bool
	}{
		{
			name:       "no user or group specified",
			runAsUser:  "",
			runAsGroup: "",
			expected:   false,
		},
		{
			name:       "user specified only",
			runAsUser:  "testuser",
			runAsGroup: "",
			expected:   true,
		},
		{
			name:       "group specified only",
			runAsUser:  "",
			runAsGroup: "testgroup",
			expected:   true,
		},
		{
			name:       "both user and group specified",
			runAsUser:  "testuser",
			runAsGroup: "testgroup",
			expected:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &Command{
				RunAsUser:  tt.runAsUser,
				RunAsGroup: tt.runAsGroup,
			}

			result := cmd.HasUserGroupSpecification()

			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestBuildEnvironmentMap_Success verifies that a well-formed Env slice
// produces the expected map without error.
func TestBuildEnvironmentMap_Success(t *testing.T) {
	c := &Command{
		Env: []string{"FOO=bar", "BAZ=qux"},
	}

	m, err := c.BuildEnvironmentMap()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got, want := m["FOO"], "bar"; got != want {
		t.Fatalf("FOO: got %q want %q", got, want)
	}
	if got, want := m["BAZ"], "qux"; got != want {
		t.Fatalf("BAZ: got %q want %q", got, want)
	}
}

// TestBuildEnvironmentMap_DuplicateKey verifies that when the same key is
// specified twice, BuildEnvironmentMap returns ErrDuplicateEnvironmentVariable.
func TestBuildEnvironmentMap_DuplicateKey(t *testing.T) {
	c := &Command{
		Env: []string{"FOO=bar", "FOO=baz"},
	}

	_, err := c.BuildEnvironmentMap()
	if err == nil {
		t.Fatalf("expected error for duplicate key, got nil")
	}

	if !errors.Is(err, ErrDuplicateEnvironmentVariable) {
		t.Fatalf("expected ErrDuplicateEnvironmentVariable, got: %v", err)
	}
}

func TestAllowlistResolution_GetEffectiveList(t *testing.T) {
	tests := []struct {
		name     string
		resolver *AllowlistResolution
		expected []string
	}{
		{
			name:     "nil resolver returns empty slice",
			resolver: nil,
			expected: []string{},
		},
		{
			name: "returns EffectiveList",
			resolver: &AllowlistResolution{
				EffectiveList: []string{"VAR1", "VAR2"},
			},
			expected: []string{"VAR1", "VAR2"},
		},
		{
			name: "empty EffectiveList",
			resolver: &AllowlistResolution{
				EffectiveList: []string{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolver.GetEffectiveList()
			if len(result) != len(tt.expected) {
				t.Fatalf("expected length %d, got %d. expected=%#v, got=%#v", len(tt.expected), len(result), tt.expected, result)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("at index %d: expected %s, got %s", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestAllowlistResolution_GetEffectiveSize(t *testing.T) {
	tests := []struct {
		name     string
		resolver *AllowlistResolution
		expected int
	}{
		{
			name:     "nil resolver returns 0",
			resolver: nil,
			expected: 0,
		},
		{
			name: "returns correct size",
			resolver: &AllowlistResolution{
				EffectiveList: []string{"VAR1", "VAR2", "VAR3"},
			},
			expected: 3,
		},
		{
			name: "empty list returns 0",
			resolver: &AllowlistResolution{
				EffectiveList: []string{},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolver.GetEffectiveSize()
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestAllowlistResolution_GetGroupAllowlist(t *testing.T) {
	tests := []struct {
		name     string
		resolver *AllowlistResolution
		expected []string
	}{
		{
			name:     "nil resolver returns empty slice",
			resolver: nil,
			expected: []string{},
		},
		{
			name: "returns GroupAllowlist",
			resolver: &AllowlistResolution{
				GroupAllowlist: []string{"GROUP_VAR1", "GROUP_VAR2"},
			},
			expected: []string{"GROUP_VAR1", "GROUP_VAR2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolver.GetGroupAllowlist()
			if len(result) != len(tt.expected) {
				// Using t.Fatalf prevents a potential panic in the loop below.
				t.Fatalf("expected length %d, got %d. expected=%#v, got=%#v", len(tt.expected), len(result), tt.expected, result)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("at index %d: expected %s, got %s", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestAllowlistResolution_GetGlobalAllowlist(t *testing.T) {
	tests := []struct {
		name     string
		resolver *AllowlistResolution
		expected []string
	}{
		{
			name:     "nil resolver returns empty slice",
			resolver: nil,
			expected: []string{},
		},
		{
			name: "returns GlobalAllowlist",
			resolver: &AllowlistResolution{
				GlobalAllowlist: []string{"GLOBAL_VAR1", "GLOBAL_VAR2"},
			},
			expected: []string{"GLOBAL_VAR1", "GLOBAL_VAR2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolver.GetGlobalAllowlist()
			if len(result) != len(tt.expected) {
				t.Errorf("expected length %d, got %d", len(tt.expected), len(result))
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("at index %d: expected %s, got %s", i, tt.expected[i], result[i])
				}
			}
		})
	}
}

func TestAllowlistResolution_GetMode(t *testing.T) {
	tests := []struct {
		name     string
		resolver *AllowlistResolution
		expected InheritanceMode
	}{
		{
			name:     "nil resolver returns default mode",
			resolver: nil,
			expected: InheritanceModeInherit,
		},
		{
			name: "returns Inherit mode",
			resolver: &AllowlistResolution{
				Mode: InheritanceModeInherit,
			},
			expected: InheritanceModeInherit,
		},
		{
			name: "returns Explicit mode",
			resolver: &AllowlistResolution{
				Mode: InheritanceModeExplicit,
			},
			expected: InheritanceModeExplicit,
		},
		{
			name: "returns Reject mode",
			resolver: &AllowlistResolution{
				Mode: InheritanceModeReject,
			},
			expected: InheritanceModeReject,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolver.GetMode()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestAllowlistResolution_GetGroupName(t *testing.T) {
	tests := []struct {
		name     string
		resolver *AllowlistResolution
		expected string
	}{
		{
			name:     "nil resolver returns empty string",
			resolver: nil,
			expected: "",
		},
		{
			name: "returns GroupName",
			resolver: &AllowlistResolution{
				GroupName: "test-group",
			},
			expected: "test-group",
		},
		{
			name: "returns empty GroupName",
			resolver: &AllowlistResolution{
				GroupName: "",
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolver.GetGroupName()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}
