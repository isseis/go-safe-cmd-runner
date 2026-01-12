// Package config provides template inheritance helper functions
// for merging and overriding configuration values from templates to commands.
package config

import (
	"maps"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// OverrideStringPointer applies the override model for *string fields.
// If cmdValue is nil, it returns templateValue (inheritance).
// If cmdValue is non-nil (including empty string), it returns cmdValue (override).
func OverrideStringPointer(cmdValue *string, templateValue *string) *string {
	if cmdValue == nil {
		return templateValue
	}
	return cmdValue
}

// MergeEnvImport merges environment import lists.
// For entries in "internal_name=SYSTEM_VAR" format, duplicates are detected
// by internal_name. Command entries override template entries with the same
// internal_name (similar to MergeVars behavior).
// For invalid format entries, duplicates are detected by the full string.
func MergeEnvImport(templateEnvImport []string, cmdEnvImport []string) []string {
	capacity := len(templateEnvImport) + len(cmdEnvImport)
	// Map from internal name to the mapping string
	mappings := make(map[string]string, capacity)
	// Track insertion order for deterministic output
	order := make([]string, 0, capacity)

	// Add template entries first
	for _, mapping := range templateEnvImport {
		internalName, _, ok := common.ParseKeyValue(mapping)
		if !ok {
			// Invalid format: treat whole string as key (for backward compatibility)
			internalName = mapping
		}

		if _, exists := mappings[internalName]; !exists {
			mappings[internalName] = mapping
			order = append(order, internalName)
		}
	}

	// Add command entries (these override template entries with same internal name)
	for _, mapping := range cmdEnvImport {
		internalName, _, ok := common.ParseKeyValue(mapping)
		if !ok {
			// Invalid format: treat whole string as key (for backward compatibility)
			internalName = mapping
		}

		if _, exists := mappings[internalName]; exists {
			// Override template entry
			mappings[internalName] = mapping
		} else {
			// New entry
			mappings[internalName] = mapping
			order = append(order, internalName)
		}
	}

	// Build result maintaining insertion order
	result := make([]string, 0, len(order))
	for _, internalName := range order {
		result = append(result, mappings[internalName])
	}

	return result
}

// MergeVars merges variable definitions.
// Template variables are added first, then command variables overlay them.
// Command variables take precedence in case of key conflicts.
func MergeVars(templateVars map[string]any, cmdVars map[string]any) map[string]any {
	result := make(map[string]any, len(templateVars)+len(cmdVars))

	// Copy template vars
	maps.Copy(result, templateVars)

	// Overlay command vars (these take precedence)
	maps.Copy(result, cmdVars)

	return result
}
