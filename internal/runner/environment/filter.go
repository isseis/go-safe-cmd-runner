// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Error definitions
var (
	ErrGroupNotFound          = errors.New("group not found")
	ErrVariableNameEmpty      = errors.New("variable name cannot be empty")
	ErrInvalidVariableName    = errors.New("invalid variable name")
	ErrDangerousVariableValue = errors.New("variable value contains dangerous pattern")
)

// Constants
const (
	envSeparatorParts = 2
)

// Filter provides environment variable filtering functionality with allowlist-based security
type Filter struct {
	config          *runnertypes.Config
	globalAllowlist map[string]bool // Map for O(1) lookups of allowed variables (always non-nil)
}

// NewFilter creates a new environment variable filter with the provided configuration
func NewFilter(config *runnertypes.Config) *Filter {
	f := &Filter{
		config:          config,
		globalAllowlist: make(map[string]bool), // Initialize with empty map
	}

	// Initialize the allowlist map with global allowlist if it exists
	// Populate with new values
	for _, v := range config.Global.EnvAllowlist {
		f.globalAllowlist[v] = true
	}

	return f
}

// FilterSystemEnvironment filters system environment variables based on the provided allowlist
func (f *Filter) FilterSystemEnvironment(allowlist []string) (map[string]string, error) {
	result := make(map[string]string)

	// Get system environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", envSeparatorParts)
		if len(parts) != envSeparatorParts {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Check if variable is in allowlist
		if f.isVariableAllowed(key, allowlist) {
			result[key] = value
		}
	}

	slog.Debug("Filtered system environment variables",
		"total_vars", len(os.Environ()),
		"filtered_vars", len(result),
		"allowlist_size", len(allowlist))

	return result, nil
}

// FilterEnvFileVariables filters environment variables from .env file based on allowlist
func (f *Filter) FilterEnvFileVariables(envFileVars map[string]string, allowlist []string) (map[string]string, error) {
	result := make(map[string]string)

	for key, value := range envFileVars {
		// Check if variable is in allowlist
		if f.isVariableAllowed(key, allowlist) {
			result[key] = value
		} else {
			slog.Warn("Environment variable from .env file rejected by allowlist",
				"variable", key,
				"source", "env_file")
		}
	}

	slog.Debug("Filtered .env file variables",
		"total_vars", len(envFileVars),
		"filtered_vars", len(result),
		"allowlist_size", len(allowlist))

	return result, nil
}

// BuildAllowedVariableMaps builds combined allowlist from global and group-level configurations
func (f *Filter) BuildAllowedVariableMaps() map[string][]string {
	result := make(map[string][]string)

	// Start with global allowlist
	globalAllowlist := f.config.Global.EnvAllowlist

	// Add group-level allowlists
	for _, group := range f.config.Groups {
		// Combine global and group-level allowlists
		combinedAllowlist := make([]string, 0, len(globalAllowlist)+len(group.EnvAllowlist))
		combinedAllowlist = append(combinedAllowlist, globalAllowlist...)
		combinedAllowlist = append(combinedAllowlist, group.EnvAllowlist...)

		result[group.Name] = combinedAllowlist
	}

	return result
}

// ResolveGroupEnvironmentVars resolves environment variables for a specific group
func (f *Filter) ResolveGroupEnvironmentVars(groupName string, loadedEnvVars map[string]string) (map[string]string, error) {
	// Find the group
	var group *runnertypes.CommandGroup
	for _, g := range f.config.Groups {
		if g.Name == groupName {
			group = &g
			break
		}
	}

	if group == nil {
		return nil, fmt.Errorf("%w: %s", ErrGroupNotFound, groupName)
	}

	// Build combined allowlist
	allowlist := make([]string, 0, len(f.config.Global.EnvAllowlist)+len(group.EnvAllowlist))
	allowlist = append(allowlist, f.config.Global.EnvAllowlist...)
	allowlist = append(allowlist, group.EnvAllowlist...)

	// Filter system environment variables
	filteredSystemEnv, err := f.FilterSystemEnvironment(allowlist)
	if err != nil {
		return nil, fmt.Errorf("failed to filter system environment: %w", err)
	}

	// Start with filtered system environment variables
	result := make(map[string]string)
	for k, v := range filteredSystemEnv {
		result[k] = v
	}

	// Add loaded environment variables from .env file (already filtered in LoadEnvironment)
	// These override system variables
	for k, v := range loadedEnvVars {
		if f.isVariableAllowed(k, allowlist) {
			result[k] = v
		}
	}

	// Add group-level environment variables (these override both system and .env vars)
	for _, env := range group.Env {
		parts := strings.SplitN(env, "=", envSeparatorParts)
		if len(parts) == envSeparatorParts {
			key := parts[0]
			value := parts[1]

			// Check if variable is allowed
			if f.isVariableAllowed(key, allowlist) {
				result[key] = value
			} else {
				slog.Warn("Group environment variable rejected by allowlist",
					"variable", key,
					"group", groupName)
			}
		}
	}

	return result, nil
}

