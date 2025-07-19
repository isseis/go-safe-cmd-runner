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

// parseSystemEnvironment parses os.Environ() and filters variables based on the provided predicate
// predicate takes a single string argument (variable name) and returns true if the variable is allowed.
func (f *Filter) parseSystemEnvironment(predicate func(string) bool) map[string]string {
	result := make(map[string]string)

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", envSeparatorParts)
		if len(parts) != envSeparatorParts {
			continue
		}

		variable, value := parts[0], parts[1]
		if predicate == nil || predicate(variable) {
			result[variable] = value
		}
	}

	return result
}

// Source represents the source of global environment variables
// Used to differentiate between system variables and .env file variables
type Source string

const (
	// SourceSystem indicates variables from the system environment
	SourceSystem Source = "system"

	// SourceEnvFile indicates variables loaded from a .env file
	SourceEnvFile Source = "env_file"
)

// FilterSystemEnvironment filters system environment variables based on their names and values.
// It returns a map of filtered variables or an error if validation fails.
func (f *Filter) FilterSystemEnvironment() (map[string]string, error) {
	// Get all system environment variables
	sysEnv := f.parseSystemEnvironment(nil)
	return f.FilterGlobalVariables(sysEnv, SourceSystem)
}

// FilterGlobalVariables filters global environment variables based on their names and values.
// It returns a map of filtered variables or an error if validation fails.
func (f *Filter) FilterGlobalVariables(envFileVars map[string]string, src Source) (map[string]string, error) {
	result := make(map[string]string)

	for variable, value := range envFileVars {
		// Validate environment variable name and value
		if err := f.ValidateEnvironmentVariable(variable, value); err != nil {
			slog.Warn("Environment variable validation failed",
				"variable", variable,
				"source", src,
				"error", err)
			// Return security error for dangerous variable values
			if errors.Is(err, ErrDangerousVariableValue) {
				return nil, fmt.Errorf("%w: environment variable %s contains dangerous pattern", security.ErrUnsafeEnvironmentVar, variable)
			}
			continue
		}

		// Add variable to the result map after validation
		result[variable] = value
	}

	slog.Debug("Filtered global variables",
		"source", src,
		"total_vars", len(envFileVars),
		"filtered_vars", len(result))

	return result, nil
}

// ResolveGroupEnvironmentVars resolves environment variables for a specific group
// Security model:
// - System environment variables: trusted, only allowlist filtering applied
// - .env file variables: already validated during loading, only allowlist filtering applied
// - Group-defined variables: external configuration, full validation required
func (f *Filter) ResolveGroupEnvironmentVars(group *runnertypes.CommandGroup, loadedEnvVars map[string]string) (map[string]string, error) {
	if group == nil {
		return nil, fmt.Errorf("%w: group is nil", ErrGroupNotFound)
	}

	// Add system environment variables using the common parsing logic
	// Note: No validation needed - system environment variables are trusted
	result := f.parseSystemEnvironment(func(variable string) bool {
		return f.isVariableAllowed(variable, group.EnvAllowlist)
	})

	// Add loaded environment variables from .env file (already filtered in LoadEnvironment)
	// Note: These variables were already validated during the loading process
	// These override system variables
	for variable, value := range loadedEnvVars {
		if f.isVariableAllowed(variable, group.EnvAllowlist) {
			result[variable] = value
		}
	}

	// Add group-level environment variables (these override both system and .env vars)
	// Note: Full validation required as these come from external configuration
	for _, env := range group.Env {
		parts := strings.SplitN(env, "=", envSeparatorParts)
		if len(parts) != envSeparatorParts {
			continue
		}

		variable, value := parts[0], parts[1]

		// Validate environment variable name and value
		if err := f.ValidateEnvironmentVariable(variable, value); err != nil {
			slog.Warn("Group environment variable validation failed",
				"variable", variable,
				"group", group.Name,
				"error", err)
			continue
		}

		// Check if variable is allowed
		if f.isVariableAllowed(variable, group.EnvAllowlist) {
			result[variable] = value
		} else {
			slog.Warn("Group environment variable rejected by allowlist",
				"variable", variable,
				"group", group.Name)
		}
	}

	return result, nil
}

// IsVariableAccessAllowed checks if a variable can be accessed in the given group context
// This function expects a non-nil group parameter
func (f *Filter) IsVariableAccessAllowed(variable string, group *runnertypes.CommandGroup) bool {
	if group == nil {
		// This should not happen in normal operation, but handle it gracefully for safety
		slog.Error("IsVariableAccessAllowed called with nil group - this indicates a programming error")
		return false
	}

	allowed := f.isVariableAllowed(variable, group.EnvAllowlist)
	if !allowed {
		slog.Warn("Variable access denied",
			"variable", variable,
			"group", group.Name,
			"allowlist_size", len(group.EnvAllowlist))
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
