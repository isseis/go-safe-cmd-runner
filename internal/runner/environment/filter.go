// Package environment provides environment variable filtering and management functionality
// for secure command execution with allowlist-based access control.
package environment

import (
	"errors"
	"log/slog"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
// to keep the Filter small and focused; callers should pass cfg.Global.EnvAllowed.
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
