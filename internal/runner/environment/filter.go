// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

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

// FilterSystemEnvironment filters system environment variables based on their names.
// Validation is deferred to command execution time to validate only variables actually used.
// It returns a map of filtered variables.
func (f *Filter) FilterSystemEnvironment() (map[string]string, error) {
	// Get all system environment variables (validation deferred to execution time)
	sysEnv := f.parseSystemEnvironment(nil)
	return f.FilterGlobalVariables(sysEnv, SourceSystem)
}

// FilterGlobalVariables filters global environment variables based on their names.
// Validation is deferred to command execution time to validate only variables actually used.
// It returns a map of filtered variables.
func (f *Filter) FilterGlobalVariables(envFileVars map[string]string, src Source) (map[string]string, error) {
	result := make(map[string]string)

	for variable, value := range envFileVars {
		// Basic variable name validation (empty name check)
		if variable == "" {
			slog.Warn("Environment variable has empty name",
				"source", src)
			continue
		}

		// Add variable to the result map (validation deferred to execution time)
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
// - System environment variables: filtered by allowlist, validated at execution time
// - .env file variables: filtered by allowlist, validated at execution time
func (f *Filter) ResolveGroupEnvironmentVars(group *runnertypes.CommandGroup, loadedEnvVars map[string]string) (map[string]string, error) {
	if group == nil {
		return nil, fmt.Errorf("%w: group is nil", ErrGroupNotFound)
	}

	// Add system environment variables using the common parsing logic
	// Note: Validation deferred to execution time - only variables actually used are validated
	result := f.parseSystemEnvironment(func(variable string) bool {
		return f.IsVariableAccessAllowed(variable, group)
	})

	// Add loaded environment variables from .env file (already filtered in LoadEnvironment)
	// Note: Validation deferred to execution time - only variables actually used are validated
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
		slog.Debug("Variable access denied",
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

// ValidateEnvironmentVariable validates both name and value of an environment variable
func (f *Filter) ValidateEnvironmentVariable(name, value string) error {
	if !ValidateVariableName(name) {
		if name == "" {
			return ErrVariableNameEmpty
		}
		return fmt.Errorf("%w: %s", ErrInvalidVariableName, name)
	}

	if !ValidateVariableValue(value) {
		return fmt.Errorf("%w: %s", security.ErrUnsafeEnvironmentVar, value)
	}

	return nil
}
