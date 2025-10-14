// Package config provides tests for the variable expansion functionality.
package config_test

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandCommandStrings_SingleCommand(t *testing.T) {
	tests := []struct {
		name            string
		cmd             runnertypes.Command
		expectedCmd     string
		expectedArgs    []string
		expectError     bool
		globalAllowlist []string
		groupAllowlist  []string
	}{
		{
			name: "basic variable expansion in cmd",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "${BIN_PATH}/ls",
				Args: []string{"-la"},
				Env:  []string{"BIN_PATH=/usr/bin"},
			},
			expectedCmd:     "/usr/bin/ls",
			expectedArgs:    []string{"-la"},
			expectError:     false,
			globalAllowlist: []string{"BIN_PATH"},
		},
		{
			name: "basic variable expansion in args",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "ls",
				Args: []string{"-la", "${HOME}/test"},
				Env:  []string{"HOME=/home/user"},
			},
			expectedCmd:     "ls",
			expectedArgs:    []string{"-la", "/home/user/test"},
			expectError:     false,
			globalAllowlist: []string{"HOME"},
		},
		{
			name: "multiple variable expansion",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "${BIN_PATH}/echo",
				Args: []string{"${MESSAGE}", "${HOME}"},
				Env:  []string{"BIN_PATH=/usr/bin", "MESSAGE=hello", "HOME=/home/user"},
			},
			expectedCmd:     "/usr/bin/echo",
			expectedArgs:    []string{"hello", "/home/user"},
			expectError:     false,
			globalAllowlist: []string{"BIN_PATH", "MESSAGE", "HOME"},
		},
		{
			name: "no variables to expand",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "/usr/bin/ls",
				Args: []string{"-la", "/home"},
				Env:  []string{},
			},
			expectedCmd:     "/usr/bin/ls",
			expectedArgs:    []string{"-la", "/home"},
			expectError:     false,
			globalAllowlist: []string{},
		},
		{
			name: "variable not in allowlist should fail",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "${FORBIDDEN_VAR}/ls",
				Args: []string{"-la"},
				Env:  []string{},
			},
			expectError:     true,
			globalAllowlist: []string{"SAFE_VAR"},
		},
		{
			name: "escape sequence handling",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"\\${HOME}", "${MESSAGE}"},
				Env:  []string{"MESSAGE=hello"},
			},
			expectedCmd:     "echo",
			expectedArgs:    []string{"${HOME}", "hello"},
			expectError:     false,
			globalAllowlist: []string{"MESSAGE"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for system-level access if needed
			if tt.name == "variable not in allowlist should fail" {
				t.Setenv("FORBIDDEN_VAR", "/usr/bin")
			}

			// Create test configuration
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: tt.globalAllowlist,
				},
			}

			// Create filter and expander
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)
			expander := environment.NewVariableExpander(filter)

			// Set up allowlist (use group allowlist if specified, otherwise global)
			allowlist := tt.globalAllowlist
			if len(tt.groupAllowlist) > 0 {
				allowlist = tt.groupAllowlist
			}

			// Create a group with single command to test
			group := &runnertypes.CommandGroup{
				Name:         "test-group",
				EnvAllowlist: allowlist,
				Commands:     []runnertypes.Command{tt.cmd},
			}

			// Store original values for immutability check
			originalCmd := group.Commands[0].Cmd
			originalArgs := make([]string, len(group.Commands[0].Args))
			copy(originalArgs, group.Commands[0].Args)

			// Test the expansion using per-command ExpandCommand (group-level helper
			// was removed in favor of caller-controlled iteration).
			var expandedGroup *runnertypes.CommandGroup
			{
				// create shallow copy and populate commands slice
				tmp := *group
				tmp.Commands = make([]runnertypes.Command, len(group.Commands))

				var err error
				for i := range group.Commands {
					expandedCmd, expandedArgs, expandedEnv, e := config.ExpandCommand(&config.ExpansionContext{
						Command:            &group.Commands[i],
						Expander:           expander,
						AutoEnv:            nil,
						GlobalEnvAllowlist: allowlist,
						GroupName:          group.Name,
						GroupEnvAllowlist:  group.EnvAllowlist,
					})
					if e != nil {
						err = e
						break
					}
					tmp.Commands[i] = group.Commands[i]
					tmp.Commands[i].ExpandedCmd = expandedCmd
					tmp.Commands[i].ExpandedArgs = expandedArgs
					tmp.Commands[i].ExpandedEnv = expandedEnv
				}

				if tt.expectError {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}

				expandedGroup = &tmp
			}

			if !tt.expectError {
				// Check expanded values in new fields
				assert.Equal(t, tt.expectedCmd, expandedGroup.Commands[0].ExpandedCmd)
				assert.Equal(t, tt.expectedArgs, expandedGroup.Commands[0].ExpandedArgs)
				// Verify original fields are unchanged (immutability)
				assert.Equal(t, originalCmd, group.Commands[0].Cmd, "Original command should not be modified")
				assert.Equal(t, originalArgs, group.Commands[0].Args, "Original args should not be modified")
				assert.Equal(t, originalCmd, expandedGroup.Commands[0].Cmd, "Expanded group original command should not be modified")
				assert.Equal(t, originalArgs, expandedGroup.Commands[0].Args, "Expanded group original args should not be modified")
			}
		})
	}
}

// TestExpandCommandStrings_AutoEnv tests that automatic environment variables
// take precedence over command environment variables
func TestExpandCommandStrings_AutoEnv(t *testing.T) {
	cmd := runnertypes.Command{
		Name: "test",
		Cmd:  "echo ${MESSAGE} ${__RUNNER_DATETIME}",
		Args: []string{"${MESSAGE}"},
		Env:  []string{"MESSAGE=from_command"}, // This should be overridden
	}

	autoEnv := map[string]string{
		"MESSAGE":           "from_auto", // Takes precedence
		"__RUNNER_DATETIME": "2025-10-06T12:34:56Z",
	}

	// Create test configuration
	cfg := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"MESSAGE", "__RUNNER_DATETIME"},
		},
	}

	// Create filter and expander
	filter := environment.NewFilter(cfg.Global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)

	// Test expansion with autoEnv
	expandedCmd, expandedArgs, _, err := config.ExpandCommand(&config.ExpansionContext{
		Command:            &cmd,
		Expander:           expander,
		AutoEnv:            autoEnv,
		GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
		GroupName:          "test-group",
		GroupEnvAllowlist:  nil,
	})
	require.NoError(t, err)

	// Auto env should take precedence over command env
	assert.Equal(t, "echo from_auto 2025-10-06T12:34:56Z", expandedCmd)
	assert.Equal(t, []string{"from_auto"}, expandedArgs)
}

