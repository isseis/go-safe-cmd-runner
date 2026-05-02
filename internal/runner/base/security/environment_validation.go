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

// ValidateEnvironmentValue validates that an environment variable value is safe.
// It rejects values that contain null bytes (\x00), newlines (\n), or carriage returns (\r),
// which can be used to inject headers or corrupt structured output. Shell meta-characters
// such as ; | $( > < are intentionally allowed because commands are executed directly
// (not via a shell), so these characters carry no injection risk.
func (v *Validator) ValidateEnvironmentValue(key, value string) error {
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%w: environment variable %s contains null byte",
			ErrUnsafeEnvironmentVar, key)
	}
	if strings.ContainsAny(value, "\n\r") {
		return fmt.Errorf("%w: environment variable %s contains newline or carriage return character",
			ErrUnsafeEnvironmentVar, key)
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
