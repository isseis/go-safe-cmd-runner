package environment

import (
	"errors"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Manager manages environment variables for command execution
type Manager interface {
	// ValidateUserEnvNames validates that user-defined env variable names do not use reserved prefixes.
	ValidateUserEnvNames(userEnv map[string]string) error

	// BuildEnv builds the environment with auto-generated variables.
	// The returned map includes all auto env variables and can be used directly by VariableExpander.
	BuildEnv() (map[string]string, error)
}

// manager implements Manager
type manager struct {
	autoProvider AutoEnvProvider
}

// NewManager creates a new Manager.
// If clock is nil, the default time.Now will be used internally.
func NewManager(clock Clock) Manager {
	return &manager{
		autoProvider: NewAutoEnvProvider(clock),
	}
}

// ValidateUserEnvNames validates that user-defined env variable names do not use the reserved prefix
func (m *manager) ValidateUserEnvNames(userEnv map[string]string) error {
	var errs []error
	for name := range userEnv {
		if strings.HasPrefix(name, AutoEnvPrefix) {
			errs = append(errs, runnertypes.NewReservedEnvPrefixError(name, AutoEnvPrefix))
		}
	}
	return errors.Join(errs...)
}

// BuildEnv builds the environment map with auto-generated variables only
func (m *manager) BuildEnv() (map[string]string, error) {
	// Generate auto environment variables
	autoEnv := m.autoProvider.Generate()

	// Create result map with auto env variables
	result := make(map[string]string, len(autoEnv))

	// Add auto-generated variables
	for k, v := range autoEnv {
		result[k] = v
	}

	return result, nil
}
