// Package debug provides debug information collection and formatting functionality
// for the dry-run mode. This package contains functions to format debug information
// as text output that matches the existing output format.
package debug

import (
	"fmt"
	"sort"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// formatStringSlice formats a slice of strings for display, similar to the existing functions.
// Returns "not defined" for empty slices and joins non-empty slices with ", ".
// The input slice order is preserved (not sorted) to match existing behavior.
func formatStringSlice(items []string, emptyMessage string) string {
	if len(items) == 0 {
		return emptyMessage
	}
	return strings.Join(items, ", ")
}

// formatGroupField formats a group field name for display output.
// It shows the field name with the specified count and format similar to the existing output.
func formatGroupField(fieldName string, count int) string {
	return fmt.Sprintf("  %s (%d):", fieldName, count)
}

// FormatInheritanceAnalysisText formats InheritanceAnalysis as text output matching
// the format of the existing PrintFromEnvInheritance function.
// Returns an empty string if analysis is nil.
// The groupName parameter specifies the name of the group to display in the output.
func FormatInheritanceAnalysisText(analysis *resource.InheritanceAnalysis, groupName string) string {
	if analysis == nil {
		return ""
	}

	var buf strings.Builder

	// Header
	buf.WriteString("===== from_env Inheritance Analysis =====\n\n")

	// Global Level section
	buf.WriteString("[Global Level]\n")
	if len(analysis.GlobalEnvImport) > 0 {
		buf.WriteString(fmt.Sprintf("  env_import defined: %d mappings\n", len(analysis.GlobalEnvImport)))
		for _, mapping := range analysis.GlobalEnvImport {
			buf.WriteString(fmt.Sprintf("    %s\n", mapping))
		}
		// Extract internal variable names
		fromEnvVars := extractInternalVarNames(analysis.GlobalEnvImport)
		buf.WriteString(fmt.Sprintf("  Internal variables created: %s\n", formatStringSlice(fromEnvVars, "")))
	} else {
		buf.WriteString("  env_import: not defined\n")
	}
	buf.WriteString("\n")

	// Group Level section
	buf.WriteString(fmt.Sprintf("[Group: %s]\n", groupName))

	if len(analysis.GroupEnvImport) == 0 {
		// Inheritance case
		buf.WriteString("  env_import: Inheriting from Global\n")
		if len(analysis.GlobalEnvImport) > 0 {
			inheritedVars := extractInternalVarNames(analysis.GlobalEnvImport)
			buf.WriteString(fmt.Sprintf("  Inherited variables (%d): %s\n",
				len(inheritedVars), formatStringSlice(inheritedVars, "")))
		} else {
			buf.WriteString("  (Global has no env_import defined, so nothing to inherit)\n")
		}
	} else {
		// Override case
		buf.WriteString("  env_import: Overriding Global configuration\n")
		buf.WriteString(fmt.Sprintf("  Group-specific mappings (%d):\n", len(analysis.GroupEnvImport)))
		for _, mapping := range analysis.GroupEnvImport {
			buf.WriteString(fmt.Sprintf("    %s\n", mapping))
		}

		groupVars := extractInternalVarNames(analysis.GroupEnvImport)
		buf.WriteString(fmt.Sprintf("  Group variables: %s\n", formatStringSlice(groupVars, "")))

		// Show unavailable variables if available (DetailLevelFull only)
		if len(analysis.UnavailableEnvImportVariables) > 0 {
			buf.WriteString(fmt.Sprintf("  Warning: Global variables (%s) are NOT available in this group\n",
				formatStringSlice(analysis.UnavailableEnvImportVariables, "")))
			buf.WriteString("  These variables will be undefined: ")
			for i, v := range analysis.UnavailableEnvImportVariables {
				if i > 0 {
					buf.WriteString(", ")
				}
				buf.WriteString(fmt.Sprintf("%%{%s}", v))
			}
			buf.WriteString("\n")
		}
	}
	buf.WriteString("\n")

	// Allowlist Inheritance section
	buf.WriteString("[Allowlist Inheritance]\n")
	switch analysis.InheritanceMode {
	case runnertypes.InheritanceModeInherit:
		buf.WriteString("  Inheriting Global env_allowlist\n")
		if len(analysis.GlobalAllowlist) > 0 {
			buf.WriteString(fmt.Sprintf("  Allowlist (%d): %s\n",
				len(analysis.GlobalAllowlist), formatStringSlice(analysis.GlobalAllowlist, "")))
		} else {
			buf.WriteString("  (Global has no env_allowlist defined, so all variables allowed)\n")
		}
	case runnertypes.InheritanceModeExplicit:
		buf.WriteString("  Using group-specific env_allowlist\n")
		buf.WriteString(fmt.Sprintf("  Group allowlist (%d): %s\n",
			len(analysis.GroupAllowlist), formatStringSlice(analysis.GroupAllowlist, "")))
		if len(analysis.RemovedAllowlistVariables) > 0 {
			buf.WriteString(fmt.Sprintf("  Removed from Global allowlist: %s\n",
				formatStringSlice(analysis.RemovedAllowlistVariables, "")))
		}
	case runnertypes.InheritanceModeReject:
		buf.WriteString("  Rejecting all environment variables\n")
		buf.WriteString("  (Group has empty env_allowlist defined, blocking all env inheritance)\n")
	default:
		buf.WriteString(fmt.Sprintf("  ERROR: Unknown inheritance mode: %v\n", analysis.InheritanceMode))
	}
	buf.WriteString("\n")

	return buf.String()
}

// FormatFinalEnvironmentText formats FinalEnvironment as text output matching
// the format of the existing PrintFinalEnvironment function.
// Returns an empty string if environment is nil.
func FormatFinalEnvironmentText(env *resource.FinalEnvironment) string {
	if env == nil {
		return ""
	}

	var buf strings.Builder

	// Header
	buf.WriteString("===== Final Process Environment =====\n\n")

	if len(env.Variables) == 0 {
		buf.WriteString("No environment variables set.\n")
		return buf.String()
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(env.Variables))
	for k := range env.Variables {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	buf.WriteString(fmt.Sprintf("Environment variables (%d):\n", len(env.Variables)))
	for _, k := range keys {
		envVar := env.Variables[k]

		// Determine display value - match existing PrintFinalEnvironment logic
		displayValue := envVar.Value
		if envVar.Masked || defaultSensitivePatterns.IsSensitiveEnvVar(k) {
			displayValue = "[REDACTED]"
		} else if len(displayValue) > MaxDisplayLength {
			// Truncate long values for readability (only if not masked)
			displayValue = displayValue[:MaxDisplayLength-EllipsisLength] + "..."
		}

		buf.WriteString(fmt.Sprintf("  %s=%s\n", k, displayValue))
		buf.WriteString(fmt.Sprintf("    (from %s)\n", envVar.Source))
	}
	buf.WriteString("\n")

	return buf.String()
}
