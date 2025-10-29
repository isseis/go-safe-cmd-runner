package runnertypes

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expected, result)
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
			assert.Equal(t, tt.expected, result)
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
			cmd := &CommandSpec{
				RiskLevel: tt.maxRiskStr,
			}

			result, err := cmd.GetMaxRiskLevel()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expected, result)
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
			cmd := &CommandSpec{
				RunAsUser:  tt.runAsUser,
				RunAsGroup: tt.runAsGroup,
			}

			result := cmd.HasUserGroupSpecification()

			assert.Equal(t, tt.expected, result)
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
			name: "returns Inherit mode",
			resolver: newAllowlistResolution(
				InheritanceModeInherit,
				"test-group",
				map[string]struct{}{},
				map[string]struct{}{},
			),
			expected: InheritanceModeInherit,
		},
		{
			name: "returns Explicit mode",
			resolver: newAllowlistResolution(
				InheritanceModeExplicit,
				"test-group",
				map[string]struct{}{},
				map[string]struct{}{},
			),
			expected: InheritanceModeExplicit,
		},
		{
			name: "returns Reject mode",
			resolver: newAllowlistResolution(
				InheritanceModeReject,
				"test-group",
				map[string]struct{}{},
				map[string]struct{}{},
			),
			expected: InheritanceModeReject,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolver.GetMode()
			assert.Equal(t, tt.expected, result)
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
			name: "returns GroupName",
			resolver: newAllowlistResolution(
				InheritanceModeInherit,
				"test-group",
				map[string]struct{}{},
				map[string]struct{}{},
			),
			expected: "test-group",
		},
		{
			name: "returns empty GroupName",
			resolver: newAllowlistResolution(
				InheritanceModeInherit,
				"",
				map[string]struct{}{},
				map[string]struct{}{},
			),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.resolver.GetGroupName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAllowlistResolution_NilReceiverPanics tests that all getter methods panic on nil receiver
func TestAllowlistResolution_NilReceiverPanics(t *testing.T) {
	var nilResolver *AllowlistResolution

	t.Run("GetMode_panics_on_nil_receiver", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("GetMode() did not panic with nil receiver")
			}
		}()
		_ = nilResolver.GetMode()
	})

	t.Run("GetGroupName_panics_on_nil_receiver", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("GetGroupName() did not panic with nil receiver")
			}
		}()
		_ = nilResolver.GetGroupName()
	})

	t.Run("IsAllowed_panics_on_nil_receiver", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("IsAllowed() did not panic with nil receiver")
			}
		}()
		_ = nilResolver.IsAllowed("TEST_VAR")
	})
}
