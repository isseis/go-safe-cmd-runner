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

func TestFilterEnvFileVariablesGlobalAllowlist(t *testing.T) {
	envFileVars := map[string]string{
		"ENV_VAR1": "value1",
		"ENV_VAR2": "value2",
		"ENV_VAR3": "value3",
	}

	tests := []struct {
		name      string
		allowlist []string
		expected  map[string]string
	}{
		{
			name:      "empty allowlist",
			allowlist: []string{},
			expected:  map[string]string{},
		},
		{
			name:      "single allowed variable",
			allowlist: []string{"ENV_VAR1"},
			expected:  map[string]string{"ENV_VAR1": "value1"},
		},
		{
			name:      "multiple allowed variables",
			allowlist: []string{"ENV_VAR1", "ENV_VAR3"},
			expected:  map[string]string{"ENV_VAR1": "value1", "ENV_VAR3": "value3"},
		},
		{
			name:      "all variables allowed",
			allowlist: []string{"ENV_VAR1", "ENV_VAR2", "ENV_VAR3"},
			expected:  envFileVars,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: tt.allowlist,
				},
			}
			filter := NewFilter(config)
			result, err := filter.FilterEnvFileVariables(envFileVars, nil)
			if err != nil {
				t.Fatalf("FilterEnvFileVariables returned error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d variables, got %d", len(tt.expected), len(result))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected variable %s not found in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("Variable %s: expected %s, got %s", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestFilterEnvFileVariablesGroupAllowlist(t *testing.T) {
	config := &runnertypes.Config{}
	filter := NewFilter(config)

	envFileVars := map[string]string{
		"ENV_VAR1": "value1",
		"ENV_VAR2": "value2",
		"ENV_VAR3": "value3",
	}

	tests := []struct {
		name      string
		allowlist []string
		expected  map[string]string
	}{
		{
			name:      "empty allowlist",
			allowlist: []string{},
			expected:  map[string]string{},
		},
		{
			name:      "single allowed variable",
			allowlist: []string{"ENV_VAR1"},
			expected:  map[string]string{"ENV_VAR1": "value1"},
		},
		{
			name:      "multiple allowed variables",
			allowlist: []string{"ENV_VAR1", "ENV_VAR3"},
			expected:  map[string]string{"ENV_VAR1": "value1", "ENV_VAR3": "value3"},
		},
		{
			name:      "all variables allowed",
			allowlist: []string{"ENV_VAR1", "ENV_VAR2", "ENV_VAR3"},
			expected:  envFileVars,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filter.FilterEnvFileVariables(envFileVars, tt.allowlist)
			if err != nil {
				t.Fatalf("FilterEnvFileVariables returned error: %v", err)
			}

			if len(result) != len(tt.expected) {
				t.Errorf("Expected %d variables, got %d", len(tt.expected), len(result))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected variable %s not found in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("Variable %s: expected %s, got %s", key, expectedValue, actualValue)
				}
			}
		})
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

func TestFilterSystemEnvironment(t *testing.T) {
	tests := []struct {
		name            string
		envVars         map[string]string
		globalAllowlist []string
		expectedVars    []string
		notExpectedVars []string
	}{
		{
			name:            "filter with simple global allowlist",
			envVars:         map[string]string{"TEST_PATH": "/bin", "TEST_HOME": "/home/user", "SECRET": "value"},
			globalAllowlist: []string{"TEST_PATH", "TEST_HOME"},
			expectedVars:    []string{"TEST_PATH", "TEST_HOME"},
			notExpectedVars: []string{"SECRET"},
		},
		{
			name:            "empty global allowlist",
			envVars:         map[string]string{"TEST_PATH": "/bin", "TEST_HOME": "/home/user"},
			globalAllowlist: []string{},
			expectedVars:    []string{},
			notExpectedVars: []string{"TEST_PATH", "TEST_HOME"},
		},
		{
			name:            "no matching variables",
			envVars:         map[string]string{"SECRET": "value", "PRIVATE": "data"},
			globalAllowlist: []string{"NONEXISTENT1", "NONEXISTENT2"},
			expectedVars:    []string{},
			notExpectedVars: []string{"SECRET", "PRIVATE", "PATH", "HOME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test environment
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			config := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: tt.globalAllowlist,
				},
			}
			filter := NewFilter(config)
			result, err := filter.FilterSystemEnvironment()
			if err != nil {
				t.Errorf("FilterSystemEnvironment() error = %v", err)
				return
			}

			// Check expected variables are present
			for _, expectedVar := range tt.expectedVars {
				if _, exists := result[expectedVar]; !exists {
					t.Errorf("Expected variable %s not found in result", expectedVar)
				}
			}

			// Check unexpected variables are not present
			for _, notExpectedVar := range tt.notExpectedVars {
				if _, exists := result[notExpectedVar]; exists {
					t.Errorf("Unexpected variable %s found in result", notExpectedVar)
				}
			}

			// Verify only expected variables are present
			if len(result) != len(tt.expectedVars) {
				t.Errorf("Expected %d variables, got %d", len(tt.expectedVars), len(result))
			}
		})
	}
}

// TestFilterSystemEnvironmentSkipsValidation tests that FilterSystemEnvironment
// does not apply ValidateEnvironmentVariable and only uses allowlist filtering,
// even for environment variables with potentially dangerous values.
func TestFilterSystemEnvironmentSkipsValidation(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string // Environment variables with dangerous patterns
		allowlist    []string
		expectedVars []string
		description  string
	}{
		{
			name: "dangerous patterns allowed through allowlist",
			envVars: map[string]string{
				"DANGEROUS_CMD": "rm -rf /tmp",           // Contains 'rm ' pattern
				"INJECTION_VAR": "test; cat /etc/passwd", // Contains ';' pattern
				"REDIRECT_VAR":  "output > /tmp/file",    // Contains '>' pattern
				"SAFE_VAR":      "normal_value",          // Safe value
				"NOT_ALLOWED":   "safe_but_not_allowed",  // Safe but not in allowlist
			},
			allowlist:    []string{"DANGEROUS_CMD", "INJECTION_VAR", "REDIRECT_VAR", "SAFE_VAR"},
			expectedVars: []string{"DANGEROUS_CMD", "INJECTION_VAR", "REDIRECT_VAR", "SAFE_VAR"},
			description:  "All allowlisted variables should be included, regardless of dangerous patterns",
		},
		{
			name: "dangerous patterns filtered by allowlist only",
			envVars: map[string]string{
				"DANGEROUS_CMD": "rm -rf /tmp",
				"SAFE_VAR":      "normal_value",
			},
			allowlist:    []string{"SAFE_VAR"}, // Only safe var in allowlist
			expectedVars: []string{"SAFE_VAR"},
			description:  "Dangerous variables should be filtered by allowlist, not by validation",
		},
		{
			name: "command execution patterns in allowlisted vars",
			envVars: map[string]string{
				"EXEC_VAR":   "exec /bin/bash",
				"SYSTEM_VAR": "system('malicious')",
				"EVAL_VAR":   "eval(dangerous)",
			},
			allowlist:    []string{"EXEC_VAR", "SYSTEM_VAR", "EVAL_VAR"},
			expectedVars: []string{"EXEC_VAR", "SYSTEM_VAR", "EVAL_VAR"},
			description:  "Command execution patterns should pass through if allowlisted",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up test environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			config := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: tt.allowlist,
				},
			}
			filter := NewFilter(config)

			// Execute FilterSystemEnvironment
			result, err := filter.FilterSystemEnvironment()
			if err != nil {
				t.Errorf("FilterSystemEnvironment() should not return error for dangerous patterns, got: %v", err)
				return
			}

			// Verify expected variables are present with their dangerous values intact
			for _, expectedVar := range tt.expectedVars {
				if value, exists := result[expectedVar]; !exists {
					t.Errorf("Expected variable %s not found in result", expectedVar)
				} else if expectedValue := tt.envVars[expectedVar]; value != expectedValue {
					t.Errorf("Variable %s: expected value %q, got %q", expectedVar, expectedValue, value)
				}
			}

			// Verify that variables not in allowlist are not present
			for varName := range tt.envVars {
				inAllowlist := false
				for _, allowedVar := range tt.allowlist {
					if varName == allowedVar {
						inAllowlist = true
						break
					}
				}
				if !inAllowlist {
					if _, exists := result[varName]; exists {
						t.Errorf("Variable %s should not be present (not in allowlist)", varName)
					}
				}
			}

			// Verify only expected number of variables
			if len(result) != len(tt.expectedVars) {
				t.Errorf("Expected %d variables, got %d. Description: %s",
					len(tt.expectedVars), len(result), tt.description)
			}
		})
	}
}

