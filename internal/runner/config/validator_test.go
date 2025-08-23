package config

import (
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigValidator(t *testing.T) {
	validator := NewConfigValidator()
	assert.NotNil(t, validator)
	assert.NotNil(t, validator.logger)
	// Security validator might be nil if creation fails, that's okay
}

func TestConfigValidator_ValidateConfig(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name           string
		config         *runnertypes.Config
		expectValid    bool
		expectedErrors int
	}{
		{
			name: "valid configuration",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{"PATH", "HOME"},
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name:         "test_group",
						EnvAllowlist: []string{"GROUP_VAR"},
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd",
								Cmd:  "/bin/echo",
								Args: []string{"hello"},
							},
						},
					},
				},
			},
			expectValid:    true,
			expectedErrors: 0,
		},
		{
			name: "invalid configuration - empty group name",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{"PATH"},
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "", // Empty name should cause error
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectValid:    false,
			expectedErrors: 1,
		},
		{
			name: "invalid configuration - empty command name",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{"PATH"},
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Commands: []runnertypes.Command{
							{
								Name: "", // Empty command name should cause error
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectValid:    false,
			expectedErrors: 1,
		},
		{
			name: "invalid configuration - invalid variable name",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{"123INVALID"}, // Invalid variable name
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectValid:    false,
			expectedErrors: 1,
		},
		{
			name: "invalid configuration - duplicate group names",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{"PATH"},
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "duplicate_group",
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd1",
								Cmd:  "/bin/echo",
							},
						},
					},
					{
						Name: "duplicate_group", // Duplicate name should cause error
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd2",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectValid:    false,
			expectedErrors: 1,
		},
		{
			name: "valid configuration - empty group names are not considered duplicates",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{"PATH"},
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "", // Empty name
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd1",
								Cmd:  "/bin/echo",
							},
						},
					},
					{
						Name: "", // Another empty name - should not be considered duplicate
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd2",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectValid:    false,
			expectedErrors: 2, // Two empty group name errors, but no duplicate error
		},
		{
			name: "invalid configuration - multiple duplicate groups",
			config: &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{"PATH"},
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "group_a",
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd1",
								Cmd:  "/bin/echo",
							},
						},
					},
					{
						Name: "group_b",
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd2",
								Cmd:  "/bin/echo",
							},
						},
					},
					{
						Name: "group_a", // Duplicate of first group
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd3",
								Cmd:  "/bin/echo",
							},
						},
					},
					{
						Name: "group_b", // Duplicate of second group
						Commands: []runnertypes.Command{
							{
								Name: "test_cmd4",
								Cmd:  "/bin/echo",
							},
						},
					},
				},
			},
			expectValid:    false,
			expectedErrors: 2, // Two duplicate group name errors
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validator.ValidateConfig(tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.expectValid, result.Valid)
			assert.Len(t, result.Errors, tt.expectedErrors)
			assert.NotZero(t, result.Timestamp)
		})
	}
}

func TestConfigValidator_ValidateGlobalConfig(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name             string
		global           *runnertypes.GlobalConfig
		expectedWarnings int
		expectedErrors   int
	}{
		{
			name: "valid global config",
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{"PATH", "HOME", "USER"},
			},
			expectedWarnings: 0,
			expectedErrors:   0,
		},
		{
			name: "empty global allowlist",
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{},
			},
			expectedWarnings: 1, // Should warn about empty allowlist
			expectedErrors:   0,
		},
		{
			name: "dangerous variables in global allowlist",
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{"PATH", "LD_PRELOAD"},
			},
			expectedWarnings: 1, // Should warn about dangerous variable
			expectedErrors:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{
				Errors:   []ValidationError{},
				Warnings: []ValidationWarning{},
			}

			validator.validateGlobalConfig(tt.global, result)
			assert.Len(t, result.Warnings, tt.expectedWarnings)
			assert.Len(t, result.Errors, tt.expectedErrors)
		})
	}
}

