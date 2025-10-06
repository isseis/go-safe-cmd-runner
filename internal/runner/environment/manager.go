package environment

import (
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Manager manages environment variables for command execution
type Manager interface {
	// ValidateUserEnvNames validates that user-defined env variable names do not use reserved prefixes.
	ValidateUserEnvNames(userEnv map[string]string) error

	// BuildEnv builds the final environment for a command, merging auto-generated
	// and user-defined variables. The returned map includes all auto env variables
	// and can be used directly by VariableExpander.
	BuildEnv(userEnv map[string]string) (map[string]string, error)
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
	for name := range userEnv {
		if strings.HasPrefix(name, AutoEnvPrefix) {
			return runnertypes.NewReservedEnvPrefixError(name, AutoEnvPrefix)
		}
	}
	return nil
}

// BuildEnv builds the final environment map by merging auto-generated and user-defined variables
func (m *manager) BuildEnv(userEnv map[string]string) (map[string]string, error) {
	// Generate auto environment variables
	autoEnv := m.autoProvider.Generate()

	// Create result map with capacity for both auto and user env
	result := make(map[string]string, len(autoEnv)+len(userEnv))

	// Add auto-generated variables first
	for k, v := range autoEnv {
		result[k] = v
	}

	// Add user-defined variables
	// Note: No conflicts possible due to ValidateUserEnv being called earlier
	for k, v := range userEnv {
		result[k] = v
	}

	return result, nil
}