// TestFilterSystemEnvironmentVsEnvFileValidationDifference demonstrates the security model difference:
// system environment variables skip validation while .env file variables require validation.
func TestFilterSystemEnvironmentVsEnvFileValidationDifference(t *testing.T) {
	dangerousValue := "rm -rf /tmp; cat /etc/passwd"
	varName := "DANGEROUS_VAR"

	// Set up system environment variable with dangerous value
	t.Setenv(varName, dangerousValue)

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{varName},
		},
	}
	filter := NewFilter(config)

	// Test 1: FilterSystemEnvironment should allow dangerous value through allowlist
	systemResult, err := filter.FilterSystemEnvironment()
	if err != nil {
		t.Errorf("FilterSystemEnvironment() should not validate dangerous patterns, got error: %v", err)
	}
	if value, exists := systemResult[varName]; !exists {
		t.Error("System environment variable with dangerous pattern should be allowed through allowlist")
	} else if value != dangerousValue {
		t.Errorf("System environment variable value should be preserved: expected %q, got %q", dangerousValue, value)
	}

	// Test 2: FilterEnvFileVariables should reject the same dangerous value
	envFileVars := map[string]string{
		varName: dangerousValue,
	}

	_, err = filter.FilterEnvFileVariables(envFileVars, []string{varName})
	if err == nil {
		t.Error("FilterEnvFileVariables() should reject dangerous patterns and return error")
	}

	t.Logf("Security model validation: System env vars trusted (allowed), .env vars validated (rejected)")
}
