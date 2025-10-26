package debug

import (
	"fmt"
	"io"
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
)

// Global sensitive patterns instance for reuse
var defaultSensitivePatterns = redaction.DefaultSensitivePatterns()

// PrintFinalEnvironment prints the final environment variables that will be set for the process.
// It shows the variable name, value, and origin (system/global/group/command).
// The envMap parameter should be obtained from executor.BuildProcessEnvironment.
// If showSensitive is false, sensitive environment variables are masked with [REDACTED].
func PrintFinalEnvironment(
	w io.Writer,
	envMap map[string]executor.EnvVar,
	showSensitive bool,
) {
	_, _ = fmt.Fprintf(w, "===== Final Process Environment =====\n\n")

	if len(envMap) == 0 {
		_, _ = fmt.Fprintf(w, "No environment variables set.\n")
		return
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(envMap))
	for k := range envMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	_, _ = fmt.Fprintf(w, "Environment variables (%d):\n", len(envMap))
	for _, k := range keys {
		envVar := envMap[k]

		// Mask sensitive environment variables unless showSensitive is true
		displayValue := envVar.Value
		if !showSensitive && defaultSensitivePatterns.IsSensitiveEnvVar(k) {
			displayValue = "[REDACTED]"
		} else if len(displayValue) > MaxDisplayLength {
			// Truncate long values for readability (only if not masked)
			displayValue = displayValue[:MaxDisplayLength-EllipsisLength] + "..."
		}

		_, _ = fmt.Fprintf(w, "  %s=%s\n", k, displayValue)
		_, _ = fmt.Fprintf(w, "    (from %s)\n", envVar.Origin)
	}
	_, _ = fmt.Fprintln(w)
}
