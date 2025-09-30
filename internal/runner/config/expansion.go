// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// expandCommand expands variables in a single command's cmd and args fields.
// This is an internal helper function called by ExpandCommandStrings.
// It returns a new Command with expanded values, leaving the original unchanged.
func expandCommand(cmd *runnertypes.Command, expander *environment.VariableExpander, allowlist []string, groupName string) (*runnertypes.Command, error) {
	// Build environment map from the command's Env block
	env, err := cmd.BuildEnvironmentMap()
	if err != nil {
		return nil, fmt.Errorf("failed to build environment map: %w", err)
	}

	// Expand command name
	expandedCmd, err := expander.ExpandString(cmd.Cmd, env, allowlist, groupName, make(map[string]bool))
	if err != nil {
		return nil, fmt.Errorf("failed to expand command: %w", err)
	}

	// Expand command arguments
	expandedArgs, err := expander.ExpandStrings(cmd.Args, env, allowlist, groupName)
	if err != nil {
		return nil, fmt.Errorf("failed to expand args: %w", err)
	}

	// Create a new command with expanded values (shallow copy of original, then replace expanded fields)
	expandedCommand := *cmd
	expandedCommand.Cmd = expandedCmd
	expandedCommand.Args = expandedArgs

	return &expandedCommand, nil
}

// ExpandCommandStrings expands command strings for all commands in a command group.
// This function is called during configuration loading to expand cmd and args fields
// before execution. It uses the VariableExpander to provide ${VAR} expansion.
// It returns a new CommandGroup with expanded values, leaving the original unchanged.
func ExpandCommandStrings(group *runnertypes.CommandGroup, expander *environment.VariableExpander) (*runnertypes.CommandGroup, error) {
	// Create a shallow copy of the group
	expandedGroup := *group

	// Create a new slice for expanded commands
	expandedGroup.Commands = make([]runnertypes.Command, len(group.Commands))

	// Expand each command
	for i := range group.Commands {
		expandedCmd, err := expandCommand(&group.Commands[i], expander, group.EnvAllowlist, group.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to expand command strings for command %s: %w", group.Commands[i].Name, err)
		}
		expandedGroup.Commands[i] = *expandedCmd
	}

	return &expandedGroup, nil
}
