// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// expandCommand expands variables in a single command's cmd and args fields.
// This is an internal helper function called by ExpandCommandStrings.
// It returns the expanded cmd string and args slice.
func expandCommand(cmd *runnertypes.Command, expander *environment.VariableExpander, allowlist []string, groupName string) (string, []string, error) {
	// Build environment map from the command's Env block
	env, err := cmd.BuildEnvironmentMap()
	if err != nil {
		return "", nil, fmt.Errorf("failed to build environment map: %w", err)
	}

	// Expand command name
	expandedCmd, err := expander.ExpandString(cmd.Cmd, env, allowlist, groupName, make(map[string]bool))
	if err != nil {
		return "", nil, fmt.Errorf("failed to expand command: %w", err)
	}

	// Expand command arguments
	expandedArgs, err := expander.ExpandStrings(cmd.Args, env, allowlist, groupName)
	if err != nil {
		return "", nil, fmt.Errorf("failed to expand args: %w", err)
	}

	return expandedCmd, expandedArgs, nil
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
		expandedCmd, expandedArgs, err := expandCommand(&group.Commands[i], expander, group.EnvAllowlist, group.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to expand command strings for command %s: %w", group.Commands[i].Name, err)
		}
		// Copy the original command and set expanded values in new fields
		// Original Cmd and Args fields are preserved unchanged for immutability
		expandedGroup.Commands[i] = group.Commands[i]
		expandedGroup.Commands[i].ExpandedCmd = expandedCmd
		expandedGroup.Commands[i].ExpandedArgs = expandedArgs
	}

	return &expandedGroup, nil
}
