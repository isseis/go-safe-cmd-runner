package config

import (
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// reservedVariablePrefix is the prefix reserved for internal variables
const reservedVariablePrefix = "__runner_"

// validateVariableName validates a variable name and returns a detailed error
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
func validateVariableName(varName, level, field string) error {
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
