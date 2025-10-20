package executor

import (
	"maps"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// BuildProcessEnvironment builds the final process environment variables for command execution.
//
// Merge order (lower priority to higher priority):
//  1. System environment variables (filtered by env_allowlist)
//  2. Global.ExpandedEnv
//  3. Command.ExpandedEnv
//
// Returns:
//   - Map of environment variables to be passed to the child process
func BuildProcessEnvironment(
	runtimeGlobal *runnertypes.RuntimeGlobal,
	cmd *runnertypes.RuntimeCommand,
) map[string]string {
	result := make(map[string]string)

	// Step 1: Get system environment variables (filtered by allowlist)
	systemEnv := getSystemEnvironment()
	allowlist := runtimeGlobal.Spec.EnvAllowlist

	for _, name := range allowlist {
		if value, ok := systemEnv[name]; ok {
			result[name] = value
		}
	}

	// Step 2: Merge Global.ExpandedEnv (overrides system env)
	maps.Copy(result, runtimeGlobal.ExpandedEnv)

	// Step 3: Merge Command.ExpandedEnv (overrides global env)
	maps.Copy(result, cmd.ExpandedEnv)

	return result
}

// getSystemEnvironment retrieves all system environment variables as a map.
func getSystemEnvironment() map[string]string {
	result := make(map[string]string)
	for _, env := range os.Environ() {
		if key, value, ok := common.ParseKeyValue(env); ok {
			result[key] = value
		}
	}
	return result
}
