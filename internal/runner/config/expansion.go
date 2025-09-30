// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// expandCommand expands variables in a single command's cmd and args fields.
// This is an internal helper function called by ExpandCommandStrings.
func expandCommand(cmd *runnertypes.Command, expander *environment.VariableExpander, allowlist []string, groupName string) error {
	// Build environment map from the command's Env block
	env, err := cmd.BuildEnvironmentMap()
	if err != nil {
		return fmt.Errorf("failed to build environment map: %w", err)
	}

	// Expand command name
	expandedCmd, err := expander.ExpandString(cmd.Cmd, env, allowlist, groupName, make(map[string]bool))
	if err != nil {
		return fmt.Errorf("failed to expand command: %w", err)
	}
	cmd.Cmd = expandedCmd

	// Expand command arguments
	expandedArgs, err := expander.ExpandStrings(cmd.Args, env, allowlist, groupName)
	if err != nil {
		return fmt.Errorf("failed to expand args: %w", err)
	}
	cmd.Args = expandedArgs

	return nil
}

// ExpandCommandStrings expands command strings for all commands in a command group.
// This function is called during configuration loading to expand cmd and args fields
// before execution. It uses the VariableExpander to provide ${VAR} expansion.
func ExpandCommandStrings(group *runnertypes.CommandGroup, expander *environment.VariableExpander) error {
	for i := range group.Commands {
		err := expandCommand(&group.Commands[i], expander, group.EnvAllowlist, group.Name)
		if err != nil {
			return fmt.Errorf("failed to expand command strings for command %s: %w", group.Commands[i].Name, err)
		}
	}
	return nil
}
