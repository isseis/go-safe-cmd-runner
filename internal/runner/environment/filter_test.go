package environment

import (
	"os"
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

func TestFilterSystemEnvironmentGlobalAllowlist(t *testing.T) {
	// Set test environment variables
	os.Setenv("TEST_VAR1", "value1")
	os.Setenv("TEST_VAR2", "value2")
	os.Setenv("TEST_VAR3", "value3")
	defer func() {
		os.Unsetenv("TEST_VAR1")
		os.Unsetenv("TEST_VAR2")
		os.Unsetenv("TEST_VAR3")
	}()

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
			allowlist: []string{"TEST_VAR1"},
			expected:  map[string]string{"TEST_VAR1": "value1"},
		},
		{
			name:      "multiple allowed variables",
			allowlist: []string{"TEST_VAR1", "TEST_VAR2"},
			expected:  map[string]string{"TEST_VAR1": "value1", "TEST_VAR2": "value2"},
		},
		{
			name:      "non-existent variable in allowlist",
			allowlist: []string{"NON_EXISTENT"},
			expected:  map[string]string{},
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
			result, err := filter.FilterSystemEnvironment(nil)
			if err != nil {
				t.Fatalf("FilterSystemEnvironment returned error: %v", err)
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected variable %s not found in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("Variable %s: expected %s, got %s", key, expectedValue, actualValue)
				}
			}

			// Check that no unexpected variables are present
			for key := range result {
				if _, expected := tt.expected[key]; !expected {
					t.Errorf("Unexpected variable %s found in result", key)
				}
			}
		})
	}
}

func TestFilterSystemEnvironmentGroupAllowlist(t *testing.T) {
	config := &runnertypes.Config{}
	filter := NewFilter(config)

	// Set test environment variables
	os.Setenv("TEST_VAR1", "value1")
	os.Setenv("TEST_VAR2", "value2")
	os.Setenv("TEST_VAR3", "value3")
	defer func() {
		os.Unsetenv("TEST_VAR1")
		os.Unsetenv("TEST_VAR2")
		os.Unsetenv("TEST_VAR3")
	}()

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
			allowlist: []string{"TEST_VAR1"},
			expected:  map[string]string{"TEST_VAR1": "value1"},
		},
		{
			name:      "multiple allowed variables",
			allowlist: []string{"TEST_VAR1", "TEST_VAR2"},
			expected:  map[string]string{"TEST_VAR1": "value1", "TEST_VAR2": "value2"},
		},
		{
			name:      "non-existent variable in allowlist",
			allowlist: []string{"NON_EXISTENT"},
			expected:  map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := filter.FilterSystemEnvironment(tt.allowlist)
			if err != nil {
				t.Fatalf("FilterSystemEnvironment returned error: %v", err)
			}

			for key, expectedValue := range tt.expected {
				if actualValue, exists := result[key]; !exists {
					t.Errorf("Expected variable %s not found in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("Variable %s: expected %s, got %s", key, expectedValue, actualValue)
				}
			}

			// Check that no unexpected variables are present
			for key := range result {
				if _, expected := tt.expected[key]; !expected {
					t.Errorf("Unexpected variable %s found in result", key)
				}
			}
		})
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

func TestBuildAllowedVariableMaps(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"GLOBAL_VAR1", "GLOBAL_VAR2"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "group1",
				EnvAllowlist: []string{"GROUP1_VAR1", "GROUP1_VAR2"},
			},
			{
				Name:         "group2",
				EnvAllowlist: []string{"GROUP2_VAR1"},
			},
			{
				Name: "group3",
				// EnvAllowlist is nil - should inherit global
			},
		},
	}

	filter := NewFilter(config)
	result := filter.BuildAllowedVariableMaps()

	// Check group1 - should only have group-level allowlist (global ignored)
	group1Allowlist := result["group1"]
	expectedGroup1 := []string{"GROUP1_VAR1", "GROUP1_VAR2"}
	if len(group1Allowlist) != len(expectedGroup1) {
		t.Errorf("Group1 allowlist length: expected %d, got %d", len(expectedGroup1), len(group1Allowlist))
	}
	for _, expectedVar := range expectedGroup1 {
		found := false
		for _, actualVar := range group1Allowlist {
			if actualVar == expectedVar {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected variable %s not found in group1 allowlist", expectedVar)
		}
	}

	// Check group2 - should only have group-level allowlist (global ignored)
	group2Allowlist := result["group2"]
	expectedGroup2 := []string{"GROUP2_VAR1"}
	if len(group2Allowlist) != len(expectedGroup2) {
		t.Errorf("Group2 allowlist length: expected %d, got %d", len(expectedGroup2), len(group2Allowlist))
	}
	for _, expectedVar := range expectedGroup2 {
		found := false
		for _, actualVar := range group2Allowlist {
			if actualVar == expectedVar {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected variable %s not found in group2 allowlist", expectedVar)
		}
	}

	// Check group3 - should inherit global allowlist (EnvAllowlist is nil)
	group3Allowlist := result["group3"]
	expectedGroup3 := []string{"GLOBAL_VAR1", "GLOBAL_VAR2"}
	if len(group3Allowlist) != len(expectedGroup3) {
		t.Errorf("Group3 allowlist length: expected %d, got %d", len(expectedGroup3), len(group3Allowlist))
	}
	for _, expectedVar := range expectedGroup3 {
		found := false
		for _, actualVar := range group3Allowlist {
			if actualVar == expectedVar {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected variable %s not found in group3 allowlist", expectedVar)
		}
	}
}

func TestIsVariableAccessAllowedGlobalOnly(t *testing.T) {
	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"GLOBAL_VAR1", "GLOBAL_VAR2"},
		},
	}

	filter := NewFilter(config)

	tests := []struct {
		name     string
		variable string
		expected bool
	}{
		{
			name:     "global variable allowed",
			variable: "GLOBAL_VAR1",
			expected: true,
		},
		{
			name:     "global variable allowed 2",
			variable: "GLOBAL_VAR2",
			expected: true,
		},
		{
			name:     "variable not in global allowlist",
			variable: "NOT_ALLOWED",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filter.IsVariableAccessAllowed(tt.variable, nil)
			if result != tt.expected {
				t.Errorf("IsVariableAccessAllowed(%s, nil): expected %v, got %v",
					tt.variable, tt.expected, result)
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
		{
			name:     "nil group uses global allowlist",
			variable: "GLOBAL_VAR",
			group:    nil,
			expected: true,
		},
		{
			name:     "nil group rejects non-global var",
			variable: "GROUP_VAR",
			group:    nil,
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