func TestExpandCommandStrings(t *testing.T) {
	tests := []struct {
		name        string
		group       runnertypes.CommandGroup
		expectError bool
	}{
		{
			name: "successful expansion for all commands in group",
			group: runnertypes.CommandGroup{
				Name:         "test-group",
				EnvAllowlist: []string{"BIN_PATH", "HOME"},
				Commands: []runnertypes.Command{
					{
						Name: "cmd1",
						Cmd:  "${BIN_PATH}/ls",
						Args: []string{"-la", "${HOME}"},
						Env:  []string{"BIN_PATH=/usr/bin", "HOME=/home/user"},
					},
					{
						Name: "cmd2",
						Cmd:  "echo",
						Args: []string{"${HOME}"},
						Env:  []string{"HOME=/home/test"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "error in one command should fail entire group",
			group: runnertypes.CommandGroup{
				Name:         "test-group",
				EnvAllowlist: []string{"SAFE_VAR"},
				Commands: []runnertypes.Command{
					{
						Name: "cmd1",
						Cmd:  "echo",
						Args: []string{"${SAFE_VAR}"},
						Env:  []string{"SAFE_VAR=safe_value"},
					},
					{
						Name: "cmd2",
						Cmd:  "${UNSAFE_VAR}/echo",
						Args: []string{"test"},
						Env:  []string{},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for system-level access if needed
			if tt.name == "error in one command should fail entire group" {
				t.Setenv("UNSAFE_VAR", "/usr/bin")
			}

			// Create test configuration
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{},
				},
			}

			// Create filter and expander
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)
			expander := environment.NewVariableExpander(filter)

			// Store original for immutability check
			originalGroupName := tt.group.Name
			originalCmd1 := tt.group.Commands[0].Cmd

			// Test group expansion by iterating commands
			{
				tmp := tt.group
				tmp.Commands = make([]runnertypes.Command, len(tt.group.Commands))

				var err error
				for i := range tt.group.Commands {
					expandedCmd, expandedArgs, expandedEnv, e := config.ExpandCommand(&config.ExpansionContext{
						Command:            &tt.group.Commands[i],
						Expander:           expander,
						AutoEnv:            nil,
						GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
						GroupName:          tt.group.Name,
						GroupEnvAllowlist:  tt.group.EnvAllowlist,
					})
					if e != nil {
						err = e
						break
					}
					tmp.Commands[i] = tt.group.Commands[i]
					tmp.Commands[i].ExpandedCmd = expandedCmd
					tmp.Commands[i].ExpandedArgs = expandedArgs
					tmp.Commands[i].ExpandedEnv = expandedEnv
				}

				if tt.expectError {
					assert.Error(t, err)
					return
				}
				require.NoError(t, err)

				// Verify expansion for first command
				require.Len(t, tmp.Commands, 2, "Should have two commands")

				cmd1 := tmp.Commands[0]
				assert.Equal(t, "/usr/bin/ls", cmd1.ExpandedCmd, "First command should be expanded")
				assert.Equal(t, []string{"-la", "/home/user"}, cmd1.ExpandedArgs, "First command args should be expanded")

				cmd2 := tmp.Commands[1]
				assert.Equal(t, "echo", cmd2.ExpandedCmd, "Second command should remain unchanged")
				assert.Equal(t, []string{"/home/test"}, cmd2.ExpandedArgs, "Second command args should be expanded")

				// Verify original is unchanged (immutability)
				assert.Equal(t, originalGroupName, tt.group.Name, "Original group name should not be modified")
				assert.Equal(t, originalCmd1, tt.group.Commands[0].Cmd, "Original command should not be modified")
			}
		})
	}
}

func TestCircularReferenceDetection(t *testing.T) {
	tests := []struct {
		name        string
		cmd         runnertypes.Command
		expectError bool
		errorMsg    string
	}{
		{
			name: "simple circular reference should be detected",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${VAR1}"},
				Env:  []string{"VAR1=${VAR2}", "VAR2=${VAR1}"},
			},
			expectError: true,
			errorMsg:    "circular variable reference",
		},
		{
			name: "three-way circular reference should be detected",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${VAR1}"},
				Env:  []string{"VAR1=${VAR2}", "VAR2=${VAR3}", "VAR3=${VAR1}"},
			},
			expectError: true,
			errorMsg:    "circular variable reference",
		},
		{
			name: "non-circular nested reference should succeed",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${VAR1}"},
				Env:  []string{"VAR1=${VAR2}/subdir", "VAR2=/base/path"},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test configuration with all variables allowed
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: []string{"VAR1", "VAR2", "VAR3"},
				},
			}

			// Create filter and expander
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)
			expander := environment.NewVariableExpander(filter)

			// Create a group with single command to test
			group := &runnertypes.CommandGroup{
				Name:         "test-group",
				EnvAllowlist: []string{"VAR1", "VAR2", "VAR3"},
				Commands:     []runnertypes.Command{tt.cmd},
			}

			// Test circular reference detection by executing expansion per-command
			{
				var err error
				for i := range group.Commands {
					_, _, _, e := config.ExpandCommand(&config.ExpansionContext{
						Command:            &group.Commands[i],
						Expander:           expander,
						AutoEnv:            nil,
						GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
						GroupName:          group.Name,
						GroupEnvAllowlist:  group.EnvAllowlist,
					})
					if e != nil {
						err = e
						break
					}
				}

				if tt.expectError {
					require.Error(t, err)
					if tt.errorMsg != "" {
						assert.Contains(t, err.Error(), tt.errorMsg)
					}
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

func TestSecurityIntegration(t *testing.T) {
	tests := []struct {
		name            string
		cmd             runnertypes.Command
		globalAllowlist []string
		groupAllowlist  []string
		expectError     bool
		errorMsg        string
	}{
		{
			name: "Command.Env variables should be implicitly allowed",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${PRIVATE_VAR}"},
				Env:  []string{"PRIVATE_VAR=private_value"},
			},
			globalAllowlist: []string{}, // Empty allowlist - should still work due to Command.Env
			expectError:     false,
		},
		{
			name: "Global allowlist should be respected",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${GLOBAL_VAR}"},
				Env:  []string{},
			},
			globalAllowlist: []string{"GLOBAL_VAR"},
			expectError:     false,
		},
		{
			name: "Group allowlist should override global",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${GROUP_VAR}"},
				Env:  []string{},
			},
			globalAllowlist: []string{"GLOBAL_VAR"},
			groupAllowlist:  []string{"GROUP_VAR"},
			expectError:     false,
		},
		{
			name: "Unauthorized variable should fail",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${FORBIDDEN_VAR}"},
				Env:  []string{},
			},
			globalAllowlist: []string{"SAFE_VAR"},
			expectError:     true,
			errorMsg:        "not allowed",
		},
		{
			name: "PATH extension with system variable reference should work",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${PATH}"},
				Env:  []string{"PATH=/custom/bin:${PATH}"},
			},
			globalAllowlist: []string{"PATH"},
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variables for system-level access
			if tt.name == "Global allowlist should be respected" {
				t.Setenv("GLOBAL_VAR", "global_value")
			}
			if tt.name == "Group allowlist should override global" {
				t.Setenv("GROUP_VAR", "group_value")
			}
			if tt.name == "Unauthorized variable should fail" {
				t.Setenv("FORBIDDEN_VAR", "forbidden_value")
			}

			// Create test configuration
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: tt.globalAllowlist,
				},
			}

			// Create filter and expander
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)
			expander := environment.NewVariableExpander(filter)

			// Use group allowlist if specified, otherwise global
			allowlist := tt.globalAllowlist
			if len(tt.groupAllowlist) > 0 {
				allowlist = tt.groupAllowlist
			}

			// Create a group with single command to test
			group := &runnertypes.CommandGroup{
				Name:         "test-group",
				EnvAllowlist: allowlist,
				Commands:     []runnertypes.Command{tt.cmd},
			}

			// Test security integration by expanding per-command
			{
				var err error
				for i := range group.Commands {
					_, _, _, e := config.ExpandCommand(&config.ExpansionContext{
						Command:            &group.Commands[i],
						Expander:           expander,
						AutoEnv:            nil,
						GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
						GroupName:          group.Name,
						GroupEnvAllowlist:  group.EnvAllowlist,
					})
					if e != nil {
						err = e
						break
					}
				}

				if tt.expectError {
					require.Error(t, err)
					if tt.errorMsg != "" {
						assert.Contains(t, err.Error(), tt.errorMsg)
					}
				} else {
					require.NoError(t, err)
				}
			}
		})
	}
}

// TestSecurityAttackPrevention tests various security attack scenarios
// to ensure the variable expansion feature is resistant to common attacks.
func TestSecurityAttackPrevention(t *testing.T) {
	tests := []struct {
		name            string
		cmd             runnertypes.Command
		globalAllowlist []string
		groupAllowlist  []string
		expectError     bool
		errorContains   string
		description     string
	}{
		{
			name: "Command injection via variable expansion",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${MALICIOUS}"},
				Env:  []string{"MALICIOUS=foo; rm -rf /"},
			},
			globalAllowlist: []string{},
			expectError:     true,
			errorContains:   "unsafe",
			description:     "Variables containing shell metacharacters should be rejected",
		},
		{
			name: "Path traversal attack via variable",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "${TRAVERSAL_PATH}/cat",
				Args: []string{"/etc/passwd"},
				Env:  []string{"TRAVERSAL_PATH=../../bin"},
			},
			globalAllowlist: []string{},
			expectError:     false,
			description:     "Path traversal is validated at command execution, not during expansion",
		},
		{
			name: "Multiple variable with dangerous content",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "${BIN}",
				Args: []string{"${ARG1}", "${ARG2}"},
				Env:  []string{"BIN=/bin/sh", "ARG1=-c", "ARG2=rm -rf /"},
			},
			globalAllowlist: []string{},
			expectError:     true,
			errorContains:   "unsafe",
			description:     "Variables with dangerous content should be rejected during validation",
		},
		{
			name: "Environment variable with suspicious patterns",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${SUSPICIOUS}"},
				Env:  []string{"SUSPICIOUS=`whoami`"},
			},
			globalAllowlist: []string{},
			expectError:     true,
			errorContains:   "unsafe",
			description:     "Command substitution patterns should be rejected",
		},
		{
			name: "Null byte in variable value",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${NULL_BYTE}"},
				Env:  []string{"NULL_BYTE=safe\x00malicious"},
			},
			globalAllowlist: []string{},
			expectError:     false,
			description:     "Null bytes are handled by Go strings, not explicitly rejected",
		},
		{
			name: "Allowlist bypass attempt using similar variable names",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${SAFE_VAR1}"},
				Env:  []string{},
			},
			globalAllowlist: []string{"SAFE_VAR"},
			expectError:     true,
			errorContains:   "not found",
			description:     "Non-matching variable names are rejected as not found",
		},
		{
			name: "Safe variable with special characters in value",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${SAFE}"},
				Env:  []string{"SAFE=/path/to/file-v1.0.txt"},
			},
			globalAllowlist: []string{},
			expectError:     false,
			description:     "Legitimate special characters in paths should be allowed",
		},
		{
			name: "Double expansion attempt",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${DOUBLE}"},
				Env:  []string{"DOUBLE=${INNER}", "INNER=safe_value"},
			},
			globalAllowlist: []string{},
			expectError:     false,
			description:     "Nested variable expansion should work correctly",
		},
		{
			name: "Group allowlist overrides global allowlist",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${GROUP_VAR}"},
				Env:  []string{},
			},
			globalAllowlist: []string{"GLOBAL_VAR"},
			groupAllowlist:  []string{"GROUP_VAR"},
			expectError:     false,
			description:     "Group allowlist should override global allowlist",
		},
		{
			name: "Empty group allowlist rejects all system variables",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${SYSTEM_VAR}"},
				Env:  []string{},
			},
			globalAllowlist: []string{"SYSTEM_VAR"},
			groupAllowlist:  []string{},
			expectError:     true,
			errorContains:   "not allowed",
			description:     "Empty group allowlist should reject all system variables",
		},
		{
			name: "Nested expansion resulting in dangerous value",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${OUTER}"},
				Env:  []string{"OUTER=${INNER}", "INNER=value; rm -rf /"},
			},
			globalAllowlist: []string{},
			expectError:     true,
			errorContains:   "unsafe",
			description:     "Nested expansion should detect dangerous values after expansion",
		},
		{
			name: "Command.Env variables work with group allowlist",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "echo",
				Args: []string{"${CMD_VAR}"},
				Env:  []string{"CMD_VAR=safe_value"},
			},
			globalAllowlist: []string{},
			groupAllowlist:  []string{},
			expectError:     false,
			description:     "Command.Env variables should be implicitly allowed regardless of group allowlist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Explicitly set environment variables for tests that need them
			if tt.name == "Group allowlist overrides global allowlist" {
				t.Setenv("GROUP_VAR", "group_value")
			}
			if tt.name == "Empty group allowlist rejects all system variables" {
				t.Setenv("SYSTEM_VAR", "system_value")
			}

			// Create test configuration
			cfg := &runnertypes.Config{
				Global: runnertypes.GlobalConfig{
					EnvAllowlist: tt.globalAllowlist,
				},
			}

			// Create filter and expander
			filter := environment.NewFilter(cfg.Global.EnvAllowlist)
			expander := environment.NewVariableExpander(filter)

			// Determine which allowlist to use
			// IMPORTANT: Use groupAllowlist if it's explicitly provided (even if empty)
			// Only fall back to globalAllowlist if groupAllowlist is nil
			var allowlist []string
			if tt.groupAllowlist != nil {
				allowlist = tt.groupAllowlist
			} else {
				allowlist = tt.globalAllowlist
			}

			// Test expansion
			_, _, _, err := config.ExpandCommand(&config.ExpansionContext{
				Command:            &tt.cmd,
				Expander:           expander,
				AutoEnv:            nil,
				GlobalEnvAllowlist: allowlist,
				GroupName:          "test-group",
				GroupEnvAllowlist:  nil,
			})

			if tt.expectError {
				require.Error(t, err, "Expected error for: %s", tt.description)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains,
						"Error should contain '%s' for: %s", tt.errorContains, tt.description)
				}
			} else {
				require.NoError(t, err, "Should not error for: %s", tt.description)
			}
		})
	}
}

