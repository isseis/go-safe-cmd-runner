package executor

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// BuildProcessEnvironment builds the final process environment variables for command execution.
//
// Merge order (lower priority to higher priority):
//  1. System environment variables (filtered by env_allowlist)
//  2. Global.ExpandedEnv
//  3. Group.ExpandedEnv
//  4. Command.ExpandedEnv
//
// Returns:
//   - Map of environment variables to be passed to the child process
func BuildProcessEnvironment(
	global *runnertypes.GlobalConfig,
	group *runnertypes.CommandGroup,
	cmd *runnertypes.Command,
	filter *environment.Filter,
) (map[string]string, error) {
	result := make(map[string]string)

	// Step 1: Get system environment variables (filtered by allowlist)
	systemEnv := filter.ParseSystemEnvironment(nil)
	allowlist := resolveAllowlist(global, group)

	for _, name := range allowlist {
		if value, ok := systemEnv[name]; ok {
			result[name] = value
		}
	}

	// Step 2: Merge Global.ExpandedEnv (overrides system env)
	for k, v := range global.ExpandedEnv {
		result[k] = v
	}

	// Step 3: Merge Group.ExpandedEnv (overrides global env)
	if group != nil {
		for k, v := range group.ExpandedEnv {
			result[k] = v
		}
	}

	// Step 4: Merge Command.ExpandedEnv (overrides group env)
	for k, v := range cmd.ExpandedEnv {
		result[k] = v
	}

	return result, nil
}

// resolveAllowlist determines the effective allowlist for a command.
func resolveAllowlist(global *runnertypes.GlobalConfig, group *runnertypes.CommandGroup) []string {
	if group != nil && group.EnvAllowlist != nil {
		return group.EnvAllowlist
	}
	return global.EnvAllowlist
}
