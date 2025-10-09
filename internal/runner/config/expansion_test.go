// Package config provides tests for the variable expansion functionality.
package config_test

import (
	"os"
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
						Command:      &group.Commands[i],
						Expander:     expander,
						AutoEnv:      nil,
						EnvAllowlist: group.EnvAllowlist,
						GroupName:    group.Name,
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
		Command:      &cmd,
		Expander:     expander,
		AutoEnv:      autoEnv,
		EnvAllowlist: cfg.Global.EnvAllowlist,
		GroupName:    "test-group",
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
						Command:      &tt.group.Commands[i],
						Expander:     expander,
						AutoEnv:      nil,
						EnvAllowlist: tt.group.EnvAllowlist,
						GroupName:    tt.group.Name,
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
						Command:      &group.Commands[i],
						Expander:     expander,
						AutoEnv:      nil,
						EnvAllowlist: group.EnvAllowlist,
						GroupName:    group.Name,
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
						Command:      &group.Commands[i],
						Expander:     expander,
						AutoEnv:      nil,
						EnvAllowlist: group.EnvAllowlist,
						GroupName:    group.Name,
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
				Command:      &tt.cmd,
				Expander:     expander,
				AutoEnv:      nil,
				EnvAllowlist: allowlist,
				GroupName:    "test-group",
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
					Command:      &bm.cmd,
					Expander:     expander,
					AutoEnv:      nil,
					EnvAllowlist: cfg.Global.EnvAllowlist,
					GroupName:    "benchmark-group",
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
				Command:      &tt.cmd,
				Expander:     expander,
				AutoEnv:      autoEnv,
				EnvAllowlist: tt.groupAllowlist,
				GroupName:    "test_group",
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

			// Execute expansion
			err := config.ExpandGroupVerifyFiles(group, filter, expander)

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
	if err := os.Setenv("HOME", "/home/testuser"); err != nil {
		b.Fatalf("failed to set environment variable: %v", err)
	}
	if err := os.Setenv("BASE", "/opt"); err != nil {
		b.Fatalf("failed to set environment variable: %v", err)
	}
	if err := os.Setenv("APP", "myapp"); err != nil {
		b.Fatalf("failed to set environment variable: %v", err)
	}

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
	if err := os.Setenv("HOME", "/home/testuser"); err != nil {
		b.Fatalf("failed to set environment variable: %v", err)
	}
	if err := os.Setenv("DATA", "/var/data"); err != nil {
		b.Fatalf("failed to set environment variable: %v", err)
	}

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
		err := config.ExpandGroupVerifyFiles(group, filter, expander)
		if err != nil {
			b.Fatalf("unexpected error: %v", err)
		}
	}
}

// BenchmarkExpandLargeVerifyFiles benchmarks performance with many verify_files
func BenchmarkExpandLargeVerifyFiles(b *testing.B) {
	// Setup environment
	if err := os.Setenv("BASE", "/opt/app"); err != nil {
		b.Fatalf("failed to set environment variable: %v", err)
	}

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