// BenchmarkVariableExpansion benchmarks the variable expansion performance
// for different scenarios to ensure performance requirements are met.
func BenchmarkVariableExpansion(b *testing.B) {
	benchmarks := []struct {
		name string
		cmd  runnertypes.Command
	}{
		{
			name: "simple_expansion",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "${BIN_PATH}/ls",
				Args: []string{"-la"},
				Env:  []string{"BIN_PATH=/usr/bin"},
			},
		},
		{
			name: "complex_args",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "${BIN_PATH}/echo",
				Args: []string{"${VAR1}", "${VAR2}", "${VAR3}", "${VAR4}", "${VAR5}"},
				Env: []string{
					"BIN_PATH=/usr/bin",
					"VAR1=value1",
					"VAR2=value2",
					"VAR3=value3",
					"VAR4=value4",
					"VAR5=value5",
				},
			},
		},
		{
			name: "braced_format_recommended",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "${HOME}/bin/script",
				Args: []string{"${CONFIG_DIR}/config.toml", "${DATA_DIR}/data.txt"},
				Env: []string{
					"HOME=/home/user",
					"CONFIG_DIR=/etc/myapp",
					"DATA_DIR=/var/lib/myapp",
				},
			},
		},
		{
			name: "glob_pattern_literal",
			cmd: runnertypes.Command{
				Name: "test",
				Cmd:  "find",
				Args: []string{"${SEARCH_DIR}", "-name", "*.txt"},
				Env:  []string{"SEARCH_DIR=/home/user/documents"},
			},
		},
	}

	// Extract all variable names from benchmark data for allowlist
	allowlistMap := make(map[string]bool)
	for _, bm := range benchmarks {
		for _, envVar := range bm.cmd.Env {
			// Extract variable name from "NAME=value" format
			if idx := len(envVar); idx > 0 {
				for i := range idx {
					if envVar[i] == '=' {
						allowlistMap[envVar[:i]] = true
						break
					}
				}
			}
		}
	}

	// Convert map to slice for allowlist
	allowlist := make([]string, 0, len(allowlistMap))
	for varName := range allowlistMap {
		allowlist = append(allowlist, varName)
	}

	// Create test configuration once
	cfg := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: allowlist,
		},
	}

	// Create filter and expander once
	filter := environment.NewFilter(cfg.Global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ResetTimer()
			for range b.N {
				_, _, _, err := config.ExpandCommand(&config.ExpansionContext{
					Command:            &bm.cmd,
					Expander:           expander,
					AutoEnv:            nil,
					GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
					GroupName:          "benchmark-group",
					GroupEnvAllowlist:  nil,
				})
				if err != nil {
					b.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// TestExpandCommand_AutoEnvInCommandEnv verifies that automatic environment variables
// can be referenced within a command's env block.
func TestExpandCommand_AutoEnvInCommandEnv(t *testing.T) {
	cfg := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"__RUNNER_DATETIME", "__RUNNER_PID", "OUTPUT_FILE"},
		},
	}
	filter := environment.NewFilter(cfg.Global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)

	// Create automatic environment variables
	autoEnv := map[string]string{
		"__RUNNER_DATETIME": "2024-01-15T10:30:00Z",
		"__RUNNER_PID":      "12345",
	}

	tests := []struct {
		name           string
		cmd            runnertypes.Command
		expectedEnv    map[string]string
		expectError    bool
		groupAllowlist []string
	}{
		{
			name: "reference automatic variable in command env",
			cmd: runnertypes.Command{
				Name: "test_auto_env",
				Cmd:  "echo",
				Args: []string{"test"},
				Env:  []string{"OUTPUT_FILE=/tmp/output_${__RUNNER_DATETIME}.txt"},
			},
			expectedEnv: map[string]string{
				"__RUNNER_DATETIME": "2024-01-15T10:30:00Z",
				"__RUNNER_PID":      "12345",
				"OUTPUT_FILE":       "/tmp/output_2024-01-15T10:30:00Z.txt",
			},
			expectError:    false,
			groupAllowlist: []string{"__RUNNER_DATETIME", "__RUNNER_PID"},
		},
		{
			name: "reference multiple automatic variables in command env",
			cmd: runnertypes.Command{
				Name: "test_multi_auto_env",
				Cmd:  "echo",
				Args: []string{"test"},
				Env: []string{
					"OUTPUT_FILE=/tmp/output_${__RUNNER_DATETIME}_${__RUNNER_PID}.txt",
					"LOG_FILE=/var/log/runner_${__RUNNER_PID}.log",
				},
			},
			expectedEnv: map[string]string{
				"__RUNNER_DATETIME": "2024-01-15T10:30:00Z",
				"__RUNNER_PID":      "12345",
				"OUTPUT_FILE":       "/tmp/output_2024-01-15T10:30:00Z_12345.txt",
				"LOG_FILE":          "/var/log/runner_12345.log",
			},
			expectError:    false,
			groupAllowlist: []string{"__RUNNER_DATETIME", "__RUNNER_PID"},
		},
		{
			name: "automatic variables take precedence over command env",
			cmd: runnertypes.Command{
				Name: "test_precedence",
				Cmd:  "echo",
				Args: []string{"test"},
				Env: []string{
					"__RUNNER_DATETIME=should_be_overridden",
					"OUTPUT_FILE=/tmp/output_${__RUNNER_DATETIME}.txt",
				},
			},
			expectedEnv: map[string]string{
				"__RUNNER_DATETIME": "2024-01-15T10:30:00Z", // autoEnv takes precedence
				"__RUNNER_PID":      "12345",
				"OUTPUT_FILE":       "/tmp/output_2024-01-15T10:30:00Z.txt",
			},
			expectError:    false,
			groupAllowlist: []string{"__RUNNER_DATETIME", "__RUNNER_PID"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, env, err := config.ExpandCommand(&config.ExpansionContext{
				Command:            &tt.cmd,
				Expander:           expander,
				AutoEnv:            autoEnv,
				GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
				GroupName:          "test_group",
				GroupEnvAllowlist:  tt.groupAllowlist,
			})

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedEnv, env)
		})
	}
}

// TestExpandGlobalVerifyFiles tests the expansion of environment variables in global verify_files
func TestExpandGlobalVerifyFiles(t *testing.T) {
	tests := []struct {
		name          string
		verifyFiles   []string
		envAllowlist  []string
		expectedFiles []string
		expectError   bool
		errorSentinel error
		errorContains string
		setupEnv      func(*testing.T)
	}{
		{
			name:          "basic variable expansion",
			verifyFiles:   []string{"${HOME}/config.toml", "${HOME}/data.txt"},
			envAllowlist:  []string{"HOME"},
			expectedFiles: []string{"/home/user/config.toml", "/home/user/data.txt"},
			setupEnv: func(t *testing.T) {
				t.Setenv("HOME", "/home/user")
			},
		},
		{
			name:          "multiple variables in single path",
			verifyFiles:   []string{"${BASE_DIR}/${APP_NAME}/config.toml"},
			envAllowlist:  []string{"BASE_DIR", "APP_NAME"},
			expectedFiles: []string{"/opt/myapp/config.toml"},
			setupEnv: func(t *testing.T) {
				t.Setenv("BASE_DIR", "/opt")
				t.Setenv("APP_NAME", "myapp")
			},
		},
		{
			name:          "allowlist violation error",
			verifyFiles:   []string{"${FORBIDDEN_VAR}/config.toml"},
			envAllowlist:  []string{"SAFE_VAR"},
			expectError:   true,
			errorSentinel: config.ErrGlobalVerifyFilesExpansionFailed,
			errorContains: "not allowed",
			setupEnv: func(t *testing.T) {
				t.Setenv("FORBIDDEN_VAR", "/forbidden")
			},
		},
		{
			name:          "undefined variable error",
			verifyFiles:   []string{"${UNDEFINED_VAR}/config.toml"},
			envAllowlist:  []string{"UNDEFINED_VAR"},
			expectError:   true,
			errorSentinel: config.ErrGlobalVerifyFilesExpansionFailed,
			errorContains: "not found",
		},
		// NOTE: Circular reference in system environment variables is extremely rare
		// because shell expands variables before setting them. This test case is
		// commented out as it's not a realistic scenario for verify_files expansion.
		// Circular reference detection is still tested in Command.Env expansion tests.
		// {
		// 	name:          "circular reference error",
		// 	verifyFiles:   []string{"${VAR1}/config.toml"},
		// 	envAllowlist:  []string{"VAR1", "VAR2"},
		// 	expectError:   true,
		// 	errorSentinel: config.ErrGlobalVerifyFilesExpansionFailed,
		// 	errorContains: "circular",
		// 	setupEnv: func(t *testing.T) {
		// 		t.Setenv("VAR1", "${VAR2}")
		// 		t.Setenv("VAR2", "${VAR1}")
		// 	},
		// },
		{
			name:          "nil config error",
			verifyFiles:   nil,
			expectError:   true,
			errorSentinel: config.ErrNilConfig,
			errorContains: "cannot be nil",
		},
		{
			name:          "empty array processing",
			verifyFiles:   []string{},
			envAllowlist:  []string{},
			expectedFiles: []string{},
		},
		{
			name:          "escape sequence handling",
			verifyFiles:   []string{"\\${HOME}/config.toml", "${DATA}/file.txt"},
			envAllowlist:  []string{"DATA"},
			expectedFiles: []string{"${HOME}/config.toml", "/var/data/file.txt"},
			setupEnv: func(t *testing.T) {
				t.Setenv("DATA", "/var/data")
			},
		},
		{
			name:          "complex variable nesting",
			verifyFiles:   []string{"${BASE}/${SUB1}/${SUB2}/config.toml"},
			envAllowlist:  []string{"BASE", "SUB1", "SUB2"},
			expectedFiles: []string{"/opt/app/subdir/config.toml"},
			setupEnv: func(t *testing.T) {
				t.Setenv("BASE", "/opt")
				t.Setenv("SUB1", "app")
				t.Setenv("SUB2", "subdir")
			},
		},
		{
			name:          "error chain verification",
			verifyFiles:   []string{"${ALLOWED_VAR}/file1.txt", "${FORBIDDEN_VAR}/file2.txt"},
			envAllowlist:  []string{"ALLOWED_VAR"},
			expectError:   true,
			errorSentinel: config.ErrGlobalVerifyFilesExpansionFailed,
			errorContains: "verify_files[1]",
			setupEnv: func(t *testing.T) {
				t.Setenv("ALLOWED_VAR", "/allowed")
				t.Setenv("FORBIDDEN_VAR", "/forbidden")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			if tt.setupEnv != nil {
				tt.setupEnv(t)
			}

			// Create global config
			var global *runnertypes.GlobalConfig
			if tt.name == "nil config error" {
				global = nil
			} else {
				global = &runnertypes.GlobalConfig{
					VerifyFiles:  tt.verifyFiles,
					EnvAllowlist: tt.envAllowlist,
				}
			}

			// Create filter and expander
			filter := environment.NewFilter(tt.envAllowlist)
			expander := environment.NewVariableExpander(filter)

			// Execute expansion
			err := config.ExpandGlobalVerifyFiles(global, filter, expander)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.errorSentinel != nil {
					assert.ErrorIs(t, err, tt.errorSentinel)
				}
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedFiles, global.ExpandedVerifyFiles)
			}
		})
	}
}

