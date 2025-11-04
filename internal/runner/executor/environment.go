package executor

import (
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// EnvVar represents an environment variable with its value and origin.
type EnvVar struct {
	Value  string
	Origin string
}

// mergeEnvWithOrigin merges environment variables from envMap into result with the specified origin.
// This helper function reduces code duplication when merging environment variables from different sources.
func mergeEnvWithOrigin(result map[string]EnvVar, envMap map[string]string, origin string) {
	for k, v := range envMap {
		result[k] = EnvVar{
			Value:  v,
			Origin: origin,
		}
	}
}

// BuildProcessEnvironment builds the final process environment variables for command execution
// and tracks the origin of each variable.
//
// Merge order (lower priority to higher priority):
//  1. System environment variables (filtered by env_allowlist)
//  2. Global.ExpandedEnv (from vars/env_import)
//  3. Group.ExpandedEnv (from vars/env_import)
//  4. Command.ExpandedEnv (from command-level env_vars)
//
// Source values in the returned EnvVar.Origin field:
//   - "system": Variables from env_allowlist (system environment)
//   - "vars": Variables from global or group level vars/env_import/env_vars
//   - "command": Variables from command-level env_vars
//
// Note: Currently, we cannot distinguish between variables from env_import and vars
// because they are merged during expansion. Both are reported as "vars" at global/group level.
// This is a known limitation that could be addressed in future by tracking env_import separately.
//
// Returns:
//   - A map where keys are environment variable names and values are EnvVar structs
//     containing the variable value and its origin
func BuildProcessEnvironment(
	runtimeGlobal *runnertypes.RuntimeGlobal,
	runtimeGroup *runnertypes.RuntimeGroup,
	cmd *runnertypes.RuntimeCommand,
) map[string]EnvVar {
	result := make(map[string]EnvVar)

	// Step 1: Get system environment variables (filtered by allowlist)
	systemEnv := getSystemEnvironment()
	allowlist := runtimeGlobal.EnvAllowlist()

	for _, name := range allowlist {
		if value, ok := systemEnv[name]; ok {
			result[name] = EnvVar{
				Value:  value,
				Origin: "system",
			}
		}
	}

	// Step 2: Merge Global.ExpandedEnv (overrides system env)
	// These come from global vars/env_import/env_vars sections
	mergeEnvWithOrigin(result, runtimeGlobal.ExpandedEnv, "vars")

	// Step 3: Merge Group.ExpandedEnv (overrides global env)
	// These come from group vars/env_import/env_vars sections
	mergeEnvWithOrigin(result, runtimeGroup.ExpandedEnv, "vars")

	// Step 4: Merge Command.ExpandedEnv (overrides group env)
	// These come from command-level env_vars section
	mergeEnvWithOrigin(result, cmd.ExpandedEnv, "command")

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
