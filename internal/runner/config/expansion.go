// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ExpandCommand expands variables in a single command's Cmd and Args fields.
// It returns the expanded cmd string and expanded args slice.
// This function is exported so callers in other packages (for example bootstrap)
// can expand commands individually instead of relying on a group-level helper.
func ExpandCommand(cmd *runnertypes.Command, expander *environment.VariableExpander, allowlist []string, groupName string) (string, []string, error) {
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

// NOTE: The previous group-level helper ExpandCommandStrings has been removed
// in favor of calling ExpandCommand per-command from the bootstrap code. Tests
// or other call-sites that previously relied on the group-level helper should
// perform the same iteration themselves so they can control immutability and
// how Command.Env is pre-expanded.
