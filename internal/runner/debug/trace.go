// Package debug provides debugging and tracing utilities for variable expansion and configuration processing.
package debug

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// VariableExpansionTrace represents a single phase of variable expansion with detailed information.
type VariableExpansionTrace struct {
	Level          string            // "global", "group[name]", "command[name]"
	Phase          string            // "from_env", "vars", "env", "cmd", "args"
	Input          string            // Original input before expansion
	Output         string            // Result after expansion
	ReferencedVars []string          // Variables referenced during expansion
	ExpandedVars   map[string]string // All available variables at this phase
	Errors         []error           // Any errors encountered during expansion
}

// PrintTrace outputs the trace information in a human-readable format.
func (t *VariableExpansionTrace) PrintTrace(w io.Writer) {
	_, _ = fmt.Fprintf(w, "[%s - %s]\n", t.Level, t.Phase)

	if t.Input != "" {
		_, _ = fmt.Fprintf(w, "  Input:  %s\n", t.Input)
	}

	if t.Output != "" {
		_, _ = fmt.Fprintf(w, "  Output: %s\n", t.Output)
	}

	if len(t.ReferencedVars) > 0 {
		sort.Strings(t.ReferencedVars)
		_, _ = fmt.Fprintf(w, "  Referenced: %s\n", strings.Join(t.ReferencedVars, ", "))
	}

	if len(t.ExpandedVars) > 0 {
		keys := make([]string, 0, len(t.ExpandedVars))
		for k := range t.ExpandedVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		_, _ = fmt.Fprintf(w, "  Available variables (%d):\n", len(t.ExpandedVars))
		const maxValueLength = 50
		const ellipsis = 3
		for _, k := range keys {
			// Truncate long values for readability
			value := t.ExpandedVars[k]
			if len(value) > maxValueLength {
				value = value[:maxValueLength-ellipsis] + "..."
			}
			_, _ = fmt.Fprintf(w, "    %s = %s\n", k, value)
		}
	}

	if len(t.Errors) > 0 {
		_, _ = fmt.Fprintf(w, "  Errors:\n")
		for _, err := range t.Errors {
			_, _ = fmt.Fprintf(w, "    - %v\n", err)
		}
	}

	_, _ = fmt.Fprintln(w)
}
