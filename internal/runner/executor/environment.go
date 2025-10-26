package executor

import (
	"fmt"
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
//  2. Global.ExpandedEnv
//  3. Group.ExpandedEnv
//  4. Command.ExpandedEnv
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
				Origin: "System (filtered by allowlist)",
			}
		}
	}

	// Step 2: Merge Global.ExpandedEnv (overrides system env)
	mergeEnvWithOrigin(result, runtimeGlobal.ExpandedEnv, "Global")

	// Step 3: Merge Group.ExpandedEnv (overrides global env)
	mergeEnvWithOrigin(result, runtimeGroup.ExpandedEnv, fmt.Sprintf("Group[%s]", runtimeGroup.Name()))

	// Step 4: Merge Command.ExpandedEnv (overrides group env)
	mergeEnvWithOrigin(result, cmd.ExpandedEnv, fmt.Sprintf("Command[%s]", cmd.Name()))

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
