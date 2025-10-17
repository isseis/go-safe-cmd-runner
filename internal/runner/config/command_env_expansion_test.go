// Package config provides tests for command-level env expansion functionality.
package config_test

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandEnvExpansion_Basic tests basic command-level env expansion
func TestCommandEnvExpansion_Basic(t *testing.T) {
	tests := []struct {
		name        string
		commandEnv  []string          // Command-level env
		commandVars []string          // Command-level vars
		groupVars   []string          // Group-level vars
		globalVars  []string          // Global-level vars (from from_env)
		allowlist   []string          // Environment allowlist
		systemEnv   map[string]string // System environment variables
		expected    map[string]string // Expected final environment (key=value)
		expectError bool
	}{
		{
			name:        "Basic command env expansion - literal value",
			commandEnv:  []string{"CMD_VAR=literal_value"},
			commandVars: []string{},
			groupVars:   []string{},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    map[string]string{"CMD_VAR": "literal_value"},
			expectError: false,
		},
		{
			name:        "Command env referencing command vars",
			commandEnv:  []string{"RESULT=%{internal_var}"},
			commandVars: []string{"internal_var=command_value"},
			groupVars:   []string{},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    map[string]string{"RESULT": "command_value"},
			expectError: false,
		},
		{
			name:        "Command env referencing group vars",
			commandEnv:  []string{"RESULT=%{group_var}"},
			commandVars: []string{},
			groupVars:   []string{"group_var=group_value"},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    map[string]string{"RESULT": "group_value"},
			expectError: false,
		},
		{
			name:        "Command env referencing global vars",
			commandEnv:  []string{"RESULT=%{global_var}"},
			commandVars: []string{},
			groupVars:   []string{},
			globalVars:  []string{"global_var=HOME"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expected:    map[string]string{"RESULT": "/home/user"},
			expectError: false,
		},
		{
			name:        "Multiple command env definitions",
			commandEnv:  []string{"VAR1=%{x}", "VAR2=%{y}"},
			commandVars: []string{"x=value1", "y=value2"},
			groupVars:   []string{},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    map[string]string{"VAR1": "value1", "VAR2": "value2"},
			expectError: false,
		},
		{
			name:        "Nested references - command vars referencing other vars",
			commandEnv:  []string{"FINAL=%{cmd_var}"},
			commandVars: []string{"cmd_var=%{grp_var}"},
			groupVars:   []string{"grp_var=%{global_var}"},
			globalVars:  []string{"global_var=HOME"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/test"},
			expected:    map[string]string{"FINAL": "/home/test"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create configuration
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					FromEnv:      tt.globalVars,
					EnvAllowlist: tt.allowlist,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Vars: tt.groupVars,
						Commands: []runnertypes.Command{
							{
								Name: "test_command",
								Cmd:  "/bin/echo",
								Vars: tt.commandVars,
								Env:  tt.commandEnv,
							},
						},
					},
				},
			}

			// Create environment filter
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)

			// Expand global config first
			err := config.ExpandGlobalConfig(&cfg.Global, filter)
			require.NoError(t, err)

			// Expand group config
			err = config.ExpandGroupConfig(&cfg.Groups[0], &cfg.Global, filter)
			require.NoError(t, err)

			// Expand command config
			err = config.ExpandCommandConfig(&cfg.Groups[0].Commands[0], &cfg.Groups[0], &cfg.Global, filter)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			// Verify expanded environment variables
			expandedEnv := cfg.Groups[0].Commands[0].ExpandedEnv
			for key, expectedValue := range tt.expected {
				actualValue, found := expandedEnv[key]
				assert.True(t, found, "Expected environment variable %s not found", key)
				if found {
					assert.Equal(t, expectedValue, actualValue, "Environment variable %s has incorrect value", key)
				}
			}
		})
	}
}

