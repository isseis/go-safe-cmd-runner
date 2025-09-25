package environment

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFilter(t *testing.T) {
	config := &runnertypes.Config{}
	filter := NewFilter(config)

	require.NotNil(t, filter, "NewFilter returned nil")

	assert.Equal(t, config, filter.config, "Filter config not set correctly")
}

func TestDetermineInheritanceMode(t *testing.T) {
	tests := []struct {
		name         string
		group        *runnertypes.CommandGroup
		expectedMode runnertypes.InheritanceMode
		expectError  bool
	}{
		{
			name:        "nil group should return error",
			group:       nil,
			expectError: true,
		},
		{
			name: "nil allowlist should inherit",
			group: &runnertypes.CommandGroup{
				Name:         "test",
				EnvAllowlist: nil,
			},
			expectedMode: runnertypes.InheritanceModeInherit,
		},
		{
			name: "empty allowlist should reject",
			group: &runnertypes.CommandGroup{
				Name:         "test",
				EnvAllowlist: []string{},
			},
			expectedMode: runnertypes.InheritanceModeReject,
		},
		{
			name: "non-empty allowlist should be explicit",
			group: &runnertypes.CommandGroup{
				Name:         "test",
				EnvAllowlist: []string{"VAR1", "VAR2"},
			},
			expectedMode: runnertypes.InheritanceModeExplicit,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := NewFilter(&runnertypes.Config{})
			mode, err := filter.determineInheritanceMode(tt.group)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedMode, mode)
		})
	}
}

func TestIsVariableAccessAllowedWithInheritance(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"GLOBAL_VAR", "COMMON_VAR"},
		},
	}

	// Groups for testing different inheritance modes
	groupInherit := &runnertypes.CommandGroup{
		Name:         "group-inherit",
		EnvAllowlist: nil, // Inherit from global
	}
	groupExplicit := &runnertypes.CommandGroup{
		Name:         "group-explicit",
		EnvAllowlist: []string{"GROUP_VAR", "COMMON_VAR"}, // Explicit allowlist
	}
	groupReject := &runnertypes.CommandGroup{
		Name:         "group-reject",
		EnvAllowlist: []string{}, // Reject all
	}
	groupNil := (*runnertypes.CommandGroup)(nil)

	filter := NewFilter(config)

	tests := []struct {
		name     string
		variable string
		group    *runnertypes.CommandGroup
		expected bool
	}{
		// --- InheritanceModeInherit ---
		{
			name:     "[Inherit] Allowed global variable",
			variable: "GLOBAL_VAR",
			group:    groupInherit,
			expected: true,
		},
		{
			name:     "[Inherit] Allowed common variable",
			variable: "COMMON_VAR",
			group:    groupInherit,
			expected: true,
		},
		{
			name:     "[Inherit] Disallowed group-specific variable",
			variable: "GROUP_VAR",
			group:    groupInherit,
			expected: false,
		},
		{
			name:     "[Inherit] Disallowed undefined variable",
			variable: "UNDEFINED_VAR",
			group:    groupInherit,
			expected: false,
		},

		// --- InheritanceModeExplicit ---
		{
			name:     "[Explicit] Disallowed global variable",
			variable: "GLOBAL_VAR",
			group:    groupExplicit,
			expected: false,
		},
		{
			name:     "[Explicit] Allowed group-specific variable",
			variable: "GROUP_VAR",
			group:    groupExplicit,
			expected: true,
		},
		{
			name:     "[Explicit] Allowed common variable",
			variable: "COMMON_VAR",
			group:    groupExplicit,
			expected: true,
		},
		{
			name:     "[Explicit] Disallowed undefined variable",
			variable: "UNDEFINED_VAR",
			group:    groupExplicit,
			expected: false,
		},

		// --- InheritanceModeReject ---
		{
			name:     "[Reject] Disallowed global variable",
			variable: "GLOBAL_VAR",
			group:    groupReject,
			expected: false,
		},
		{
			name:     "[Reject] Disallowed group-specific variable",
			variable: "GROUP_VAR",
			group:    groupReject,
			expected: false,
		},
		{
			name:     "[Reject] Disallowed common variable",
			variable: "COMMON_VAR",
			group:    groupReject,
			expected: false,
		},
		{
			name:     "[Reject] Disallowed undefined variable",
			variable: "UNDEFINED_VAR",
			group:    groupReject,
			expected: false,
		},

		// --- Edge Case: Nil Group ---
		{
			name:     "Nil group should always deny access",
			variable: "ANY_VAR",
			group:    groupNil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsVariableAccessAllowed(tt.variable, tt.group)
			groupName := "nil"
			if tt.group != nil {
				groupName = tt.group.Name
			}
			assert.Equal(t, tt.expected, result, "IsVariableAccessAllowed(%s, %s)", tt.variable, groupName)
		})
	}
}

