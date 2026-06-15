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
			name:     "unknown is rejected in configuration",
			input:    "unknown",
			expected: RiskLevelUnknown,
			hasError: true,
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

// TestParseRiskLevel_UnknownError verifies that "unknown" is rejected as a
// configuration value with ErrInvalidRiskLevel.
func TestParseRiskLevel_UnknownError(t *testing.T) {
	level, err := ParseRiskLevel("unknown")
	assert.ErrorIs(t, err, ErrInvalidRiskLevel)
	assert.Equal(t, RiskLevelUnknown, level)
}

// TestParseRiskLevel_UnknownConfigRejected verifies that a command configured
// with risk_level="unknown" surfaces the error through GetRiskLevel.
func TestParseRiskLevel_UnknownConfigRejected(t *testing.T) {
	cmd := &CommandSpec{RiskLevel: StringPtr("unknown")}
	level, err := cmd.GetRiskLevel()
	assert.ErrorIs(t, err, ErrInvalidRiskLevel)
	assert.Equal(t, RiskLevelUnknown, level)
}

// TestParseRiskLevel_ValidValues verifies that the previously accepted values
// (low/medium/high and omitted/empty) keep parsing unchanged.
func TestParseRiskLevel_ValidValues(t *testing.T) {
	tests := []struct {
		input    string
		expected RiskLevel
	}{
		{"low", RiskLevelLow},
		{"medium", RiskLevelMedium},
		{"high", RiskLevelHigh},
		{"", RiskLevelLow}, // empty/omitted defaults to low
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			level, err := ParseRiskLevel(tt.input)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, level)
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

func TestCommandGetRiskLevel(t *testing.T) {
	tests := []struct {
		name        string
		riskStr     string
		expected    RiskLevel
		expectError bool
	}{
		{
			name:        "unknown is rejected in configuration",
			riskStr:     "unknown",
			expected:    RiskLevelUnknown,
			expectError: true,
		},
		{
			name:        "valid low risk",
			riskStr:     "low",
			expected:    RiskLevelLow,
			expectError: false,
		},
		{
			name:        "valid medium risk",
			riskStr:     "medium",
			expected:    RiskLevelMedium,
			expectError: false,
		},
		{
			name:        "valid high risk",
			riskStr:     "high",
			expected:    RiskLevelHigh,
			expectError: false,
		},
		{
			name:        "empty defaults to low",
			riskStr:     "",
			expected:    RiskLevelLow,
			expectError: false,
		},
		{
			name:        "invalid risk level",
			riskStr:     "invalid",
			expected:    RiskLevelUnknown,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &CommandSpec{
				RiskLevel: StringPtr(tt.riskStr),
			}

			result, err := cmd.GetRiskLevel()

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
				errMsg := err.Error()
				// Extract the input value without JSON quotes for checking
				var inputValue string
				unmarshalErr := json.Unmarshal([]byte(tt.input), &inputValue)
				assert.NoError(t, unmarshalErr, "test case input must be a valid JSON string")
				// Skip empty string check (would be a false positive)
				if inputValue != "" {
					// The error message should only contain the base error, not the input
					assert.NotContains(t, errMsg, inputValue,
						"error message should not contain user input to prevent log injection")
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, mode)
			}
		})
	}
}