// TestExpandGroupVerifyFiles tests the expansion of environment variables in group verify_files
func TestExpandGroupVerifyFiles(t *testing.T) {
	tests := []struct {
		name               string
		verifyFiles        []string
		groupEnvAllowlist  []string
		globalEnvAllowlist []string
		expectedFiles      []string
		expectError        bool
		errorSentinel      error
		errorContains      string
		setupEnv           func(*testing.T)
		groupName          string
	}{
		{
			name:              "system environment variable expansion",
			verifyFiles:       []string{"${HOME}/group/config.toml"},
			groupEnvAllowlist: []string{"HOME"},
			expectedFiles:     []string{"/home/user/group/config.toml"},
			groupName:         "test-group",
			setupEnv: func(t *testing.T) {
				t.Setenv("HOME", "/home/user")
			},
		},
		{
			name:               "allowlist inheritance - inherit mode",
			verifyFiles:        []string{"${GLOBAL_VAR}/config.toml"},
			groupEnvAllowlist:  nil, // nil means inherit from global
			globalEnvAllowlist: []string{"GLOBAL_VAR"},
			expectedFiles:      []string{"/global/config.toml"},
			groupName:          "test-group",
			setupEnv: func(t *testing.T) {
				t.Setenv("GLOBAL_VAR", "/global")
			},
		},
		{
			name:              "allowlist inheritance - explicit mode",
			verifyFiles:       []string{"${GROUP_VAR}/config.toml"},
			groupEnvAllowlist: []string{"GROUP_VAR"},
			expectedFiles:     []string{"/group/config.toml"},
			groupName:         "test-group",
			setupEnv: func(t *testing.T) {
				t.Setenv("GROUP_VAR", "/group")
			},
		},
		{
			name:               "allowlist inheritance - reject mode",
			verifyFiles:        []string{"${GLOBAL_VAR}/config.toml"},
			groupEnvAllowlist:  []string{}, // empty slice means reject all
			globalEnvAllowlist: []string{"GLOBAL_VAR"},
			expectError:        true,
			errorSentinel:      config.ErrGroupVerifyFilesExpansionFailed,
			errorContains:      "not allowed", // empty allowlist blocks all variables
			groupName:          "test-group",
			setupEnv: func(t *testing.T) {
				t.Setenv("GLOBAL_VAR", "/global")
			},
		},
		{
			name:              "group name in error",
			verifyFiles:       []string{"${FORBIDDEN}/config.toml"},
			groupEnvAllowlist: []string{"SAFE"},
			expectError:       true,
			errorSentinel:     config.ErrGroupVerifyFilesExpansionFailed,
			errorContains:     "my-group",
			groupName:         "my-group",
			setupEnv: func(t *testing.T) {
				t.Setenv("FORBIDDEN", "/forbidden")
			},
		},
		{
			name:          "nil config error",
			verifyFiles:   nil,
			expectError:   true,
			errorSentinel: config.ErrNilConfig,
			errorContains: "cannot be nil",
			groupName:     "test-group",
		},
		{
			name:               "inheritance mode determination",
			verifyFiles:        []string{"${INHERITED}/config.toml"},
			groupEnvAllowlist:  nil, // nil means inherit from global
			globalEnvAllowlist: []string{"INHERITED"},
			expectedFiles:      []string{"/inherited/config.toml"},
			groupName:          "test-group",
			setupEnv: func(t *testing.T) {
				t.Setenv("INHERITED", "/inherited")
			},
		},
		{
			name:              "environment variable priority",
			verifyFiles:       []string{"${VAR1}/file.txt", "${VAR2}/file.txt"},
			groupEnvAllowlist: []string{"VAR1", "VAR2"},
			expectedFiles:     []string{"/path1/file.txt", "/path2/file.txt"},
			groupName:         "test-group",
			setupEnv: func(t *testing.T) {
				t.Setenv("VAR1", "/path1")
				t.Setenv("VAR2", "/path2")
			},
		},
		// NOTE: Circular reference in system environment variables is extremely rare
		// because shell expands variables before setting them. This test case is
		// commented out as it's not a realistic scenario for verify_files expansion.
		// Circular reference detection is still tested in Command.Env expansion tests.
		// {
		// 	name:              "circular reference error in group",
		// 	verifyFiles:       []string{"${VAR1}/config.toml"},
		// 	groupEnvAllowlist: []string{"VAR1", "VAR2"},
		// 	expectError:       true,
		// 	errorSentinel:     config.ErrGroupVerifyFilesExpansionFailed,
		// 	errorContains:     "circular",
		// 	groupName:         "test-group",
		// 	setupEnv: func(t *testing.T) {
		// 		t.Setenv("VAR1", "${VAR2}")
		// 		t.Setenv("VAR2", "${VAR1}")
		// 	},
		// },
		{
			name:              "error context verification",
			verifyFiles:       []string{"${ALLOWED}/file1.txt", "${FORBIDDEN}/file2.txt"},
			groupEnvAllowlist: []string{"ALLOWED"},
			expectError:       true,
			errorSentinel:     config.ErrGroupVerifyFilesExpansionFailed,
			errorContains:     "verify_files[1]",
			groupName:         "test-group",
			setupEnv: func(t *testing.T) {
				t.Setenv("ALLOWED", "/allowed")
				t.Setenv("FORBIDDEN", "/forbidden")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			if tt.setupEnv != nil {
				tt.setupEnv(t)
			}

			// Create group config
			var group *runnertypes.CommandGroup
			if tt.name == "nil config error" {
				group = nil
			} else {
				group = &runnertypes.CommandGroup{
					Name:         tt.groupName,
					VerifyFiles:  tt.verifyFiles,
					EnvAllowlist: tt.groupEnvAllowlist,
				}
			}

			// Create filter with global allowlist
			filter := environment.NewFilter(tt.globalEnvAllowlist)
			expander := environment.NewVariableExpander(filter)

			// Create empty global config for backward compatibility
			global := &runnertypes.GlobalConfig{}

			// Execute expansion
			err := config.ExpandGroupVerifyFiles(group, global, filter, expander)

			// Verify results
			if tt.expectError {
				require.Error(t, err)
				if tt.errorSentinel != nil {
					assert.ErrorIs(t, err, tt.errorSentinel)
				}
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedFiles, group.ExpandedVerifyFiles)
			}
		})
	}
}

