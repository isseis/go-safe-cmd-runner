package security

import (
	"fmt"
	"strings"
)

// SanitizeEnvironmentVariables removes or sanitizes sensitive environment variables
func (v *Validator) SanitizeEnvironmentVariables(envVars map[string]string) map[string]string {
	if envVars == nil {
		return make(map[string]string)
	}

	sanitized := make(map[string]string)

	for key, value := range envVars {
		if v.isSensitiveEnvVar(key) {
			// Replace sensitive values with a placeholder
			sanitized[key] = "[REDACTED]"
		} else {
			sanitized[key] = value
		}
	}

	return sanitized
}

// isSensitiveEnvVar checks if an environment variable name matches sensitive patterns
func (v *Validator) isSensitiveEnvVar(name string) bool {
	// Use the new common functionality first
	if v.sensitivePatterns.IsSensitiveEnvVar(name) {
		return true
	}

	// Fallback to the legacy regex patterns for backward compatibility
	upperName := strings.ToUpper(name)
	for _, re := range v.sensitiveEnvRegexps {
		if re.MatchString(upperName) {
			return true
		}
	}

	return false
}

// ValidateEnvironmentValue validates that an environment variable value is safe
func (v *Validator) ValidateEnvironmentValue(key, value string) error {
	// Check for potential command injection patterns using compiled regexes
	for _, re := range v.dangerousEnvRegexps {
		if re.MatchString(value) {
			return fmt.Errorf("%w: environment variable %s contains potentially dangerous pattern: %s",
				ErrUnsafeEnvironmentVar, key, re.String())
		}
	}

	return nil
}

// ValidateAllEnvironmentVars validates all environment variables for safety
func (v *Validator) ValidateAllEnvironmentVars(envVars map[string]string) error {
	for key, value := range envVars {
		if err := v.ValidateEnvironmentValue(key, value); err != nil {
			return err
		}
	}
	return nil
}

// ValidateVariableValue validates that a variable value contains no dangerous patterns
// This is a convenience function that wraps ValidateEnvironmentValue for use by other packages
func (v *Validator) ValidateVariableValue(value string) error {
	// Use a dummy key name for the validation since we only care about the value
	return v.ValidateEnvironmentValue("VAR", value)
}

// ValidateVariableName validates that a variable name is safe and well-formed
// This is a global convenience function for validating environment variable names
func ValidateVariableName(name string) error {
	if name == "" {
		return ErrVariableNameEmpty
	}

	// Check first character - must be a letter or underscore
	firstChar := name[0]
	if !isLetterOrUnderscore(firstChar) {
		return ErrVariableNameInvalidStart
	}

	// Check remaining characters - must be letter, digit, or underscore
	for i := 1; i < len(name); i++ {
		char := name[i]
		if !isLetterOrUnderscoreOrDigit(char) {
			return fmt.Errorf("%w: '%c'", ErrVariableNameInvalidChar, char)
		}
	}

	return nil
}

// isLetterOrUnderscore checks if a byte is a letter (A-Z, a-z) or underscore
func isLetterOrUnderscore(char byte) bool {
	return (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_'
}

// isLetterOrUnderscoreOrDigit checks if a byte is a letter, digit, or underscore
func isLetterOrUnderscoreOrDigit(char byte) bool {
	return isLetterOrUnderscore(char) || (char >= '0' && char <= '9')
}

// IsVariableValueSafe validates that a variable value contains no dangerous patterns
// This is a global convenience function that creates a default validator to check variable values
func IsVariableValueSafe(value string) error {
	validator, err := NewValidator(nil) // Use default config
	if err != nil {
		return fmt.Errorf("failed to create validator: %w", err)
	}
	return validator.ValidateVariableValue(value)
}
