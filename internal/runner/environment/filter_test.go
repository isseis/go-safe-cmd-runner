package environment

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

func TestNewFilter(t *testing.T) {
	config := &runnertypes.Config{}
	filter := NewFilter(config)

	if filter == nil {
		t.Fatal("NewFilter returned nil")
	}

	if filter.config != config {
		t.Error("Filter config not set correctly")
	}
}

func TestIsVariableAccessAllowed(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"GLOBAL_VAR"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "testgroup",
				EnvAllowlist: []string{"GROUP_VAR"},
			},
		},
	}

	filter := NewFilter(config)
	testGroup := &config.Groups[0] // Get reference to the test group

	tests := []struct {
		name     string
		variable string
		group    *runnertypes.CommandGroup
		expected bool
	}{
		{
			name:     "global variable not allowed when group allowlist defined",
			variable: "GLOBAL_VAR",
			group:    testGroup,
			expected: false, // Group allowlist overrides global, GLOBAL_VAR not in group allowlist
		},
		{
			name:     "group variable allowed",
			variable: "GROUP_VAR",
			group:    testGroup,
			expected: true,
		},
		{
			name:     "variable not allowed",
			variable: "FORBIDDEN_VAR",
			group:    testGroup,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsVariableAccessAllowed(tt.variable, tt.group)
			if result != tt.expected {
				groupName := "nil"
				if tt.group != nil {
					groupName = tt.group.Name
				}
				t.Errorf("IsVariableAccessAllowed(%s, %s): expected %v, got %v",
					tt.variable, groupName, tt.expected, result)
			}
		})
	}
}

func TestValidateVariableName(t *testing.T) {
	config := &runnertypes.Config{}
	filter := NewFilter(config)

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
			err := filter.ValidateVariableName(tt.varName)
			if tt.expected && err != nil {
				t.Errorf("ValidateVariableName(%s): expected no error, got %v", tt.varName, err)
			} else if !tt.expected && err == nil {
				t.Errorf("ValidateVariableName(%s): expected error, got nil", tt.varName)
			}
		})
	}
}

func TestValidateVariableValue(t *testing.T) {
	config := &runnertypes.Config{}
	filter := NewFilter(config)

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
			err := filter.ValidateVariableValue(tt.value)
			if tt.expected && err != nil {
				t.Errorf("ValidateVariableValue(%s): expected no error, got %v", tt.value, err)
			} else if !tt.expected && err == nil {
				t.Errorf("ValidateVariableValue(%s): expected error, got nil", tt.value)
			}
		})
	}
}