// BenchmarkExpandGlobalVerifyFiles benchmarks the performance of global verify_files expansion
func BenchmarkExpandGlobalVerifyFiles(b *testing.B) {
	// Setup environment
	b.Setenv("HOME", "/home/testuser")
	b.Setenv("BASE", "/opt")
	b.Setenv("APP", "myapp")

	global := &runnertypes.GlobalConfig{
		VerifyFiles: []string{
			"${HOME}/config.toml",
			"${HOME}/data.txt",
			"${BASE}/${APP}/file.txt",
		},
		EnvAllowlist: []string{"HOME", "BASE", "APP"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := config.ExpandGlobalVerifyFiles(global, filter, expander)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

// BenchmarkExpandGroupVerifyFiles benchmarks the performance of group verify_files expansion
func BenchmarkExpandGroupVerifyFiles(b *testing.B) {
	// Setup environment
	b.Setenv("HOME", "/home/testuser")
	b.Setenv("DATA", "/var/data")

	group := &runnertypes.CommandGroup{
		Name: "test-group",
		VerifyFiles: []string{
			"${HOME}/group/config.toml",
			"${DATA}/file.txt",
		},
		EnvAllowlist: []string{"HOME", "DATA"},
	}

	globalAllowlist := []string{"HOME", "DATA"}
	filter := environment.NewFilter(globalAllowlist)
	expander := environment.NewVariableExpander(filter)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Create empty global config for backward compatibility
		global := &runnertypes.GlobalConfig{}
		err := config.ExpandGroupVerifyFiles(group, global, filter, expander)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

// BenchmarkExpandLargeVerifyFiles benchmarks performance with many verify_files
func BenchmarkExpandLargeVerifyFiles(b *testing.B) {
	// Setup environment
	b.Setenv("BASE", "/opt/app")

	// Create config with 100 verify_files
	verifyFiles := make([]string, 100)
	for i := 0; i < 100; i++ {
		verifyFiles[i] = "${BASE}/file" + string(rune('0'+i%10)) + ".txt"
	}

	global := &runnertypes.GlobalConfig{
		VerifyFiles:  verifyFiles,
		EnvAllowlist: []string{"BASE"},
	}

	filter := environment.NewFilter(global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := config.ExpandGlobalVerifyFiles(global, filter, expander)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

// TestExpandCommand_CommandEnvExpansionError tests that command environment variable
// expansion failures return ErrCommandEnvExpansionFailed
func TestExpandCommand_CommandEnvExpansionError(t *testing.T) {
	// Create test configuration
	cfg := &runnertypes.Config{
		Global: runnertypes.GlobalConfig{
			EnvAllowlist: []string{"SAFE_VAR"},
		},
	}

	// Create filter and expander
	filter := environment.NewFilter(cfg.Global.EnvAllowlist)
	expander := environment.NewVariableExpander(filter)

	// Create a command with an invalid environment variable that should cause expansion to fail
	cmd := runnertypes.Command{
		Name: "test_command",
		Cmd:  "echo",
		Args: []string{"hello"},
		Env:  []string{"INVALID_VAR=${FORBIDDEN_VAR}"}, // FORBIDDEN_VAR is not in allowlist
	}

	// Set up the forbidden environment variable in system env
	t.Setenv("FORBIDDEN_VAR", "/forbidden/path")

	// Attempt expansion - this should fail
	_, _, _, err := config.ExpandCommand(&config.ExpansionContext{
		Command:            &cmd,
		Expander:           expander,
		AutoEnv:            nil,
		GlobalEnvAllowlist: cfg.Global.EnvAllowlist,
		GroupName:          "test-group",
		GroupEnvAllowlist:  nil,
	}) // Verify that we get the expected error type
	require.Error(t, err)
	require.ErrorIs(t, err, config.ErrCommandEnvExpansionFailed)
	assert.Contains(t, err.Error(), "command environment variable expansion failed")
}

// TestExpandGlobalEnv_Basic tests basic environment variable expansion
func TestExpandGlobalEnv_Basic(t *testing.T) {
	// Prepare test environment
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expected    map[string]string
		expectError bool
	}{
		{
			name:        "simple variable expansion",
			globalEnv:   []string{"VAR1=value1", "VAR2=value2"},
			allowlist:   []string{"HOME"},
			expected:    map[string]string{"VAR1": "value1", "VAR2": "value2"},
			expectError: false,
		},
		{
			name:        "empty env list",
			globalEnv:   []string{},
			allowlist:   []string{"HOME"},
			expected:    map[string]string{},
			expectError: false,
		},
		{
			name:        "nil env list",
			globalEnv:   nil,
			allowlist:   []string{"HOME"},
			expected:    map[string]string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.expected == nil {
				assert.Nil(t, cfg.ExpandedEnv)
			} else {
				require.NotNil(t, cfg.ExpandedEnv)
				assert.Equal(t, tt.expected, cfg.ExpandedEnv)
			}
		})
	}
}

// TestExpandGlobalEnv_VariableReference tests variable references within Global.Env
func TestExpandGlobalEnv_VariableReference(t *testing.T) {
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expected    map[string]string
		expectError bool
	}{
		{
			name:        "reference within global env",
			globalEnv:   []string{"BASE_DIR=/opt/app", "CONFIG_DIR=${BASE_DIR}/config"},
			allowlist:   []string{"HOME"},
			expected:    map[string]string{"BASE_DIR": "/opt/app", "CONFIG_DIR": "/opt/app/config"},
			expectError: false,
		},
		{
			name:        "multiple variable references",
			globalEnv:   []string{"A=1", "B=${A}2", "C=${A}${B}3"},
			allowlist:   []string{"HOME"},
			expected:    map[string]string{"A": "1", "B": "12", "C": "1123"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.ExpandedEnv)
		})
	}
}

// TestExpandGlobalEnv_SystemEnvReference tests system environment variable references
func TestExpandGlobalEnv_SystemEnvReference(t *testing.T) {
	// Set up test environment variables
	testHome := "/test/home"
	t.Setenv("TEST_HOME", testHome)

	filter := environment.NewFilter([]string{"TEST_HOME"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expected    map[string]string
		expectError bool
	}{
		{
			name:        "reference system env with allowlist",
			globalEnv:   []string{"APP_HOME=${TEST_HOME}/app"},
			allowlist:   []string{"TEST_HOME"},
			expected:    map[string]string{"APP_HOME": "/test/home/app"},
			expectError: false,
		},
		{
			name:        "reference system env without allowlist",
			globalEnv:   []string{"APP_HOME=${TEST_HOME}/app"},
			allowlist:   []string{"OTHER_VAR"},
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.ExpandedEnv)
		})
	}
}

// TestExpandGlobalEnv_SelfReference tests self-reference (e.g., PATH=/custom:${PATH})
func TestExpandGlobalEnv_SelfReference(t *testing.T) {
	// Set up test environment variable
	originalPath := "/usr/bin:/bin"
	t.Setenv("TEST_PATH", originalPath)

	filter := environment.NewFilter([]string{"TEST_PATH"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expected    map[string]string
		expectError bool
	}{
		{
			name:        "self reference to system env",
			globalEnv:   []string{"TEST_PATH=/custom:${TEST_PATH}"},
			allowlist:   []string{"TEST_PATH"},
			expected:    map[string]string{"TEST_PATH": "/custom:/usr/bin:/bin"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.ExpandedEnv)
		})
	}
}

// TestExpandGlobalEnv_CircularReference tests circular reference detection
func TestExpandGlobalEnv_CircularReference(t *testing.T) {
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expectError bool
	}{
		{
			name:        "direct circular reference",
			globalEnv:   []string{"A=${A}"},
			allowlist:   []string{"HOME"},
			expectError: true,
		},
		{
			name:        "indirect circular reference",
			globalEnv:   []string{"A=${B}", "B=${A}"},
			allowlist:   []string{"HOME"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestExpandGlobalEnv_DuplicateKey tests duplicate key detection
func TestExpandGlobalEnv_DuplicateKey(t *testing.T) {
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expectError bool
	}{
		{
			name:        "duplicate key",
			globalEnv:   []string{"VAR1=value1", "VAR1=value2"},
			allowlist:   []string{"HOME"},
			expectError: true,
		},
		{
			name:        "no duplicate key",
			globalEnv:   []string{"VAR1=value1", "VAR2=value2"},
			allowlist:   []string{"HOME"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestExpandGlobalEnv_InvalidFormat tests invalid format detection
func TestExpandGlobalEnv_InvalidFormat(t *testing.T) {
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expectError bool
	}{
		{
			name:        "missing equals sign",
			globalEnv:   []string{"VAR1_NO_EQUALS"},
			allowlist:   []string{"HOME"},
			expectError: true,
		},
		{
			name:        "invalid key format",
			globalEnv:   []string{"123VAR=value"},
			allowlist:   []string{"HOME"},
			expectError: true,
		},
		{
			name:        "reserved prefix",
			globalEnv:   []string{"__RUNNER_TEST=value"},
			allowlist:   []string{"HOME"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestExpandGlobalEnv_AllowlistViolation tests allowlist violation errors
func TestExpandGlobalEnv_AllowlistViolation(t *testing.T) {
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	// Set up test environment variable
	t.Setenv("FORBIDDEN_VAR", "forbidden_value")

	tests := []struct {
		name        string
		globalEnv   []string
		allowlist   []string
		expectError bool
	}{
		{
			name:        "reference forbidden system env",
			globalEnv:   []string{"TEST_VAR=${FORBIDDEN_VAR}"},
			allowlist:   []string{"HOME"}, // FORBIDDEN_VAR not in allowlist
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestExpandGlobalEnv_Empty tests empty and nil cases
func TestExpandGlobalEnv_Empty(t *testing.T) {
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name      string
		globalEnv []string
		allowlist []string
	}{
		{
			name:      "empty array",
			globalEnv: []string{},
			allowlist: []string{"HOME"},
		},
		{
			name:      "nil array",
			globalEnv: nil,
			allowlist: []string{"HOME"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalEnv(cfg, expander, nil)

			require.NoError(t, err)

			// After the change, expandEnvironment always returns an empty map instead of nil
			assert.NotNil(t, cfg.ExpandedEnv)
			assert.Empty(t, cfg.ExpandedEnv)
		})
	}
}

// TestExpandGlobalEnv_EmptyValue tests that Global.Env can have empty string values
func TestExpandGlobalEnv_EmptyValue(t *testing.T) {
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	cfg := &runnertypes.GlobalConfig{
		Env:          []string{"EMPTY_VAR=", "NORMAL_VAR=value"},
		EnvAllowlist: []string{"HOME"},
	}

	err := config.ExpandGlobalEnv(cfg, expander, nil)
	require.NoError(t, err)

	assert.Equal(t, "", cfg.ExpandedEnv["EMPTY_VAR"])
	assert.Equal(t, "value", cfg.ExpandedEnv["NORMAL_VAR"])
}

// TestExpandGlobalEnv_SpecialCharacters tests that Global.Env handles special characters correctly
func TestExpandGlobalEnv_SpecialCharacters(t *testing.T) {
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	cfg := &runnertypes.GlobalConfig{
		Env: []string{
			"URL=https://example.com:8080/path?query=value&other=123",
			"PATH_VAR=/usr/local/bin:/usr/bin:/bin",
			"SPECIAL=value-with_special.chars@123",
		},
		EnvAllowlist: []string{"HOME"},
	}

	err := config.ExpandGlobalEnv(cfg, expander, nil)
	require.NoError(t, err)

	assert.Equal(t, "https://example.com:8080/path?query=value&other=123", cfg.ExpandedEnv["URL"])
	assert.Equal(t, "/usr/local/bin:/usr/bin:/bin", cfg.ExpandedEnv["PATH_VAR"])
	assert.Equal(t, "value-with_special.chars@123", cfg.ExpandedEnv["SPECIAL"])
}

// TestConfigLoader_GlobalEnvIntegration tests Config Loader integration with Global.Env
func TestConfigLoader_GlobalEnvIntegration(t *testing.T) {
	// Set up test environment variable
	t.Setenv("HOME", "/test/home")

	// Sample TOML content with Global.Env
	tomlContent := `[global]
env = ["BASE_DIR=/opt/app", "LOG_LEVEL=info", "ECHO_CMD=/bin/echo"]
env_allowlist = ["HOME"]
verify_files = ["${BASE_DIR}/verify.sh", "${HOME}/script.sh"]

[[groups]]
name = "test_group"
[[groups.commands]]
name = "test_cmd"
cmd = "${ECHO_CMD}"
args = ["${BASE_DIR}"]`

	// Load configuration
	loader := config.NewLoader()
	cfg, err := loader.LoadConfig([]byte(tomlContent))
	require.NoError(t, err)

	// Verify Global.ExpandedEnv was populated
	require.NotNil(t, cfg.Global.ExpandedEnv)
	assert.Equal(t, "/opt/app", cfg.Global.ExpandedEnv["BASE_DIR"])
	assert.Equal(t, "info", cfg.Global.ExpandedEnv["LOG_LEVEL"])

	// Verify Global.VerifyFiles was expanded correctly
	expectedVerifyFiles := []string{"/opt/app/verify.sh", "/test/home/script.sh"}
	assert.Equal(t, expectedVerifyFiles, cfg.Global.ExpandedVerifyFiles)

	// Verify the group and command structure
	require.Len(t, cfg.Groups, 1)
	assert.Equal(t, "test_group", cfg.Groups[0].Name)
	require.Len(t, cfg.Groups[0].Commands, 1)
	assert.Equal(t, "test_cmd", cfg.Groups[0].Commands[0].Name)
	assert.Equal(t, "${ECHO_CMD}", cfg.Groups[0].Commands[0].Cmd)
	assert.Equal(t, "/bin/echo", cfg.Groups[0].Commands[0].ExpandedCmd)
	assert.Equal(t, []string{"${BASE_DIR}"}, cfg.Groups[0].Commands[0].Args)
	assert.Equal(t, []string{"/opt/app"}, cfg.Groups[0].Commands[0].ExpandedArgs)
}

// TestConfigLoader_GlobalEnvError tests error handling in Global.Env expansion
func TestConfigLoader_GlobalEnvError(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		expectError bool
		errorText   string
	}{
		{
			name: "duplicate global env key",
			tomlContent: `[global]
env = ["VAR1=value1", "VAR1=value2"]`,
			expectError: true,
			errorText:   "duplicate environment variable",
		},
		{
			name: "invalid global env format",
			tomlContent: `[global]
env = ["INVALID_FORMAT"]`,
			expectError: true,
			errorText:   "malformed environment variable",
		},
		{
			name: "reserved prefix in global env",
			tomlContent: `[global]
env = ["__RUNNER_TEST=value"]`,
			expectError: true,
			errorText:   "reserved prefix",
		},
		{
			name: "circular reference in global env",
			tomlContent: `[global]
env = ["A=${B}", "B=${A}"]`,
			expectError: true,
			errorText:   "circular variable reference detected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := config.NewLoader()
			_, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorText)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestConfigLoader_BackwardCompatibility tests backward compatibility with configs without Global.Env
func TestConfigLoader_BackwardCompatibility(t *testing.T) {
	// TOML content without Global.Env (existing config format)
	tomlContent := `[global]
env_allowlist = ["HOME", "USER"]
verify_files = ["${HOME}/verify.sh"]

[[groups]]
name = "test_group"
[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["hello"]`

	// Set up test environment variable
	t.Setenv("HOME", "/test/home")

	// Load configuration
	loader := config.NewLoader()
	cfg, err := loader.LoadConfig([]byte(tomlContent))
	require.NoError(t, err)

	// Verify Global.ExpandedEnv is empty map (no Global.Env defined)
	// After the change, expandEnvironment always returns an empty map instead of nil
	assert.NotNil(t, cfg.Global.ExpandedEnv)
	assert.Empty(t, cfg.Global.ExpandedEnv)

	// Verify Global.VerifyFiles was expanded correctly using system env
	expectedVerifyFiles := []string{"/test/home/verify.sh"}
	assert.Equal(t, expectedVerifyFiles, cfg.Global.ExpandedVerifyFiles)

	// Verify the group and command structure
	require.Len(t, cfg.Groups, 1)
	assert.Equal(t, "test_group", cfg.Groups[0].Name)
	require.Len(t, cfg.Groups[0].Commands, 1)
	assert.Equal(t, "test_cmd", cfg.Groups[0].Commands[0].Name)
	assert.Equal(t, "echo", cfg.Groups[0].Commands[0].Cmd)
	assert.Equal(t, []string{"hello"}, cfg.Groups[0].Commands[0].Args)
}

// TestExpandGlobalVerifyFiles_WithGlobalEnv tests Global.VerifyFiles expansion with Global.Env references
func TestExpandGlobalVerifyFiles_WithGlobalEnv(t *testing.T) {
	t.Setenv("HOME", "/home/test")
	filter := environment.NewFilter([]string{"HOME"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		verifyFiles []string
		allowlist   []string
		expected    []string
		expectError bool
	}{
		{
			name:        "reference global env in verify files",
			globalEnv:   []string{"BASE_DIR=/opt/app"},
			verifyFiles: []string{"${BASE_DIR}/verify.sh", "${BASE_DIR}/check.py"},
			allowlist:   []string{"HOME"},
			expected:    []string{"/opt/app/verify.sh", "/opt/app/check.py"},
			expectError: false,
		},
		{
			name:        "mixed global env and system env",
			globalEnv:   []string{"APP_DIR=/opt/myapp"},
			verifyFiles: []string{"${APP_DIR}/script.sh", "${HOME}/user_script.sh"},
			allowlist:   []string{"HOME"},
			expected:    []string{"/opt/myapp/script.sh", "/home/test/user_script.sh"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				VerifyFiles:  tt.verifyFiles,
				EnvAllowlist: tt.allowlist,
			}

			// First expand Global.Env
			err := config.ExpandGlobalEnv(cfg, expander, nil)
			require.NoError(t, err)

			// Then expand Global.VerifyFiles
			err = config.ExpandGlobalVerifyFiles(cfg, filter, expander)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.ExpandedVerifyFiles)
		})
	}
}

// TestExpandGlobalVerifyFiles_SystemEnv tests Global.VerifyFiles expansion with system environment variables
func TestExpandGlobalVerifyFiles_SystemEnv(t *testing.T) {
	t.Setenv("TEST_BASE", "/test/base")

	filter := environment.NewFilter([]string{"TEST_BASE"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		verifyFiles []string
		allowlist   []string
		expected    []string
		expectError bool
	}{
		{
			name:        "reference system env only",
			verifyFiles: []string{"${TEST_BASE}/verify.sh"},
			allowlist:   []string{"TEST_BASE"},
			expected:    []string{"/test/base/verify.sh"},
			expectError: false,
		},
		{
			name:        "reference system env not in allowlist",
			verifyFiles: []string{"${TEST_BASE}/verify.sh"},
			allowlist:   []string{"OTHER_VAR"},
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				VerifyFiles:  tt.verifyFiles,
				EnvAllowlist: tt.allowlist,
			}

			err := config.ExpandGlobalVerifyFiles(cfg, filter, expander)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.ExpandedVerifyFiles)
		})
	}
}

// TestExpandGlobalVerifyFiles_Priority tests priority order: Global.Env > System Env
func TestExpandGlobalVerifyFiles_Priority(t *testing.T) {
	// Set system environment variable
	t.Setenv("TEST_VAR", "system_value")

	filter := environment.NewFilter([]string{"TEST_VAR"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name        string
		globalEnv   []string
		verifyFiles []string
		allowlist   []string
		expected    []string
		expectError bool
	}{
		{
			name:        "global env overrides system env",
			globalEnv:   []string{"TEST_VAR=global_value"},
			verifyFiles: []string{"${TEST_VAR}/verify.sh"},
			allowlist:   []string{"TEST_VAR"},
			expected:    []string{"global_value/verify.sh"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &runnertypes.GlobalConfig{
				Env:          tt.globalEnv,
				VerifyFiles:  tt.verifyFiles,
				EnvAllowlist: tt.allowlist,
			}

			// First expand Global.Env
			err := config.ExpandGlobalEnv(cfg, expander, nil)
			require.NoError(t, err)

			// Then expand Global.VerifyFiles
			err = config.ExpandGlobalVerifyFiles(cfg, filter, expander)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.ExpandedVerifyFiles)
		})
	}
}

// TestExpandGroupEnv_Basic tests basic Group.Env expansion
func TestExpandGroupEnv_Basic(t *testing.T) {
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME", "USER"},
		ExpandedEnv:  map[string]string{}, // Empty global env
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"VAR1=value1", "VAR2=value2"},
		EnvAllowlist: nil, // Should inherit from global
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)

	expected := map[string]string{
		"VAR1": "value1",
		"VAR2": "value2",
	}
	assert.Equal(t, expected, group.ExpandedEnv)
}

// TestExpandGroupEnv_ReferenceGlobal tests Group.Env referencing Global.ExpandedEnv
func TestExpandGroupEnv_ReferenceGlobal(t *testing.T) {
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"HOME"},
		ExpandedEnv:  map[string]string{"BASE_DIR": "/opt/app", "LOG_LEVEL": "info"},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"APP_DIR=${BASE_DIR}/myapp", "CONFIG_FILE=${BASE_DIR}/config.ini"},
		EnvAllowlist: nil, // Should inherit from global
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)

	expected := map[string]string{
		"APP_DIR":     "/opt/app/myapp",
		"CONFIG_FILE": "/opt/app/config.ini",
	}
	assert.Equal(t, expected, group.ExpandedEnv)
}

// TestExpandGroupEnv_ReferenceSystemEnv tests Group.Env referencing system environment
func TestExpandGroupEnv_ReferenceSystemEnv(t *testing.T) {
	// Set system environment variable
	t.Setenv("SYSTEM_VAR", "system_value")

	filter := environment.NewFilter([]string{"SYSTEM_VAR"})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"SYSTEM_VAR"},
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"GROUP_VAR=${SYSTEM_VAR}/suffix"},
		EnvAllowlist: nil, // Should inherit from global allowlist
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)

	expected := map[string]string{
		"GROUP_VAR": "system_value/suffix",
	}
	assert.Equal(t, expected, group.ExpandedEnv)
}

// TestExpandGroupEnv_AllowlistInherit tests allowlist inheritance
func TestExpandGroupEnv_AllowlistInherit(t *testing.T) {
	// Set system environment variable
	t.Setenv("ALLOWED_VAR", "allowed_value")
	t.Setenv("FORBIDDEN_VAR", "forbidden_value")

	filter := environment.NewFilter([]string{"ALLOWED_VAR", "FORBIDDEN_VAR"})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"ALLOWED_VAR"}, // Only allow ALLOWED_VAR
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"GROUP_VAR=${ALLOWED_VAR}/suffix"},
		EnvAllowlist: nil, // Should inherit global allowlist (ALLOWED_VAR only)
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)

	expected := map[string]string{
		"GROUP_VAR": "allowed_value/suffix",
	}
	assert.Equal(t, expected, group.ExpandedEnv)

	// Test that forbidden variable causes error
	group.Env = []string{"GROUP_VAR=${FORBIDDEN_VAR}/suffix"}
	err = config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.Error(t, err)
}