// TestCommandEnvExpansion_Priority tests environment variable priority
func TestCommandEnvExpansion_Priority(t *testing.T) {
	tests := []struct {
		name        string
		commandEnv  []string
		commandVars []string
		groupVars   []string
		globalVars  []string
		allowlist   []string
		systemEnv   map[string]string
		expected    map[string]string
	}{
		{
			name:        "Command vars override group vars",
			commandEnv:  []string{"RESULT=%{var}"},
			commandVars: []string{"var=command_level"},
			groupVars:   []string{"var=group_level"},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expected:    map[string]string{"RESULT": "command_level"},
		},
		{
			name:        "Command vars override global vars",
			commandEnv:  []string{"RESULT=%{var}"},
			commandVars: []string{"var=command_level"},
			groupVars:   []string{},
			globalVars:  []string{"var=HOME"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expected:    map[string]string{"RESULT": "command_level"},
		},
		{
			name:        "Group vars override global vars",
			commandEnv:  []string{"RESULT=%{var}"},
			commandVars: []string{},
			groupVars:   []string{"var=group_level"},
			globalVars:  []string{"var=HOME"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/user"},
			expected:    map[string]string{"RESULT": "group_level"},
		},
		{
			name:        "Priority test with different variable names",
			commandEnv:  []string{"A=%{cmd_var}", "B=%{grp_var}", "C=%{global_var}"},
			commandVars: []string{"cmd_var=cmd"},
			groupVars:   []string{"grp_var=grp"},
			globalVars:  []string{"global_var=HOME"},
			allowlist:   []string{"HOME"},
			systemEnv:   map[string]string{"HOME": "/home/test"},
			expected:    map[string]string{"A": "cmd", "B": "grp", "C": "/home/test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create configuration
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					FromEnv:      tt.globalVars,
					EnvAllowlist: tt.allowlist,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Vars: tt.groupVars,
						Commands: []runnertypes.Command{
							{
								Name: "test_command",
								Cmd:  "/bin/echo",
								Vars: tt.commandVars,
								Env:  tt.commandEnv,
							},
						},
					},
				},
			}

			// Create environment filter
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)

			// Expand global config first
			err := config.ExpandGlobalConfig(&cfg.Global, filter)
			require.NoError(t, err)

			// Expand group config
			err = config.ExpandGroupConfig(&cfg.Groups[0], &cfg.Global, filter)
			require.NoError(t, err)

			// Expand command config
			err = config.ExpandCommandConfig(&cfg.Groups[0].Commands[0], &cfg.Groups[0], &cfg.Global, filter)
			require.NoError(t, err)

			// Verify expanded environment variables
			expandedEnv := cfg.Groups[0].Commands[0].ExpandedEnv
			for key, expectedValue := range tt.expected {
				actualValue, found := expandedEnv[key]
				assert.True(t, found, "Expected environment variable %s not found", key)
				if found {
					assert.Equal(t, expectedValue, actualValue, "Environment variable %s has incorrect value", key)
				}
			}
		})
	}
}

// TestCommandEnvExpansion_ErrorHandling tests error handling in command env expansion
func TestCommandEnvExpansion_ErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		commandEnv  []string
		commandVars []string
		groupVars   []string
		globalVars  []string
		allowlist   []string
		systemEnv   map[string]string
		expectError bool
		errorCheck  func(*testing.T, error)
	}{
		{
			name:        "Undefined variable reference - error",
			commandEnv:  []string{"RESULT=%{undefined_var}"},
			commandVars: []string{},
			groupVars:   []string{},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrUndefinedVariable)
			},
		},
		{
			name:        "Invalid env format - no equals",
			commandEnv:  []string{"INVALID_FORMAT"},
			commandVars: []string{},
			groupVars:   []string{},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrInvalidEnvFormat)
			},
		},
		{
			name:        "Multiple equals - allowed (equals in value)",
			commandEnv:  []string{"VAR=VALUE=EXTRA"},
			commandVars: []string{},
			groupVars:   []string{},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: false, // SplitN allows "=" in value part
		},
		{
			name:        "Empty variable name",
			commandEnv:  []string{"=value"},
			commandVars: []string{},
			groupVars:   []string{},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: true,
			errorCheck: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, config.ErrInvalidEnvFormat)
			},
		},
		{
			name:        "Reserved prefix __runner_ is allowed in env",
			commandEnv:  []string{"__runner_var=value"},
			commandVars: []string{},
			groupVars:   []string{},
			globalVars:  []string{},
			allowlist:   []string{},
			systemEnv:   map[string]string{},
			expectError: false, // Reserved prefix is only checked for vars, not env
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up system environment variables
			for key, value := range tt.systemEnv {
				t.Setenv(key, value)
			}

			// Create configuration
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					FromEnv:      tt.globalVars,
					EnvAllowlist: tt.allowlist,
				},
				Groups: []runnertypes.CommandGroup{
					{
						Name: "test_group",
						Vars: tt.groupVars,
						Commands: []runnertypes.Command{
							{
								Name: "test_command",
								Cmd:  "/bin/echo",
								Vars: tt.commandVars,
								Env:  tt.commandEnv,
							},
						},
					},
				},
			}

			// Create environment filter
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)

			// Expand global config first
			err := config.ExpandGlobalConfig(&cfg.Global, filter)
			if err != nil && tt.expectError {
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
				return
			}
			require.NoError(t, err)

			// Expand group config
			err = config.ExpandGroupConfig(&cfg.Groups[0], &cfg.Global, filter)
			if err != nil && tt.expectError {
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
				return
			}
			require.NoError(t, err)

			// Expand command config
			err = config.ExpandCommandConfig(&cfg.Groups[0].Commands[0], &cfg.Groups[0], &cfg.Global, filter)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorCheck != nil {
					tt.errorCheck(t, err)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
