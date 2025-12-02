//go:build test

package security

import (
	"fmt"
)

// IsVariableValueSafe validates that a variable value contains no dangerous patterns
// This is a global convenience function that creates a default validator to check variable values
func IsVariableValueSafe(name, value string) error {
	validator, err := NewValidator(nil) // Use default config
	if err != nil {
		return fmt.Errorf("failed to create validator: %w", err)
	}
	return validator.ValidateEnvironmentValue(name, value)
}
