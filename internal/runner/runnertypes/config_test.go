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
