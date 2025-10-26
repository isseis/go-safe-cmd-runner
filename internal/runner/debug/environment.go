package debug

import (
	"fmt"
	"io"
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
)

// Global sensitive patterns instance for reuse
var defaultSensitivePatterns = redaction.DefaultSensitivePatterns()

// PrintFinalEnvironment prints the final environment variables that will be set for the process.
// It shows the variable name, value, and origin (system/global/group/command).
// The origins parameter should be obtained from executor.BuildProcessEnvironment.
// If showSensitive is false, sensitive environment variables are masked with [REDACTED].
func PrintFinalEnvironment(
	w io.Writer,
	envVars map[string]string,
	origins map[string]string,
	showSensitive bool,
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

		// Mask sensitive environment variables unless showSensitive is true
		displayValue := value
		if !showSensitive && defaultSensitivePatterns.IsSensitiveEnvVar(k) {
			displayValue = "[REDACTED]"
		} else if len(displayValue) > MaxDisplayLength {
			// Truncate long values for readability (only if not masked)
			displayValue = displayValue[:MaxDisplayLength-EllipsisLength] + "..."
		}

		_, _ = fmt.Fprintf(w, "  %s=%s\n", k, displayValue)
		_, _ = fmt.Fprintf(w, "    (from %s)\n", origin)
	}
	_, _ = fmt.Fprintln(w)
}