// TestExpandGroupEnv_AllowlistOverride tests allowlist override
func TestExpandGroupEnv_AllowlistOverride(t *testing.T) {
	// Set system environment variables
	t.Setenv("GLOBAL_ALLOWED", "global_value")
	t.Setenv("GROUP_ALLOWED", "group_value")

	filter := environment.NewFilter([]string{"GLOBAL_ALLOWED", "GROUP_ALLOWED"})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"GLOBAL_ALLOWED"}, // Global allows GLOBAL_ALLOWED
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"GROUP_VAR=${GROUP_ALLOWED}/suffix"},
		EnvAllowlist: []string{"GROUP_ALLOWED"}, // Group overrides to allow GROUP_ALLOWED
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)

	expected := map[string]string{
		"GROUP_VAR": "group_value/suffix",
	}
	assert.Equal(t, expected, group.ExpandedEnv)

	// Test that global allowed var is now forbidden
	group.Env = []string{"GROUP_VAR=${GLOBAL_ALLOWED}/suffix"}
	err = config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.Error(t, err)
}

// TestExpandGroupEnv_AllowlistReject tests allowlist rejection (empty slice)
func TestExpandGroupEnv_AllowlistReject(t *testing.T) {
	// Set system environment variable
	t.Setenv("SYSTEM_VAR", "system_value")

	filter := environment.NewFilter([]string{"SYSTEM_VAR"})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"SYSTEM_VAR"}, // Global allows SYSTEM_VAR
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"GROUP_VAR=${SYSTEM_VAR}/suffix"},
		EnvAllowlist: []string{}, // Empty slice should reject all system env vars
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.Error(t, err) // Should fail because SYSTEM_VAR is not allowed
}

