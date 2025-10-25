package debug

import (
	"fmt"
	"io"
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// PrintFinalEnvironment prints the final environment variables that will be set for the process.
// It shows the variable name, value, and origin (system/global/group/command).
func PrintFinalEnvironment(
	w io.Writer,
	envVars map[string]string,
	global *runnertypes.RuntimeGlobal,
	group *runnertypes.RuntimeGroup,
	cmd *runnertypes.RuntimeCommand,
) {
	_, _ = fmt.Fprintf(w, "===== Final Process Environment =====\n\n")

	if len(envVars) == 0 {
		_, _ = fmt.Fprintf(w, "No environment variables set.\n")
		return
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	_, _ = fmt.Fprintf(w, "Environment variables (%d):\n", len(envVars))
	for _, k := range keys {
		value := envVars[k]
		origin := determineOrigin(k, value, global, group, cmd)

		// Truncate long values for readability
		displayValue := value
		if len(displayValue) > MaxDisplayLength {
			displayValue = displayValue[:MaxDisplayLength-EllipsisLength] + "..."
		}

		_, _ = fmt.Fprintf(w, "  %s=%s\n", k, displayValue)
		_, _ = fmt.Fprintf(w, "    (from %s)\n", origin)
	}
	_, _ = fmt.Fprintln(w)
}

// determineOrigin determines the origin of an environment variable.
func determineOrigin(key, value string, global *runnertypes.RuntimeGlobal, group *runnertypes.RuntimeGroup, cmd *runnertypes.RuntimeCommand) string {
	// Check Command.ExpandedEnv
	if cmd.ExpandedEnv != nil {
		if cmdVal, ok := cmd.ExpandedEnv[key]; ok && cmdVal == value {
			return fmt.Sprintf("Command[%s]", cmd.Name())
		}
	}

	// Check Group.ExpandedEnv
	if group.ExpandedEnv != nil {
		if groupVal, ok := group.ExpandedEnv[key]; ok && groupVal == value {
			return fmt.Sprintf("Group[%s]", group.Name())
		}
	}

	// Check Global.ExpandedEnv
	if global.ExpandedEnv != nil {
		if globalVal, ok := global.ExpandedEnv[key]; ok && globalVal == value {
			return "Global"
		}
	}

	// Otherwise, it's from the system environment
	return "System (filtered by allowlist)"
}
