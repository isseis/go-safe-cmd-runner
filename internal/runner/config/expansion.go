// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ExpandVariables expands environment variables in command cmd and args fields.
// This function integrates with the CommandEnvProcessor to provide ${VAR} expansion.
func ExpandVariables(cmd *runnertypes.Command, processor *environment.CommandEnvProcessor, allowlist []string, groupName string) error {
	// Build environment map from the command's Env block
	env, err := cmd.BuildEnvironmentMap()
	if err != nil {
		return fmt.Errorf("failed to build environment map: %w", err)
	}

	// Expand command name
	expandedCmd, err := processor.Expand(cmd.Cmd, env, allowlist, groupName, make(map[string]bool))
	if err != nil {
		return fmt.Errorf("failed to expand command: %w", err)
	}
	cmd.Cmd = expandedCmd

	// Expand command arguments
	expandedArgs, err := processor.ExpandAll(cmd.Args, env, allowlist, groupName)
	if err != nil {
		return fmt.Errorf("failed to expand args: %w", err)
	}
	cmd.Args = expandedArgs

	return nil
}

// ExpandVariablesInGroup expands environment variables for all commands in a command group.
// This is a convenience function to expand variables for all commands at once.
func ExpandVariablesInGroup(group *runnertypes.CommandGroup, processor *environment.CommandEnvProcessor) error {
	for i := range group.Commands {
		err := ExpandVariables(&group.Commands[i], processor, group.EnvAllowlist, group.Name)
		if err != nil {
			return fmt.Errorf("failed to expand variables for command %s: %w", group.Commands[i].Name, err)
		}
	}
	return nil
}
