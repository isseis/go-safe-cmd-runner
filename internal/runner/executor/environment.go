package executor

import (
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

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
//   - envVars: Map of environment variables to be passed to the child process
//   - origins: Map tracking the origin of each environment variable
func BuildProcessEnvironment(
	runtimeGlobal *runnertypes.RuntimeGlobal,
	runtimeGroup *runnertypes.RuntimeGroup,
	cmd *runnertypes.RuntimeCommand,
) (envVars map[string]string, origins map[string]string) {
	result := make(map[string]string)
	originsMap := make(map[string]string)

	// Step 1: Get system environment variables (filtered by allowlist)
	systemEnv := getSystemEnvironment()
	allowlist := runtimeGlobal.EnvAllowlist()

	for _, name := range allowlist {
		if value, ok := systemEnv[name]; ok {
			result[name] = value
			originsMap[name] = "System (filtered by allowlist)"
		}
	}

	// Step 2: Merge Global.ExpandedEnv (overrides system env)
	for k, v := range runtimeGlobal.ExpandedEnv {
		result[k] = v
		originsMap[k] = "Global"
	}

	// Step 3: Merge Group.ExpandedEnv (overrides global env)
	for k, v := range runtimeGroup.ExpandedEnv {
		result[k] = v
		originsMap[k] = fmt.Sprintf("Group[%s]", runtimeGroup.Name())
	}

	// Step 4: Merge Command.ExpandedEnv (overrides group env)
	for k, v := range cmd.ExpandedEnv {
		result[k] = v
		originsMap[k] = fmt.Sprintf("Command[%s]", cmd.Name())
	}

	return result, originsMap
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
