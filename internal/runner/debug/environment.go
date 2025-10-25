package debug

import (
	"fmt"
	"io"
	"sort"
)

// PrintFinalEnvironment prints the final environment variables that will be set for the process.
// It shows the variable name, value, and origin (system/global/group/command).
// The origins parameter should be obtained from executor.BuildProcessEnvironment.
func PrintFinalEnvironment(
	w io.Writer,
	envVars map[string]string,
	origins map[string]string,
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
		origin := origins[k]

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
