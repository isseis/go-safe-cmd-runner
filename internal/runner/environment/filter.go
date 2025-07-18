// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
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
	config            *runnertypes.Config
	globalAllowlist   map[string]bool // Map for O(1) lookups of allowed variables (always non-nil)
	dangerousPatterns []string        // Pre-compiled list of dangerous patterns
}

// NewFilter creates a new environment variable filter with the provided configuration
func NewFilter(config *runnertypes.Config) *Filter {
	f := &Filter{
		config:          config,
		globalAllowlist: make(map[string]bool), // Initialize with empty map
		dangerousPatterns: []string{
			// Command injection patterns
			";", "&&", "||", "|", "$(", "`",
			// Redirection patterns (more specific to avoid false positives, e.g. HTML tags)
			">", "<",
			// Destructive file system operations
			"rm ", "del ", "format ", "mkfs ", "mkfs.",
			"dd if=", "dd of=",
			// Code execution patterns
			"exec ", "exec(", "system ", "system(", "eval ", "eval(",
		},
	}

	// Initialize the allowlist map with global allowlist if it exists
	for _, v := range config.Global.EnvAllowlist {
		f.globalAllowlist[v] = true
	}

	return f
}

// FilterSystemEnvironment filters system environment variables based on the provided allowlist
func (f *Filter) FilterSystemEnvironment(groupAllowlist []string) (map[string]string, error) {
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
		if f.isVariableAllowed(key, groupAllowlist) {
			result[key] = value
		}
	}

	slog.Debug("Filtered system environment variables",
		"total_vars", len(os.Environ()),
		"filtered_vars", len(result),
		"allowlist_size", len(groupAllowlist)+len(f.globalAllowlist))

	return result, nil
}

// FilterEnvFileVariables filters environment variables from .env file based on allowlist
func (f *Filter) FilterEnvFileVariables(envFileVars map[string]string, groupAllowlist []string) (map[string]string, error) {
	result := make(map[string]string)

	for key, value := range envFileVars {
		// Validate environment variable name and value
		if err := f.ValidateEnvironmentVariable(key, value); err != nil {
			slog.Warn("Environment variable from .env file validation failed",
				"variable", key,
				"source", "env_file",
				"error", err)
			// Return security error for dangerous variable values
			if errors.Is(err, ErrDangerousVariableValue) {
				return nil, fmt.Errorf("%w: environment variable %s contains dangerous pattern", security.ErrUnsafeEnvironmentVar, key)
			}
			continue
		}

		// Check if variable is in allowlist
		if f.isVariableAllowed(key, groupAllowlist) {
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
		"allowlist_size", len(groupAllowlist)+len(f.globalAllowlist))

	return result, nil
}

// BuildAllowedVariableMaps builds allowlist maps for each group
// If a group has env_allowlist defined, it overrides global settings
func (f *Filter) BuildAllowedVariableMaps() map[string][]string {
	result := make(map[string][]string)

	// Global allowlist as fallback
	globalAllowlist := f.config.Global.EnvAllowlist

	// Process each group
	for _, group := range f.config.Groups {
		// If group has env_allowlist defined (including empty slice), use it exclusively
		// Otherwise, use global allowlist
		if group.EnvAllowlist != nil {
			result[group.Name] = group.EnvAllowlist
		} else {
			result[group.Name] = globalAllowlist
		}
	}

	return result
}

// ResolveGroupEnvironmentVars resolves environment variables for a specific group
func (f *Filter) ResolveGroupEnvironmentVars(group *runnertypes.CommandGroup, loadedEnvVars map[string]string) (map[string]string, error) {
	if group == nil {
		return nil, fmt.Errorf("%w: group is nil", ErrGroupNotFound)
	}

	// Filter system environment variables
	filteredSystemEnv, err := f.FilterSystemEnvironment(group.EnvAllowlist)
	if err != nil {
		return nil, fmt.Errorf("failed to filter system environment: %w", err)
	}

	// Start with filtered system environment variables
	result := make(map[string]string)
	maps.Copy(result, filteredSystemEnv)

	// Add loaded environment variables from .env file (already filtered in LoadEnvironment)
	// These override system variables
	for k, v := range loadedEnvVars {
		if f.isVariableAllowed(k, group.EnvAllowlist) {
			result[k] = v
		}
	}

	// Add group-level environment variables (these override both system and .env vars)
	for _, env := range group.Env {
		parts := strings.SplitN(env, "=", envSeparatorParts)
		if len(parts) != envSeparatorParts {
			continue
		}

		key := parts[0]
		value := parts[1]

		// Validate environment variable name and value
		if err := f.ValidateEnvironmentVariable(key, value); err != nil {
			slog.Warn("Group environment variable validation failed",
				"variable", key,
				"group", group.Name,
				"error", err)
			continue
		}

		// Check if variable is allowed
		if f.isVariableAllowed(key, group.EnvAllowlist) {
			result[key] = value
		} else {
			slog.Warn("Group environment variable rejected by allowlist",
				"variable", key,
				"group", group.Name)
		}
	}

	return result, nil
}

// IsVariableAccessAllowed checks if a variable can be accessed in the given group context
// If no group is provided, it checks against the global allowlist only
func (f *Filter) IsVariableAccessAllowed(variable string, group *runnertypes.CommandGroup) bool {
	// If no group is provided, check against global allowlist only
	if group == nil {
		allowed := f.isVariableAllowed(variable, nil)
		if !allowed {
			slog.Warn("Variable access denied by global allowlist", "variable", variable, "allowlist_size", len(f.config.Global.EnvAllowlist))
		}
		return allowed
	}

	allowed := f.isVariableAllowed(variable, group.EnvAllowlist)
	if !allowed {
		slog.Warn("Variable access denied",
			"variable", variable,
			"group", group.Name,
			"allowlist_size", len(group.EnvAllowlist)+len(f.globalAllowlist))
	}

	return allowed
}

// isVariableAllowed checks if a variable is in the allowlist
// If groupAllowlist is provided (non-nil), it takes precedence over global allowlist
func (f *Filter) isVariableAllowed(variable string, groupAllowlist []string) bool {
	// If group allowlist is provided, use it exclusively (ignore global)
	if groupAllowlist != nil {
		return slices.Contains(groupAllowlist, variable)
	}

	// If no group allowlist provided, use global allowlist
	return f.globalAllowlist[variable]
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
	// Check for potentially dangerous patterns using pre-compiled list
	for _, pattern := range f.dangerousPatterns {
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
func (f *Filter) LogVariableAccess(variable string, group *runnertypes.CommandGroup, allowed bool) {
	groupName := ""
	if group != nil {
		groupName = group.Name
	}

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
