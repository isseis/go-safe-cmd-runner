// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ExpandCommand expands variables in a single command's Cmd, Args, and Env fields.
// It returns the expanded cmd string, expanded args slice, and expanded environment map.
// This function is exported so callers in other packages (for example bootstrap)
// can expand commands individually instead of relying on a group-level helper.
func ExpandCommand(cmd *runnertypes.Command, expander *environment.VariableExpander, allowlist []string, groupName string) (string, []string, map[string]string, error) {
	// Expand Command.Env variables (this handles cases like PATH=/custom/bin:${PATH})
	env, err := expander.ExpandCommandEnv(cmd, groupName, allowlist)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to expand command environment: %w", err)
	}

	// Expand command name
	expandedCmd, err := expander.ExpandString(cmd.Cmd, env, allowlist, groupName, make(map[string]bool))
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to expand command: %w", err)
	}

	// Expand command arguments
	expandedArgs, err := expander.ExpandStrings(cmd.Args, env, allowlist, groupName)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to expand args: %w", err)
	}

	return expandedCmd, expandedArgs, env, nil
}
