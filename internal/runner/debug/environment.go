package debug

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
)

// Global sensitive patterns instance for reuse
var defaultSensitivePatterns = redaction.DefaultSensitivePatterns()

// escapeControlChars escapes control characters in a string for safe display.
// This ensures terminal control characters don't corrupt the output.
//
// Uses unicode.IsControl to detect control characters, then escapes them using
// standard escape sequences (\n, \t, etc.) for common ones, or \xNN for others.
// Regular printable characters are left unchanged for readability.
func escapeControlChars(s string) string {
	var result strings.Builder
	for _, r := range s {
		if !unicode.IsControl(r) {
			// Not a control character - output as-is
			result.WriteRune(r)
			continue
		}

		// Use standard escape sequences for common control characters
		switch r {
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\b':
			result.WriteString("\\b")
		case '\f':
			result.WriteString("\\f")
		case '\v':
			result.WriteString("\\v")
		case '\a':
			result.WriteString("\\a")
		default:
			// For other control characters, use \xNN notation
			fmt.Fprintf(&result, "\\x%02x", r)
		}
	}
	return result.String()
}

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
		} else {
			// Escape control characters for safe display (preserve full value for dry-run verification)
			displayValue = escapeControlChars(displayValue)
		}

		_, _ = fmt.Fprintf(w, "  %s=%s\n", k, displayValue)
		_, _ = fmt.Fprintf(w, "    (from %s)\n", envVar.Origin)
	}
	_, _ = fmt.Fprintln(w)
}