// TestExpandGroupEnv_CircularReference tests circular reference detection
func TestExpandGroupEnv_CircularReference(t *testing.T) {
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{},
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"VAR1=${VAR2}/suffix", "VAR2=${VAR1}/suffix"}, // Circular reference
		EnvAllowlist: nil,
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.Error(t, err) // Should detect circular reference
}

// TestExpandGroupEnv_DuplicateKey tests duplicate key detection
func TestExpandGroupEnv_DuplicateKey(t *testing.T) {
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{},
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"VAR1=value1", "VAR1=value2"}, // Duplicate key
		EnvAllowlist: nil,
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.Error(t, err) // Should detect duplicate key
}

// TestExpandGroupEnv_Empty tests empty and nil cases
func TestExpandGroupEnv_Empty(t *testing.T) {
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{},
		ExpandedEnv:  map[string]string{},
	}

	// Test nil Env
	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          nil,
		EnvAllowlist: nil,
	}

	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{}, group.ExpandedEnv)

	// Test empty Env
	group.Env = []string{}
	err = config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{}, group.ExpandedEnv)
}

// TestExpandGroupVerifyFiles_WithGroupEnv tests Group.VerifyFiles expansion with Group.Env
func TestExpandGroupVerifyFiles_WithGroupEnv(t *testing.T) {
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{},
		ExpandedEnv:  map[string]string{},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"GROUP_DIR=/opt/group", "FILE_NAME=verify.sh"},
		VerifyFiles:  []string{"${GROUP_DIR}/${FILE_NAME}"},
		EnvAllowlist: nil, // Inherit from global
	}

	// First expand Group.Env
	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)

	// Then expand Group.VerifyFiles
	err = config.ExpandGroupVerifyFiles(group, global, filter, expander)
	require.NoError(t, err)

	expected := []string{"/opt/group/verify.sh"}
	assert.Equal(t, expected, group.ExpandedVerifyFiles)
}

// TestExpandGroupVerifyFiles_WithGlobalEnv tests Group.VerifyFiles expansion with Global.Env
func TestExpandGroupVerifyFiles_WithGlobalEnv(t *testing.T) {
	filter := environment.NewFilter([]string{})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{},
		ExpandedEnv:  map[string]string{"BASE_DIR": "/opt/app", "LOG_LEVEL": "info"},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{},
		VerifyFiles:  []string{"${BASE_DIR}/verify.sh", "${BASE_DIR}/logs/check.sh"},
		EnvAllowlist: nil, // Inherit from global
	}

	// First expand Group.Env (empty in this case)
	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)

	// Then expand Group.VerifyFiles with Global.Env
	err = config.ExpandGroupVerifyFiles(group, global, filter, expander)
	require.NoError(t, err)

	expected := []string{"/opt/app/verify.sh", "/opt/app/logs/check.sh"}
	assert.Equal(t, expected, group.ExpandedVerifyFiles)
}

// TestExpandGroupVerifyFiles_Priority tests priority: Group.Env > Global.Env > System Env
func TestExpandGroupVerifyFiles_Priority(t *testing.T) {
	// Set system environment variable
	t.Setenv("TEST_VAR", "system_value")

	filter := environment.NewFilter([]string{"TEST_VAR"})
	expander := environment.NewVariableExpander(filter)

	global := &runnertypes.GlobalConfig{
		EnvAllowlist: []string{"TEST_VAR"},
		ExpandedEnv:  map[string]string{"TEST_VAR": "global_value"},
	}

	group := &runnertypes.CommandGroup{
		Name:         "test_group",
		Env:          []string{"TEST_VAR=group_value"},
		VerifyFiles:  []string{"${TEST_VAR}/verify.sh"},
		EnvAllowlist: nil, // Inherit from global
	}

	// First expand Group.Env
	err := config.ExpandGroupEnv(group, expander, nil, global.ExpandedEnv, global.EnvAllowlist)
	require.NoError(t, err)

	// Then expand Group.VerifyFiles - should use Group.Env value (highest priority)
	err = config.ExpandGroupVerifyFiles(group, global, filter, expander)
	require.NoError(t, err)

	expected := []string{"group_value/verify.sh"}
	assert.Equal(t, expected, group.ExpandedVerifyFiles)
}

func TestExpandCommandEnv(t *testing.T) {
	filter := environment.NewFilter([]string{"PATH", "HOME", "USER"})
	expander := environment.NewVariableExpander(filter)

	tests := []struct {
		name         string
		cmd          runnertypes.Command
		groupName    string
		allowlist    []string
		baseEnv      map[string]string
		expectedVars map[string]string
		expectError  bool
		expectedErr  error
	}{
		{
			name: "process simple command env variables",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"FOO=bar", "BAZ=qux"},
			},
			groupName: "test_group",
			allowlist: []string{"PATH", "HOME"},
			baseEnv:   nil,
			expectedVars: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
		{
			name: "process command env variables with expansion",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"PATH=/custom/path", "NEW_VAR=value"},
			},
			groupName: "test_group",
			allowlist: []string{"PATH", "HOME"},
			baseEnv:   nil,
			expectedVars: map[string]string{
				"PATH":    "/custom/path",
				"NEW_VAR": "value",
			},
		},
		{
			name: "reject invalid environment variable format",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"VALID=value", "INVALID_NO_EQUALS", "ANOTHER=valid"},
			},
			groupName:   "test_group",
			allowlist:   []string{"PATH"},
			baseEnv:     nil,
			expectError: true,
			expectedErr: config.ErrMalformedEnvVariable,
		},
		{
			name: "reject dangerous variable value",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"DANGEROUS=value; rm -rf /"},
			},
			groupName:   "test_group",
			allowlist:   []string{"PATH"},
			baseEnv:     nil,
			expectError: true,
		},
		{
			name: "reject invalid variable name",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env:  []string{"123INVALID=value"},
			},
			groupName:   "test_group",
			allowlist:   []string{"PATH"},
			baseEnv:     nil,
			expectError: true,
			expectedErr: config.ErrInvalidEnvKey,
		},
		{
			name: "cmd.Env variable ignored due to baseEnv conflict",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env: []string{
					"__RUNNER_DATETIME=user_override", // This should be ignored
					"CUSTOM_VAR=user_value",           // This should be accepted
				},
			},
			groupName: "test_group",
			allowlist: []string{},
			baseEnv: map[string]string{
				"__RUNNER_DATETIME": "202510051430.123", // Auto-generated value
				"__RUNNER_PID":      "12345",
			},
			expectedVars: map[string]string{
				"CUSTOM_VAR": "user_value", // Only user's non-conflicting variable
			},
		},
		{
			name: "multiple cmd.Env conflicts with baseEnv",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env: []string{
					"__RUNNER_DATETIME=override1", // Should be ignored
					"__RUNNER_PID=override2",      // Should be ignored
					"VALID_VAR=accepted",          // Should be accepted
				},
			},
			groupName: "test_group",
			allowlist: []string{},
			baseEnv: map[string]string{
				"__RUNNER_DATETIME": "202510051430.123",
				"__RUNNER_PID":      "12345",
			},
			expectedVars: map[string]string{
				"VALID_VAR": "accepted",
			},
		},
		{
			name: "no conflicts - all cmd.Env accepted",
			cmd: runnertypes.Command{
				Name: "test_cmd",
				Env: []string{
					"VAR1=value1",
					"VAR2=value2",
				},
			},
			groupName: "test_group",
			allowlist: []string{},
			baseEnv: map[string]string{
				"__RUNNER_DATETIME": "202510051430.123",
			},
			expectedVars: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// In this test, baseEnv represents autoEnv (automatic environment variables)
			// globalEnv and groupEnv are nil since we're testing Command.Env expansion in isolation
			cmd := tt.cmd // Create a copy to avoid modifying test data
			err := config.ExpandCommandEnv(&cmd, expander, tt.baseEnv, nil, tt.allowlist, nil, nil, tt.groupName)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErr != nil {
					assert.ErrorIs(t, err, tt.expectedErr, "Expected error type %v, got %v", tt.expectedErr, err)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedVars, cmd.ExpandedEnv)
		})
	}
}

// ============================================================================
// Phase 2: InternalVariableExpander Tests (TDD)
// ============================================================================

