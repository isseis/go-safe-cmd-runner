package config

import (
	"fmt"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/environment"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// reservedVariablePrefix is the prefix reserved for internal variables
const reservedVariablePrefix = "__runner_"

// validateEnvList validates a list of environment variables in KEY=VALUE format.
// The context parameter is used for error reporting (e.g., "global.env", "group.env:groupname").
// Returns an error if:
// - Any entry is not in KEY=VALUE format
// - Duplicate keys are found (case-sensitive comparison)
// - Variable names don't match the required pattern (via security.ValidateVariableName)
// - Variable names use reserved prefix "__RUNNER_" (via environment.ValidateUserEnvNames)
func validateEnvList(envList []string, context string) error {
	_, err := validateAndParseEnvList(envList, context)
	return err
}

// validateAndParseEnvList validates environment variable list and returns parsed map on success.
// This function parses KEY=VALUE format, checks for duplicates, validates key names,
// and checks for reserved prefixes in a single pass.
func validateAndParseEnvList(envList []string, context string) (map[string]string, error) {
	if len(envList) == 0 {
		return nil, nil
	}

	envMap := make(map[string]string)

	// Parse KEY=VALUE format, check for duplicates, and validate key names in a single loop
	for _, envVar := range envList {
		key, value, ok := common.ParseEnvVariable(envVar)
		if !ok {
			return nil, fmt.Errorf("%w: %q in %s", ErrMalformedEnvVariable, envVar, context)
		}

		// Check for duplicate key
		if firstValue, exists := envMap[key]; exists {
			return nil, fmt.Errorf("%w: %q in %s\n  First definition: %s=%s\n  Duplicate definition: %s=%s",
				ErrDuplicateEnvVariable, key, context, key, firstValue, key, value)
		}

		// Validate variable name using security.ValidateVariableName
		if err := security.ValidateVariableName(key); err != nil {
			return nil, fmt.Errorf("%w in %s: %w", ErrInvalidEnvKey, context, err)
		}

		envMap[key] = value
	}

	// Check for reserved prefix using environment.ValidateUserEnvNames
	if err := environment.ValidateUserEnvNames(envMap); err != nil {
		return nil, fmt.Errorf("%w in %s: %w", ErrReservedEnvPrefix, context, err)
	}

	return envMap, nil
}

// validateVariableName validates internal variable names for POSIX compliance and reserved prefix.
// This function wraps security.ValidateVariableName and adds reserved prefix checking.
// Returns an error if:
// - The name is empty (checked by security.ValidateVariableName)
// - The name does not match POSIX pattern (checked by security.ValidateVariableName)
// - The name starts with reserved prefix "__runner_" (checked here)
func validateVariableName(name string) error {
	// First, check POSIX compliance using the existing security package function
	if err := security.ValidateVariableName(name); err != nil {
		return err
	}

	// Then, check for reserved prefix (additional check specific to internal variables)
	if strings.HasPrefix(name, reservedVariablePrefix) {
		return fmt.Errorf("%w: '%s' (prefix '%s' is reserved for internal use)", ErrReservedVariablePrefix, name, reservedVariablePrefix)
	}

	return nil
}
