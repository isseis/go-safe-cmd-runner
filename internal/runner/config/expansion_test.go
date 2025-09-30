// Package config provides tests for the variable expansion functionality.
package config_test

import (
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
			filter := environment.NewFilter(cfg)
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

			// Test the expansion
			expandedGroup, err := config.ExpandCommandStrings(group, expander)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
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
			filter := environment.NewFilter(cfg)
			expander := environment.NewVariableExpander(filter)

			// Store original for immutability check
			originalGroupName := tt.group.Name
			originalCmd1 := tt.group.Commands[0].Cmd

			// Test group expansion
			expandedGroup, err := config.ExpandCommandStrings(&tt.group, expander)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				// Verify expansion for first command
				require.Len(t, expandedGroup.Commands, 2, "Should have two commands")

				cmd1 := expandedGroup.Commands[0]
				assert.Equal(t, "/usr/bin/ls", cmd1.ExpandedCmd, "First command should be expanded")
				assert.Equal(t, []string{"-la", "/home/user"}, cmd1.ExpandedArgs, "First command args should be expanded")

				cmd2 := expandedGroup.Commands[1]
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
			filter := environment.NewFilter(cfg)
			expander := environment.NewVariableExpander(filter)

			// Create a group with single command to test
			group := &runnertypes.CommandGroup{
				Name:         "test-group",
				EnvAllowlist: []string{"VAR1", "VAR2", "VAR3"},
				Commands:     []runnertypes.Command{tt.cmd},
			}

			// Test circular reference detection
			_, err := config.ExpandCommandStrings(group, expander)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
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
			filter := environment.NewFilter(cfg)
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

			// Test security integration
			_, err := config.ExpandCommandStrings(group, expander)

			if tt.expectError {
				require.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}