func TestConfigValidator_ValidateAllowlist(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name             string
		allowlist        []string
		expectedErrors   int
		expectedWarnings int
	}{
		{
			name:             "valid allowlist",
			allowlist:        []string{"PATH", "HOME", "USER"},
			expectedErrors:   0,
			expectedWarnings: 0,
		},
		{
			name:             "empty variable name",
			allowlist:        []string{"PATH", "", "HOME"},
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name:             "invalid variable name",
			allowlist:        []string{"PATH", "123INVALID", "HOME"},
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name:             "duplicate variables",
			allowlist:        []string{"PATH", "HOME", "PATH"},
			expectedErrors:   0,
			expectedWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{
				Errors:   []ValidationError{},
				Warnings: []ValidationWarning{},
			}

			validator.validateAllowlist(tt.allowlist, "test.allowlist", result)
			assert.Len(t, result.Errors, tt.expectedErrors)
			assert.Len(t, result.Warnings, tt.expectedWarnings)
		})
	}
}

func TestConfigValidator_ValidateVariableName(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name        string
		varName     string
		expectError bool
	}{
		{
			name:        "valid variable name",
			varName:     "VALID_VAR",
			expectError: false,
		},
		{
			name:        "valid variable name with underscore start",
			varName:     "_VALID",
			expectError: false,
		},
		{
			name:        "valid variable name with numbers",
			varName:     "VAR123",
			expectError: false,
		},
		{
			name:        "empty variable name",
			varName:     "",
			expectError: true,
		},
		{
			name:        "variable name starting with digit",
			varName:     "123INVALID",
			expectError: true,
		},
		{
			name:        "variable name with invalid characters",
			varName:     "VAR-INVALID",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateVariableName(tt.varName)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigValidator_ValidateCommandEnv(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name             string
		env              []string
		expectedErrors   int
		expectedWarnings int
	}{
		{
			name:             "valid command env",
			env:              []string{"FOO=bar", "BAZ=qux"},
			expectedErrors:   0,
			expectedWarnings: 0,
		},
		{
			name:             "invalid env format",
			env:              []string{"INVALID_NO_EQUALS", "VALID=value"},
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name:             "invalid variable name",
			env:              []string{"123INVALID=value"},
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name:             "duplicate variables",
			env:              []string{"FOO=bar", "FOO=baz"},
			expectedErrors:   0,
			expectedWarnings: 1,
		},
		{
			name:             "dangerous variable value",
			env:              []string{"DANGEROUS=value; rm -rf /"},
			expectedErrors:   0,
			expectedWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{
				Errors:   []ValidationError{},
				Warnings: []ValidationWarning{},
			}

			validator.validateCommandEnv(tt.env, "test.env", result)
			assert.Len(t, result.Errors, tt.expectedErrors)
			assert.Len(t, result.Warnings, tt.expectedWarnings)
		})
	}
}

func TestConfigValidator_ValidateVariableValue(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name        string
		value       string
		expectError bool
	}{
		{
			name:        "safe value",
			value:       "safe_value",
			expectError: false,
		},
		{
			name:        "value with semicolon",
			value:       "value; echo hello",
			expectError: true,
		},
		{
			name:        "value with command substitution",
			value:       "value$(echo hello)",
			expectError: true,
		},
		{
			name:        "value with pipe",
			value:       "value | grep something",
			expectError: true,
		},
		{
			name:        "value with redirection",
			value:       "value > /tmp/file",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.validateVariableValue(tt.value)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigValidator_AnalyzeInheritanceMode(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name             string
		group            *runnertypes.CommandGroup
		global           *runnertypes.GlobalConfig
		expectedWarnings int
	}{
		{
			name: "inherit mode with non-empty global",
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: nil, // inherit mode
				Commands:     []runnertypes.Command{},
			},
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{"PATH", "HOME"},
			},
			expectedWarnings: 0,
		},
		{
			name: "inherit mode with empty global",
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: nil, // inherit mode
				Commands:     []runnertypes.Command{},
			},
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{}, // empty global allowlist
			},
			expectedWarnings: 1,
		},
		{
			name: "reject mode with command env",
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{}, // reject mode
				Commands: []runnertypes.Command{
					{
						Name: "test_cmd",
						Env:  []string{"FOO=bar"}, // has command env
					},
				},
			},
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{"PATH"},
			},
			expectedWarnings: 1,
		},
		{
			name: "explicit mode",
			group: &runnertypes.CommandGroup{
				Name:         "test_group",
				EnvAllowlist: []string{"GROUP_VAR"}, // explicit mode
				Commands:     []runnertypes.Command{},
			},
			global: &runnertypes.GlobalConfig{
				EnvAllowlist: []string{"PATH"},
			},
			expectedWarnings: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{
				Warnings: []ValidationWarning{},
			}

			validator.analyzeInheritanceMode(tt.group, "test.location", tt.global, result)
			assert.Len(t, result.Warnings, tt.expectedWarnings)
		})
	}
}

