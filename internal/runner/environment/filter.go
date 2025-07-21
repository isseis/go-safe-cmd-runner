// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	ErrVariableNotFound       = errors.New("variable reference not found")
	ErrVariableNotAllowed     = errors.New("variable not allowed by group allowlist")
	ErrMalformedEnvVariable   = errors.New("malformed environment variable")
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
		variable, value, ok := ParseEnvVariable(env)
		if !ok {
			continue
		}

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
// - .env file variables: validated during loading, only allowlist filtering applied
func (f *Filter) ResolveGroupEnvironmentVars(group *runnertypes.CommandGroup, loadedEnvVars map[string]string) (map[string]string, error) {
	if group == nil {
		return nil, fmt.Errorf("%w: group is nil", ErrGroupNotFound)
	}

	// Add system environment variables using the common parsing logic
	// Note: No validation needed - system environment variables are trusted
	result := f.parseSystemEnvironment(func(variable string) bool {
		return f.IsVariableAccessAllowed(variable, group)
	})

	// Add loaded environment variables from .env file (already filtered in LoadEnvironment)
	// Note: These variables were already validated during the loading process
	// These override system variables
	for variable, value := range loadedEnvVars {
		if f.IsVariableAccessAllowed(variable, group) {
			result[variable] = value
		}
	}

	return result, nil
}

// determineInheritanceMode determines the inheritance mode based on group configuration
func (f *Filter) determineInheritanceMode(group *runnertypes.CommandGroup) (runnertypes.InheritanceMode, error) {
	if group == nil {
		return 0, ErrGroupNotFound
	}

	// nil slice = inherit, empty slice = reject, non-empty = explicit
	if group.EnvAllowlist == nil {
		return runnertypes.InheritanceModeInherit, nil
	}

	if len(group.EnvAllowlist) == 0 {
		return runnertypes.InheritanceModeReject, nil
	}

	return runnertypes.InheritanceModeExplicit, nil
}

// resolveAllowlistConfiguration resolves the effective allowlist configuration for a group
func (f *Filter) resolveAllowlistConfiguration(group *runnertypes.CommandGroup) (*runnertypes.AllowlistResolution, error) {
	mode, err := f.determineInheritanceMode(group)
	if err != nil {
		return nil, fmt.Errorf("failed to determine inheritance mode: %w", err)
	}

	resolution := &runnertypes.AllowlistResolution{
		Mode:           mode,
		GroupAllowlist: group.EnvAllowlist,
		GroupName:      group.Name,
	}

	// Convert global allowlist map to slice for consistent interface
	globalList := make([]string, 0, len(f.globalAllowlist))
	for variable := range f.globalAllowlist {
		globalList = append(globalList, variable)
	}
	resolution.GlobalAllowlist = globalList

	// Set effective list based on mode
	switch mode {
	case runnertypes.InheritanceModeInherit:
		resolution.EffectiveList = resolution.GlobalAllowlist
	case runnertypes.InheritanceModeExplicit:
		resolution.EffectiveList = resolution.GroupAllowlist
	case runnertypes.InheritanceModeReject:
		resolution.EffectiveList = []string{} // Explicitly empty
	}

	// Log the resolution for debugging
	slog.Debug("Resolved allowlist configuration",
		"group", group.Name,
		"mode", mode.String(),
		"group_allowlist_size", len(group.EnvAllowlist),
		"global_allowlist_size", len(f.globalAllowlist),
		"effective_allowlist_size", len(resolution.EffectiveList))

	return resolution, nil
}

// resolveAllowedVariable checks if a variable is allowed based on the inheritance configuration
// This replaces the old isVariableAllowed function with clearer logic
func (f *Filter) resolveAllowedVariable(variable string, group *runnertypes.CommandGroup) (bool, error) {
	resolution, err := f.resolveAllowlistConfiguration(group)
	if err != nil {
		return false, fmt.Errorf("failed to resolve allowlist configuration: %w", err)
	}

	allowed := resolution.IsAllowed(variable)

	if !allowed {
		slog.Warn("Variable access denied",
			"variable", variable,
			"group", group.Name,
			"inheritance_mode", resolution.Mode.String(),
			"effective_allowlist_size", len(resolution.EffectiveList))
	} else {
		slog.Debug("Variable access granted",
			"variable", variable,
			"group", group.Name,
			"inheritance_mode", resolution.Mode.String())
	}

	return allowed, nil
}

// IsVariableAccessAllowed checks if a variable can be accessed in the given group context
// This function now uses the improved inheritance logic
func (f *Filter) IsVariableAccessAllowed(variable string, group *runnertypes.CommandGroup) bool {
	if group == nil {
		slog.Error("IsVariableAccessAllowed called with nil group - this indicates a programming error")
		return false
	}

	allowed, err := f.resolveAllowedVariable(variable, group)
	if err != nil {
		slog.Error("Failed to resolve variable allowlist",
			"variable", variable,
			"group", group.Name,
			"error", err)
		return false
	}

	return allowed
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
