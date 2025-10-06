// Package config provides configuration management and variable expansion for commands.
package config

import (
	"fmt"
	"maps"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// ExpandCommand expands variables in a single command's Cmd, Args, and Env fields,
// including automatic environment variables provided in autoEnv.
//
// The autoEnv map contains automatic environment variables (e.g., __RUNNER_DATETIME, __RUNNER_PID)
// that take precedence over Command.Env and are available for expansion:
//   - Command.Env can REFERENCE automatic variables (e.g., OUTPUT=${__RUNNER_DATETIME}.log)
//   - Command.Env CANNOT OVERRIDE automatic variables (conflicts are ignored with warning)
//   - If autoEnv is nil, an empty map is used (no automatic environment variables)
func ExpandCommand(cmd *runnertypes.Command, expander *environment.VariableExpander, autoEnv map[string]string, allowlist []string, groupName string) (string, []string, map[string]string, error) {
	// Use empty map if autoEnv is nil
	if autoEnv == nil {
		autoEnv = map[string]string{}
	}

	// Expand Command.Env variables (this handles cases like PATH=/custom/bin:${PATH})
	// Pass autoEnv as baseEnv to:
	// 1. Allow Command.Env to reference automatic variables (e.g., OUTPUT=${__RUNNER_DATETIME}.log)
	// 2. Prevent Command.Env from overriding automatic variables (silently ignored with warning)
	commandEnv, err := expander.ExpandCommandEnv(cmd, groupName, allowlist, autoEnv)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to expand command environment: %w", err)
	}

	// Merge command environment with automatic environment variables
	// Auto env variables are added last, taking precedence over command env for same keys
	env := make(map[string]string, len(commandEnv)+len(autoEnv))
	maps.Copy(env, commandEnv)
	maps.Copy(env, autoEnv)

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
