package runnertypes

import (
	"encoding/json"
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

func TestInheritanceMode_MarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		mode     InheritanceMode
		expected string
	}{
		{
			name:     "InheritanceModeInherit",
			mode:     InheritanceModeInherit,
			expected: `"inherit"`,
		},
		{
			name:     "InheritanceModeExplicit",
			mode:     InheritanceModeExplicit,
			expected: `"explicit"`,
		},
		{
			name:     "InheritanceModeReject",
			mode:     InheritanceModeReject,
			expected: `"reject"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.mode)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, string(data))
		})
	}
}

func TestInheritanceMode_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected InheritanceMode
		wantErr  bool
	}{
		{
			name:     "inherit",
			input:    `"inherit"`,
			expected: InheritanceModeInherit,
			wantErr:  false,
		},
		{
			name:     "explicit",
			input:    `"explicit"`,
			expected: InheritanceModeExplicit,
			wantErr:  false,
		},
		{
			name:     "reject",
			input:    `"reject"`,
			expected: InheritanceModeReject,
			wantErr:  false,
		},
		{
			name:     "invalid value",
			input:    `"unknown_mode"`,
			expected: InheritanceModeInherit,
			wantErr:  true,
		},
		{
			name:     "empty string",
			input:    `""`,
			expected: InheritanceModeInherit,
			wantErr:  true,
		},
		{
			name:     "security test: log injection with newline",
			input:    `"bad_value\ninjected_log_line"`,
			expected: InheritanceModeInherit,
			wantErr:  true,
		},
		{
			name:     "security test: path traversal attempt",
			input:    `"../../../etc/passwd"`,
			expected: InheritanceModeInherit,
			wantErr:  true,
		},
		{
			name:     "security test: null byte injection",
			input:    `"badvalue\u0000injection"`,
			expected: InheritanceModeInherit,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mode InheritanceMode
			err := json.Unmarshal([]byte(tt.input), &mode)

			if tt.wantErr {
				assert.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidInheritanceMode)
				// Security check: error message should NOT contain the input value
				if err != nil {
					errMsg := err.Error()
					// Extract the input value without JSON quotes for checking
					var inputValue string
					_ = json.Unmarshal([]byte(tt.input), &inputValue)
					// Skip empty string check (would be a false positive)
					if inputValue != "" {
						// The error message should only contain the base error, not the input
						assert.NotContains(t, errMsg, inputValue,
							"error message should not contain user input to prevent log injection")
					}
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, mode)
			}
		})
	}
}
