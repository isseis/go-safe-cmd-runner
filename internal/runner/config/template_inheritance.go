// Package config provides template inheritance helper functions
// for merging and overriding configuration values from templates to commands.
package config

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
// Template entries are added first, then command entries.
// Duplicates are removed (first occurrence wins).
func MergeEnvImport(templateEnvImport []string, cmdEnvImport []string) []string {
	seen := make(map[string]struct{})
	result := []string{}

	// Add template entries first
	for _, item := range templateEnvImport {
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}

	// Add command entries
	for _, item := range cmdEnvImport {
		if _, exists := seen[item]; !exists {
			seen[item] = struct{}{}
			result = append(result, item)
		}
	}

	return result
}

// MergeVars merges variable definitions.
// Template variables are added first, then command variables overlay them.
// Command variables take precedence in case of key conflicts.
func MergeVars(templateVars map[string]any, cmdVars map[string]any) map[string]any {
	result := make(map[string]any, len(templateVars)+len(cmdVars))

	// Copy template vars
	for key, value := range templateVars {
		result[key] = value
	}

	// Overlay command vars (these take precedence)
	for key, value := range cmdVars {
		result[key] = value
	}

	return result
}
