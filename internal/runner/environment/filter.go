// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
	ErrVariableNotAllowed     = errors.New("variable not allowed by allowlist")
	// Note: ErrMalformedEnvVariable is defined in config package as it's the primary user
)

// Filter provides environment variable filtering functionality with allowlist-based security
type Filter struct {
	// Map for O(1) lookups of allowed variables (guaranteed non-nil after construction
	// via NewFilter)
	globalAllowlist map[string]struct{}
}

// NewFilter creates a new environment variable filter with the provided global allowlist
// The function intentionally accepts only the global allowlist (slice of variable names)
// to keep the Filter small and focused; callers should pass cfg.Global.EnvAllowlist.
func NewFilter(allowList []string) *Filter {
	return &Filter{
		globalAllowlist: common.SliceToSet(allowList),
	}
}

// ParseSystemEnvironment parses os.Environ() and returns all environment variables as a map.
// No filtering is applied - use IsVariableAccessAllowed for filtering.
func (f *Filter) ParseSystemEnvironment() map[string]string {
	result := make(map[string]string)

	for _, env := range os.Environ() {
		variable, value, ok := common.ParseKeyValue(env)
		if !ok {
			continue
		}

		result[variable] = value
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
	sysEnv := f.ParseSystemEnvironment()
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

	// Get all system environment variables first
	sysEnv := f.ParseSystemEnvironment()

	// Filter system environment variables by allowlist
	// Note: Validation deferred to execution time - only variables actually used are validated
	result := make(map[string]string)
	for variable, value := range sysEnv {
		if f.IsVariableAccessAllowed(variable, group.EnvAllowlist, group.Name) {
			result[variable] = value
		}
	}

	// Add loaded environment variables from .env file (already filtered in LoadEnvironment)
	// Note: Validation deferred to execution time - only variables actually used are validated
	// These override system variables
	for variable, value := range loadedEnvVars {
		if f.IsVariableAccessAllowed(variable, group.EnvAllowlist, group.Name) {
			result[variable] = value
		}
	}

	return result, nil
}

// determineInheritanceMode determines the inheritance mode based on allowlist configuration
func (f *Filter) determineInheritanceMode(allowlist []string) runnertypes.InheritanceMode {
	// nil slice = inherit, empty slice = reject, non-empty = explicit
	if allowlist == nil {
		return runnertypes.InheritanceModeInherit
	}

	if len(allowlist) == 0 {
		return runnertypes.InheritanceModeReject
	}

	return runnertypes.InheritanceModeExplicit
}

// ResolveAllowlistConfiguration resolves the effective allowlist configuration for a group
func (f *Filter) ResolveAllowlistConfiguration(allowlist []string, groupName string) *runnertypes.AllowlistResolution {
	mode := f.determineInheritanceMode(allowlist)

	// Use Builder pattern with set-based API for efficiency
	// This avoids unnecessary map -> slice -> map conversions
	resolution := runnertypes.NewAllowlistResolutionBuilder().
		WithMode(mode).
		WithGroupName(groupName).
		WithGroupVariables(allowlist).
		WithGlobalVariablesSet(f.globalAllowlist).
		Build()

	// Log the resolution for debugging
	slog.Debug("Resolved allowlist configuration",
		"group", groupName,
		"mode", mode.String(),
		"group_allowlist_size", len(allowlist),
		"global_allowlist_size", len(f.globalAllowlist),
		"effective_allowlist_size", resolution.GetEffectiveSize())

	return resolution
}

// IsVariableAccessAllowed checks if a variable is allowed based on the inheritance configuration
// This replaces the old isVariableAllowed function with clearer logic
func (f *Filter) IsVariableAccessAllowed(variable string, allowlist []string, groupName string) bool {
	resolution := f.ResolveAllowlistConfiguration(allowlist, groupName)
	allowed := resolution.IsAllowed(variable)

	if !allowed {
		slog.Debug("Variable access denied",
			"variable", variable,
			"group", groupName,
			"inheritance_mode", resolution.Mode.String(),
			"effective_allowlist_size", resolution.GetEffectiveSize())
	} else {
		slog.Debug("Variable access granted",
			"variable", variable,
			"group", groupName,
			"inheritance_mode", resolution.Mode.String())
	}

	return allowed
}

// ValidateEnvironmentVariable validates both name and value of an environment variable
func (f *Filter) ValidateEnvironmentVariable(name, value string) error {
	if err := security.ValidateVariableName(name); err != nil {
		return fmt.Errorf("invalid name for variable %q: %w", name, err)
	}

	if err := security.IsVariableValueSafe(name, value); err != nil {
		return fmt.Errorf("unsafe value for variable %q: %w", name, err)
	}

	return nil
}