func TestExpandString_Basic(t *testing.T) {
	// Test basic variable expansion with %{VAR} syntax
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
		wantErr  bool
	}{
		{
			name:     "single variable expansion",
			input:    "prefix_%{var1}_suffix",
			vars:     map[string]string{"var1": "value1"},
			expected: "prefix_value1_suffix",
			wantErr:  false,
		},
		{
			name:     "variable at start",
			input:    "%{var1}_suffix",
			vars:     map[string]string{"var1": "start"},
			expected: "start_suffix",
			wantErr:  false,
		},
		{
			name:     "variable at end",
			input:    "prefix_%{var1}",
			vars:     map[string]string{"var1": "end"},
			expected: "prefix_end",
			wantErr:  false,
		},
		{
			name:     "variable only",
			input:    "%{var1}",
			vars:     map[string]string{"var1": "only"},
			expected: "only",
			wantErr:  false,
		},
		{
			name:     "no variables",
			input:    "plain text",
			vars:     map[string]string{"var1": "unused"},
			expected: "plain text",
			wantErr:  false,
		},
		{
			name:     "empty string",
			input:    "",
			vars:     map[string]string{"var1": "unused"},
			expected: "",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandString_Multiple(t *testing.T) {
	// Test multiple variable expansions in a single string
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:     "two variables",
			input:    "%{var1}/%{var2}",
			vars:     map[string]string{"var1": "a", "var2": "b"},
			expected: "a/b",
		},
		{
			name:     "three variables",
			input:    "%{var1}/%{var2}/%{var3}",
			vars:     map[string]string{"var1": "x", "var2": "y", "var3": "z"},
			expected: "x/y/z",
		},
		{
			name:     "same variable multiple times",
			input:    "%{var1}_%{var1}_%{var1}",
			vars:     map[string]string{"var1": "repeat"},
			expected: "repeat_repeat_repeat",
		},
		{
			name:     "variables with text",
			input:    "start_%{a}_middle_%{b}_end",
			vars:     map[string]string{"a": "AAA", "b": "BBB"},
			expected: "start_AAA_middle_BBB_end",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandString_Nested(t *testing.T) {
	// Test nested variable expansions (variable values containing %{VAR} references)
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:  "two-level nesting",
			input: "%{var2}",
			vars: map[string]string{
				"var1": "x",
				"var2": "%{var1}/y",
			},
			expected: "x/y",
		},
		{
			name:  "three-level nesting",
			input: "%{var3}",
			vars: map[string]string{
				"var1": "x",
				"var2": "%{var1}/y",
				"var3": "%{var2}/z",
			},
			expected: "x/y/z",
		},
		{
			name:  "complex nested expansion",
			input: "%{final}",
			vars: map[string]string{
				"base":  "/opt/app",
				"logs":  "%{base}/logs",
				"temp":  "%{logs}/temp",
				"final": "%{temp}/output.log",
			},
			expected: "/opt/app/logs/temp/output.log",
		},
		{
			name:  "nested with multiple references",
			input: "%{combined}",
			vars: map[string]string{
				"a":        "A",
				"b":        "B",
				"combined": "%{a}_%{b}",
			},
			expected: "A_B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "vars")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandString_UndefinedVariable(t *testing.T) {
	// Test error handling for undefined variables
	tests := []struct {
		name        string
		input       string
		vars        map[string]string
		expectedVar string
	}{
		{
			name:        "undefined variable",
			input:       "%{undefined}",
			vars:        map[string]string{"defined": "value"},
			expectedVar: "undefined",
		},
		{
			name:        "undefined in middle",
			input:       "start_%{missing}_end",
			vars:        map[string]string{},
			expectedVar: "missing",
		},
		{
			name:        "one defined, one undefined",
			input:       "%{defined}/%{undefined}",
			vars:        map[string]string{"defined": "ok"},
			expectedVar: "undefined",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")

			require.Error(t, err)
			assert.Empty(t, result)

			var undefinedErr *config.ErrUndefinedVariableDetail
			assert.ErrorAs(t, err, &undefinedErr)
			assert.Equal(t, tt.expectedVar, undefinedErr.VariableName)
			assert.Equal(t, "global", undefinedErr.Level)
			assert.Equal(t, "test_field", undefinedErr.Field)
		})
	}
}

func TestExpandString_CircularReference(t *testing.T) {
	// Test circular reference detection
	tests := []struct {
		name            string
		input           string
		vars            map[string]string
		expectedVarName string
	}{
		{
			name:  "direct self-reference",
			input: "%{A}",
			vars: map[string]string{
				"A": "%{A}",
			},
			expectedVarName: "A",
		},
		{
			name:  "two-variable cycle",
			input: "%{A}",
			vars: map[string]string{
				"A": "%{B}",
				"B": "%{A}",
			},
			expectedVarName: "A",
		},
		{
			name:  "three-variable cycle",
			input: "%{A}",
			vars: map[string]string{
				"A": "%{B}",
				"B": "%{C}",
				"C": "%{A}",
			},
			expectedVarName: "A",
		},
		{
			name:  "cycle with prefix",
			input: "%{B}",
			vars: map[string]string{
				"A": "prefix_%{B}",
				"B": "suffix_%{A}",
			},
			expectedVarName: "B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "vars")

			require.Error(t, err)
			assert.Empty(t, result)

			var circularErr *config.ErrCircularReferenceDetail
			// Use require.ErrorAs to ensure the typed error value is set for further assertions
			require.ErrorAs(t, err, &circularErr)
			require.NotNil(t, circularErr)
			assert.Equal(t, "global", circularErr.Level)
			assert.Equal(t, "vars", circularErr.Field)
			// The error should mention the variable involved in the cycle
			assert.Contains(t, err.Error(), "circular reference")
			// Verify the variable name reported matches the expected one from the test case
			if tt.expectedVarName != "" {
				assert.Equal(t, tt.expectedVarName, circularErr.VariableName)
			}
		})
	}
}

func TestExpandString_MaxRecursionDepth(t *testing.T) {
	// Test maximum recursion depth limit to prevent stack overflow
	_ = slog.Default()

	// Create a chain of variables that exceeds MaxRecursionDepth
	// var1 -> var2 -> var3 -> ... -> var101
	vars := make(map[string]string)
	for i := 1; i <= config.MaxRecursionDepth+1; i++ {
		varName := fmt.Sprintf("var%d", i)
		if i < config.MaxRecursionDepth+1 {
			nextVarName := fmt.Sprintf("var%d", i+1)
			vars[varName] = fmt.Sprintf("value_%s", "%{"+nextVarName+"}")
		} else {
			vars[varName] = "final_value"
		}
	}

	result, err := config.ExpandString("%{var1}", vars, "global", "vars")

	require.Error(t, err)
	assert.Empty(t, result)

	var maxDepthErr *config.ErrMaxRecursionDepthExceededDetail
	assert.ErrorAs(t, err, &maxDepthErr)
	assert.Equal(t, "global", maxDepthErr.Level)
	assert.Equal(t, "vars", maxDepthErr.Field)
	assert.Equal(t, config.MaxRecursionDepth, maxDepthErr.MaxDepth)
	assert.Contains(t, err.Error(), "maximum recursion depth exceeded")
}

func TestExpandString_EscapeSequence(t *testing.T) {
	// Test escape sequence handling
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		expected string
	}{
		{
			name:     "escape percent",
			input:    `literal \%{var1}`,
			vars:     map[string]string{"var1": "value1"},
			expected: "literal %{var1}",
		},
		{
			name:     "escape backslash",
			input:    `path\\name`,
			vars:     map[string]string{},
			expected: `path\name`,
		},
		{
			name:     "mixed escapes",
			input:    `\%{var1} and \\path`,
			vars:     map[string]string{"var1": "value"},
			expected: `%{var1} and \path`,
		},
		{
			name:     "escape before variable",
			input:    `\\%{var1}`,
			vars:     map[string]string{"var1": "test"},
			expected: `\test`,
		},
		{
			name:     "multiple escapes",
			input:    `\%{a} \%{b} \\c`,
			vars:     map[string]string{"a": "A", "b": "B"},
			expected: `%{a} %{b} \c`,
		},
		{
			name:     "escape and expansion",
			input:    `\%{literal} %{var1}`,
			vars:     map[string]string{"literal": "L", "var1": "expanded"},
			expected: `%{literal} expanded`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExpandString_InvalidEscape(t *testing.T) {
	// Test invalid escape sequence handling
	tests := []struct {
		name             string
		input            string
		vars             map[string]string
		expectedSequence string
	}{
		{
			name:             "invalid escape dollar",
			input:            `\$invalid`,
			vars:             map[string]string{},
			expectedSequence: `\$`,
		},
		{
			name:             "invalid escape x",
			input:            `\xtest`,
			vars:             map[string]string{},
			expectedSequence: `\x`,
		},
		{
			name:             "invalid escape n",
			input:            `\ntest`,
			vars:             map[string]string{},
			expectedSequence: `\n`,
		},
		{
			name:             "invalid escape in middle",
			input:            `prefix_\t_suffix`,
			vars:             map[string]string{},
			expectedSequence: `\t`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = slog.Default()
			result, err := config.ExpandString(tt.input, tt.vars, "global", "test_field")

			require.Error(t, err)
			assert.Empty(t, result)

			var escapeErr *config.ErrInvalidEscapeSequenceDetail
			assert.ErrorAs(t, err, &escapeErr)
			assert.Equal(t, tt.expectedSequence, escapeErr.Sequence)
			assert.Equal(t, "global", escapeErr.Level)
			assert.Equal(t, "test_field", escapeErr.Field)
		})
	}
}

func TestExpandString_UnclosedVariableReference(t *testing.T) {
	// Test unclosed variable reference detection
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "unclosed at end",
			input: "prefix_%{var",
		},
		{
			name:  "unclosed in middle",
			input: "start_%{var_middle",
		},
		{
			name:  "only opening brace",
			input: "%{",
		},
		{
			name:  "unclosed with content after",
			input: "%{\var_more text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := config.ExpandString(tt.input, nil, "global", "test_field")

			require.Error(t, err)
			assert.Empty(t, result)

			var unclosedErr *config.ErrUnclosedVariableReferenceDetail
			assert.ErrorAs(t, err, &unclosedErr)
			assert.Equal(t, "global", unclosedErr.Level)
			assert.Equal(t, "test_field", unclosedErr.Field)
			assert.Equal(t, tt.input, unclosedErr.Context)
		})
	}
}