// IsVariableAccessAllowed checks if a variable can be accessed in the given group context
func (f *Filter) IsVariableAccessAllowed(variable string, groupName string) bool {
	// If no group name is provided, check against global allowlist only
	if groupName == "" {
		return f.IsGlobalVariableAllowed(variable)
	}

	// Find the group
	var group *runnertypes.CommandGroup
	for _, g := range f.config.Groups {
		if g.Name == groupName {
			group = &g
			break
		}
	}

	if group == nil {
		slog.Warn("Group not found for variable access check",
			"variable", variable,
			"group", groupName)
		return false
	}

	// Build combined allowlist
	allowlist := make([]string, 0, len(f.config.Global.EnvAllowlist)+len(group.EnvAllowlist))
	allowlist = append(allowlist, f.config.Global.EnvAllowlist...)
	allowlist = append(allowlist, group.EnvAllowlist...)

	allowed := f.isVariableAllowed(variable, allowlist)

	if !allowed {
		slog.Warn("Variable access denied",
			"variable", variable,
			"group", groupName,
			"allowlist_size", len(allowlist))
	}

	return allowed
}

// IsGlobalVariableAllowed checks if a variable is allowed by global allowlist only
func (f *Filter) IsGlobalVariableAllowed(variable string) bool {
	allowed := f.isVariableAllowed(variable, f.config.Global.EnvAllowlist)

	if !allowed {
		slog.Warn("Variable access denied by global allowlist",
			"variable", variable,
			"allowlist_size", len(f.config.Global.EnvAllowlist))
	}

	return allowed
}

// isVariableAllowed checks if a variable is in the allowlist
// It first checks the global allowlist map for O(1) lookups
// If not found in the global map, it falls back to checking the provided allowlist slice
func (f *Filter) isVariableAllowed(variable string, allowlist []string) bool {
	// If allowlist is empty, nothing is allowed
	if len(allowlist) == 0 {
		return false
	}

	// Check the global map first for O(1) lookup
	if f.globalAllowlist[variable] {
		return true
	}

	// If not found in global map, check the provided allowlist
	return slices.Contains(allowlist, variable)
}

// ValidateVariableName validates that a variable name is safe and well-formed
func (f *Filter) ValidateVariableName(name string) error {
	if name == "" {
		return ErrVariableNameEmpty
	}

	// Check for invalid characters
	for i, char := range name {
		if i == 0 {
			// First character must be letter or underscore
			if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && char != '_' {
				return fmt.Errorf("%w: %s (must start with letter or underscore)", ErrInvalidVariableName, name)
			}
		} else {
			// Subsequent characters can be letters, digits, or underscores
			if (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
				return fmt.Errorf("%w: %s (contains invalid character)", ErrInvalidVariableName, name)
			}
		}
	}

	return nil
}

// ValidateVariableValue validates that a variable value is safe
func (f *Filter) ValidateVariableValue(value string) error {
	// Check for potentially dangerous patterns
	dangerousPatterns := []string{
		";", "&&", "||", "|", "$(",
		"`", "$(", "${", ">/", "<",
		"rm ", "del ", "format ", "mkfs.",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(value, pattern) {
			return fmt.Errorf("%w: %s", ErrDangerousVariableValue, pattern)
		}
	}

	return nil
}

// ValidateEnvironmentVariable validates both name and value of an environment variable
func (f *Filter) ValidateEnvironmentVariable(name, value string) error {
	if err := f.ValidateVariableName(name); err != nil {
		return err
	}

	if err := f.ValidateVariableValue(value); err != nil {
		return err
	}

	return nil
}

// ContainsSensitiveData checks if a variable name or value contains sensitive information
func (f *Filter) ContainsSensitiveData(name, value string) bool {
	sensitivePatterns := []string{
		"password", "passwd", "secret", "token", "key", "auth",
		"credential", "private", "secure", "hidden", "confidential",
	}

	lowerName := strings.ToLower(name)
	lowerValue := strings.ToLower(value)

	for _, pattern := range sensitivePatterns {
		if strings.Contains(lowerName, pattern) || strings.Contains(lowerValue, pattern) {
			return true
		}
	}

	return false
}

// LogEnvironmentFiltering logs environment variable filtering actions
func (f *Filter) LogEnvironmentFiltering(source string, totalVars, filteredVars int, allowlistSize int) {
	slog.Info("Environment variable filtering completed",
		"source", source,
		"total_variables", totalVars,
		"filtered_variables", filteredVars,
		"allowlist_size", allowlistSize,
		"rejection_count", totalVars-filteredVars)
}

// LogVariableAccess logs variable access attempts for auditing
func (f *Filter) LogVariableAccess(variable string, groupName string, allowed bool) {
	if allowed {
		slog.Debug("Variable access granted",
			"variable", variable,
			"group", groupName)
	} else {
		slog.Warn("Variable access denied",
			"variable", variable,
			"group", groupName)
	}
}

// GetVariableNames extracts variable names from a list of environment variable strings
func (f *Filter) GetVariableNames(envVars []string) []string {
	names := make([]string, 0, len(envVars))

	for _, env := range envVars {
		parts := strings.SplitN(env, "=", envSeparatorParts)
		if len(parts) >= 1 {
			names = append(names, parts[0])
		}
	}

	return names
}