func TestConfigValidator_CalculateSummary(t *testing.T) {
	validator := NewConfigValidator()

	config := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"PATH", "HOME"},
		},
		Groups: []runnertypes.CommandGroup{
			{
				Name:         "group1",
				EnvAllowlist: []string{"GROUP_VAR"}, // explicit allowlist
				Commands: []runnertypes.Command{
					{
						Name: "cmd1",
						Env:  []string{"FOO=bar"}, // has env
					},
					{
						Name: "cmd2",
						Env:  []string{}, // no env
					},
				},
			},
			{
				Name:         "group2",
				EnvAllowlist: nil, // inherit mode
				Commands: []runnertypes.Command{
					{
						Name: "cmd3",
						Env:  []string{"BAZ=qux"}, // has env
					},
				},
			},
		},
	}

	result := &ValidationResult{}
	validator.calculateSummary(config, result)

	assert.Equal(t, 2, result.Summary.TotalGroups)
	assert.Equal(t, 1, result.Summary.GroupsWithAllowlist) // only group1 has explicit allowlist
	assert.Equal(t, 2, result.Summary.GlobalAllowlistSize)
	assert.Equal(t, 3, result.Summary.TotalCommands)
	assert.Equal(t, 2, result.Summary.CommandsWithEnv) // cmd1 and cmd3
}

func TestConfigValidator_GenerateValidationReport(t *testing.T) {
	validator := NewConfigValidator()

	result := &ValidationResult{
		Valid:     false,
		Timestamp: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Summary: ValidationSummary{
			TotalGroups:         2,
			GroupsWithAllowlist: 1,
			GlobalAllowlistSize: 3,
			TotalCommands:       5,
			CommandsWithEnv:     2,
		},
		Errors: []ValidationError{
			{
				Type:     "test_error",
				Message:  "Test error message",
				Location: "test.location",
				Severity: "error",
			},
		},
		Warnings: []ValidationWarning{
			{
				Type:       "test_warning",
				Message:    "Test warning message",
				Location:   "test.location",
				Suggestion: "Test suggestion",
			},
		},
	}

	report, err := validator.GenerateValidationReport(result)
	require.NoError(t, err)

	assert.Contains(t, report, "Configuration Validation Report")
	assert.Contains(t, report, "Overall Status: INVALID")
	assert.Contains(t, report, "Total Groups: 2")
	assert.Contains(t, report, "Test error message")
	assert.Contains(t, report, "Test warning message")
	assert.Contains(t, report, "Test suggestion")
}

func TestConfigValidator_GetStatusString(t *testing.T) {
	validator := NewConfigValidator()

	assert.Equal(t, "VALID", validator.getStatusString(true))
	assert.Equal(t, "INVALID", validator.getStatusString(false))
}

func TestConfigValidator_ValidateWorkingDirectory(t *testing.T) {
	validator := NewConfigValidator()

	tests := []struct {
		name             string
		path             string
		expectedErrors   int
		expectedWarnings int
	}{
		{
			name:             "valid working directory",
			path:             "/home/user/project",
			expectedErrors:   0,
			expectedWarnings: 0,
		},
		{
			name:             "empty working directory",
			path:             "",
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name:             "whitespace only working directory",
			path:             "   ",
			expectedErrors:   1,
			expectedWarnings: 0,
		},
		{
			name:             "dangerous working directory - root",
			path:             "/",
			expectedErrors:   0,
			expectedWarnings: 1,
		},
		{
			name:             "dangerous working directory - tmp",
			path:             "/tmp",
			expectedErrors:   0,
			expectedWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := &ValidationResult{
				Errors:   []ValidationError{},
				Warnings: []ValidationWarning{},
			}

			validator.validateWorkingDirectory(tt.path, "test.workdir", result)
			assert.Len(t, result.Errors, tt.expectedErrors)
			assert.Len(t, result.Warnings, tt.expectedWarnings)
		})
	}
}
