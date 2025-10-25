package debug

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// PrintFromEnvInheritance prints the from_env inheritance state for a group.
// It shows whether the group inherits from global, overrides, or explicitly disables from_env.
func PrintFromEnvInheritance(
	w io.Writer,
	global *runnertypes.GlobalSpec,
	group *runnertypes.GroupSpec,
) {
	_, _ = fmt.Fprintf(w, "===== from_env Inheritance Analysis =====\n\n")

	// Print Global.env_import state
	_, _ = fmt.Fprintf(w, "[Global Level]\n")
	if len(global.EnvImport) > 0 {
		_, _ = fmt.Fprintf(w, "  env_import defined: %d mappings\n", len(global.EnvImport))
		for _, mapping := range global.EnvImport {
			_, _ = fmt.Fprintf(w, "    %s\n", mapping)
		}
		fromEnvVars := extractFromEnvVariables(global.EnvImport)
		_, _ = fmt.Fprintf(w, "  Internal variables created: %s\n", strings.Join(fromEnvVars, ", "))
	} else {
		_, _ = fmt.Fprintf(w, "  env_import: not defined\n")
	}
	_, _ = fmt.Fprintln(w)

	// Print Group.env_import inheritance state
	_, _ = fmt.Fprintf(w, "[Group: %s]\n", group.Name)

	if len(group.EnvImport) == 0 {
		// Inheritance: group.EnvImport is nil or empty (inherits from global)
		_, _ = fmt.Fprintf(w, "  env_import: Inheriting from Global\n")
		if len(global.EnvImport) > 0 {
			fromEnvVars := extractFromEnvVariables(global.EnvImport)
			_, _ = fmt.Fprintf(w, "  Inherited variables (%d): %s\n", len(fromEnvVars), strings.Join(fromEnvVars, ", "))
		} else {
			_, _ = fmt.Fprintf(w, "  (Global has no env_import defined, so nothing to inherit)\n")
		}
	} else {
		// Override: group.EnvImport is defined with values
		_, _ = fmt.Fprintf(w, "  env_import: Overriding Global configuration\n")
		_, _ = fmt.Fprintf(w, "  Group-specific mappings (%d):\n", len(group.EnvImport))
		for _, mapping := range group.EnvImport {
			_, _ = fmt.Fprintf(w, "    %s\n", mapping)
		}

		groupVars := extractFromEnvVariables(group.EnvImport)
		_, _ = fmt.Fprintf(w, "  Group variables: %s\n", strings.Join(groupVars, ", "))

		// Warn about Global variables that are no longer available
		if len(global.EnvImport) > 0 {
			globalVars := extractFromEnvVariables(global.EnvImport)
			unavailableVars := findUnavailableVars(globalVars, groupVars)
			if len(unavailableVars) > 0 {
				_, _ = fmt.Fprintf(w, "  Warning: Global variables (%s) are NOT available in this group\n",
					strings.Join(unavailableVars, ", "))
				_, _ = fmt.Fprintf(w, "  These variables will be undefined: ")
				for i, v := range unavailableVars {
					if i > 0 {
						_, _ = fmt.Fprintf(w, ", ")
					}
					_, _ = fmt.Fprintf(w, "%%{%s}", v)
				}
				_, _ = fmt.Fprintln(w)
			}
		}
	}
	_, _ = fmt.Fprintln(w)

	// Print allowlist inheritance state
	_, _ = fmt.Fprintf(w, "[Allowlist Inheritance]\n")
	if len(group.EnvAllowed) > 0 {
		_, _ = fmt.Fprintf(w, "  Group overrides Global allowlist\n")
		_, _ = fmt.Fprintf(w, "  Group allowlist (%d): %s\n",
			len(group.EnvAllowed), strings.Join(group.EnvAllowed, ", "))
		if len(global.EnvAllowed) > 0 {
			removedVars := findRemovedAllowlistVars(global.EnvAllowed, group.EnvAllowed)
			if len(removedVars) > 0 {
				_, _ = fmt.Fprintf(w, "  Removed from Global allowlist: %s\n", strings.Join(removedVars, ", "))
			}
		}
	} else {
		_, _ = fmt.Fprintf(w, "  Inheriting Global allowlist\n")
		if len(global.EnvAllowed) > 0 {
			_, _ = fmt.Fprintf(w, "  Allowlist (%d): %s\n",
				len(global.EnvAllowed), strings.Join(global.EnvAllowed, ", "))
		}
	}
	_, _ = fmt.Fprintln(w)
}

// extractFromEnvVariables extracts internal variable names from env_import mappings.
// env_import format: "internal_name=SYSTEM_VAR"
func extractFromEnvVariables(envImport []string) []string {
	const expectedParts = 2
	vars := make([]string, 0, len(envImport))
	for _, mapping := range envImport {
		parts := strings.SplitN(mapping, "=", expectedParts)
		if len(parts) == expectedParts {
			vars = append(vars, parts[0])
		}
	}
	sort.Strings(vars)
	return vars
}

// findUnavailableVars returns variables in globalVars that are not in groupVars.
func findUnavailableVars(globalVars, groupVars []string) []string {
	groupSet := make(map[string]bool)
	for _, v := range groupVars {
		groupSet[v] = true
	}

	unavailable := make([]string, 0)
	for _, v := range globalVars {
		if !groupSet[v] {
			unavailable = append(unavailable, v)
		}
	}
	sort.Strings(unavailable)
	return unavailable
}

// findRemovedAllowlistVars returns variables in globalAllowlist that are not in groupAllowlist.
func findRemovedAllowlistVars(globalAllowlist, groupAllowlist []string) []string {
	groupSet := make(map[string]bool)
	for _, v := range groupAllowlist {
		groupSet[v] = true
	}

	removed := make([]string, 0)
	for _, v := range globalAllowlist {
		if !groupSet[v] {
			removed = append(removed, v)
		}
	}
	sort.Strings(removed)
	return removed
}
