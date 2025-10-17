package config

import (
	"fmt"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
func validateEnvList(envList []string, context string) error {
	_, err := validateAndParseEnvList(envList, context)
	return err
}

// validateAndParseEnvList validates environment variable list and returns parsed map on success.
// This function parses KEY=VALUE format, checks for duplicates, and validates key names.
func validateAndParseEnvList(envList []string, context string) (map[string]string, error) {
	if len(envList) == 0 {
		return nil, nil
	}

	envMap := make(map[string]string)

	// Parse KEY=VALUE format, check for duplicates, and validate key names in a single loop
	for _, envVar := range envList {
		key, value, ok := common.ParseKeyValue(envVar)
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

	return envMap, nil
}

// validateVariableNameWithDetail validates a variable name and returns a detailed error
// if validation fails. This helper function standardizes error handling across
// ProcessEnv, ProcessFromEnv, and ProcessVars.
//
// The function performs two checks:
// 1. POSIX compliance using security.ValidateVariableName (empty name, pattern matching)
// 2. Reserved prefix check (names starting with "__runner_" are rejected)
//
// Parameters:
//   - varName: The variable name to validate
//   - level: The configuration level (e.g., "global", "group:mygroup", "cmd:mycmd")
//   - field: The field name where the variable appears (e.g., "env", "from_env", "vars")
//
// Returns:
//   - nil if valid
//   - *ErrReservedVariablePrefixDetail if the name uses a reserved prefix
//   - *ErrInvalidVariableNameDetail for POSIX validation errors
func validateVariableNameWithDetail(varName, level, field string) error {
	// First, check POSIX compliance using the existing security package function
	if err := security.ValidateVariableName(varName); err != nil {
		// POSIX validation error from security.ValidateVariableName
		return &ErrInvalidVariableNameDetail{
			Level:        level,
			Field:        field,
			VariableName: varName,
			Reason:       err.Error(),
		}
	}

	// Then, check for reserved prefix (additional check specific to internal variables)
	if strings.HasPrefix(varName, reservedVariablePrefix) {
		return &ErrReservedVariablePrefixDetail{
			Level:        level,
			Field:        field,
			VariableName: varName,
			Prefix:       reservedVariablePrefix,
		}
	}

	return nil
}