func TestValidateVariableName(t *testing.T) {
	tests := []struct {
		name     string
		varName  string
		expected bool
	}{
		{
			name:     "valid name with letters",
			varName:  "VALID_NAME",
			expected: true,
		},
		{
			name:     "valid name starting with underscore",
			varName:  "_VALID_NAME",
			expected: true,
		},
		{
			name:     "valid name with numbers",
			varName:  "VAR_123",
			expected: true,
		},
		{
			name:     "empty name",
			varName:  "",
			expected: false,
		},
		{
			name:     "name starting with number",
			varName:  "123_VAR",
			expected: false,
		},
		{
			name:     "name with special characters",
			varName:  "VAR-NAME",
			expected: false,
		},
		{
			name:     "name with spaces",
			varName:  "VAR NAME",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := ValidateVariableName(tt.varName)
			if tt.expected {
				assert.True(t, isValid, "ValidateVariableName(%s): expected true", tt.varName)
			} else {
				assert.False(t, isValid, "ValidateVariableName(%s): expected false", tt.varName)
			}
		})
	}
}

func TestValidateVariableValue(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected bool
	}{
		// Safe values
		{
			name:     "simple safe value",
			value:    "safe_value_123",
			expected: true,
		},
		{
			name:     "empty value",
			value:    "",
			expected: true,
		},

		// Command injection patterns
		{
			name:     "value with semicolon",
			value:    "value;dangerous",
			expected: false,
		},
		{
			name:     "value with double ampersand",
			value:    "value && dangerous",
			expected: false,
		},
		{
			name:     "value with double pipe",
			value:    "value || dangerous",
			expected: false,
		},
		{
			name:     "value with pipe",
			value:    "value | dangerous",
			expected: false,
		},
		{
			name:     "value with command substitution",
			value:    "value$(rm -rf /)",
			expected: false,
		},
		{
			name:     "value with backticks",
			value:    "value`rm -rf /`",
			expected: false,
		},

		// Redirection patterns
		{
			name:     "value with greater than",
			value:    "value > file",
			expected: false,
		},
		{
			name:     "value with less than",
			value:    "value < file",
			expected: false,
		},

		// Destructive file operations
		{
			name:     "value with rm command",
			value:    "rm -rf /tmp",
			expected: false,
		},
		{
			name:     "value with del command",
			value:    "del /s /q *",
			expected: false,
		},
		{
			name:     "value with format command",
			value:    "format C:",
			expected: false,
		},
		{
			name:     "value with mkfs command",
			value:    "mkfs /dev/sda1",
			expected: false,
		},
		{
			name:     "value with mkfs. prefix",
			value:    "mkfs.ext4 /dev/sda1",
			expected: false,
		},
		{
			name:     "value with dd if",
			value:    "dd if=/dev/zero of=/dev/sda",
			expected: false,
		},
		{
			name:     "value with dd of",
			value:    "dd if=source of=destination",
			expected: false,
		},

		// Code execution patterns
		{
			name:     "value with exec",
			value:    "exec /bin/sh",
			expected: false,
		},
		{
			name:     "value with system",
			value:    "system('rm -rf /')",
			expected: false,
		},
		{
			name:     "value with eval",
			value:    "eval('dangerous code')",
			expected: false,
		},

		// Edge cases
		{
			name:     "value with partial match at start",
			value:    "rmdir safe",
			expected: true,
		},
		{
			name:     "value with partial match in middle",
			value:    "some_rm_command",
			expected: true,
		},
		{
			name:     "value with case sensitivity check",
			value:    "RM -RF /",
			expected: true, // Assuming case-sensitive matching
		},
		// False positives
		{
			name:     "value with HTML tag",
			value:    "<div></div>",
			expected: false, // Should be true, but false positives are allowed for now
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid := ValidateVariableValue(tt.value)
			if tt.expected {
				assert.True(t, isValid, "ValidateVariableValue(%s): expected true", tt.value)
			} else {
				assert.False(t, isValid, "ValidateVariableValue(%s): expected false", tt.value)
			}
		})
	}
}
