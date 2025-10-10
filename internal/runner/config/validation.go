package config

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// validateEnvList validates a list of environment variables in KEY=VALUE format.
// The context parameter is used for error reporting (e.g., "global.env", "group.env:groupname").
// Returns an error if:
// - Any entry is not in KEY=VALUE format
// - Duplicate keys are found (case-sensitive comparison)
// - Variable names don't match the required pattern (via security.ValidateVariableName)
// - Variable names use reserved prefix "__RUNNER_" (via environment.ValidateUserEnvNames)
func validateEnvList(envList []string, context string) error {
	if len(envList) == 0 {
		return nil
	}

	envMap := make(map[string]string)

	// Parse KEY=VALUE format and check for duplicates
	for _, envVar := range envList {
		key, value, ok := common.ParseEnvVariable(envVar)
		if !ok {
			return fmt.Errorf("%w: %q in %s", ErrMalformedEnvVariable, envVar, context)
		}

		// Check for duplicate key
		if firstValue, exists := envMap[key]; exists {
			return fmt.Errorf("%w: %q in %s\n  First definition: %s=%s\n  Duplicate definition: %s=%s",
				ErrDuplicateEnvVariable, key, context, key, firstValue, key, value)
		}

		envMap[key] = value
	}

	// Validate variable names using security.ValidateVariableName
	for key := range envMap {
		if err := security.ValidateVariableName(key); err != nil {
			return fmt.Errorf("%w in %s: %w", ErrInvalidEnvKey, context, err)
		}
	}

	// Check for reserved prefix using environment.ValidateUserEnvNames
	if err := environment.ValidateUserEnvNames(envMap); err != nil {
		return fmt.Errorf("in %s: %w", context, err)
	}

	return nil
}
