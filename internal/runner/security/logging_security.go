package security

// SanitizeErrorForLogging sanitizes an error message for safe logging
func (v *Validator) SanitizeErrorForLogging(err error) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// If error details should not be included, return a generic message
	if !v.config.LoggingOptions.IncludeErrorDetails {
		return "[error details redacted for security]"
	}

	// Redact sensitive information if enabled
	if v.config.LoggingOptions.RedactSensitiveInfo {
		errMsg = v.redactSensitivePatterns(errMsg)
	}

	// Truncate if too long
	if v.config.LoggingOptions.MaxErrorMessageLength > 0 && len(errMsg) > v.config.LoggingOptions.MaxErrorMessageLength {
		errMsg = errMsg[:v.config.LoggingOptions.MaxErrorMessageLength] + "...[truncated]"
	}

	return errMsg
}

// SanitizeOutputForLogging sanitizes command output for safe logging
func (v *Validator) SanitizeOutputForLogging(output string) string {
	if output == "" {
		return ""
	}

	// Redact sensitive information if enabled
	if v.config.LoggingOptions.RedactSensitiveInfo {
		output = v.redactSensitivePatterns(output)
	}

	// Truncate stdout if configured
	if v.config.LoggingOptions.TruncateStdout && v.config.LoggingOptions.MaxStdoutLength > 0 && len(output) > v.config.LoggingOptions.MaxStdoutLength {
		output = output[:v.config.LoggingOptions.MaxStdoutLength] + "...[truncated for security]"
	}

	return output
}

// redactSensitivePatterns removes or redacts potentially sensitive information
func (v *Validator) redactSensitivePatterns(text string) string {
	// Use the new common redaction functionality
	return v.redactionConfig.RedactText(text)
}

// CreateSafeLogFields creates log fields with sensitive data redaction
func (v *Validator) CreateSafeLogFields(fields map[string]any) map[string]any {
	if !v.config.LoggingOptions.RedactSensitiveInfo {
		return fields
	}

	safeFields := make(map[string]any)
	for k, value := range fields {
		switch val := value.(type) {
		case string:
			safeFields[k] = v.SanitizeOutputForLogging(val)
		case error:
			safeFields[k] = v.SanitizeErrorForLogging(val)
		default:
			// For non-string, non-error types, include as-is
			safeFields[k] = value
		}
	}

	return safeFields
}

// LogFieldsWithError creates safe log fields including a sanitized error
func (v *Validator) LogFieldsWithError(baseFields map[string]any, err error) map[string]any {
	fields := make(map[string]any)

	// Copy base fields with sanitization
	for k, value := range baseFields {
		switch val := value.(type) {
		case string:
			fields[k] = v.SanitizeOutputForLogging(val)
		case error:
			fields[k] = v.SanitizeErrorForLogging(val)
		default:
			fields[k] = value
		}
	}

	// Add sanitized error
	if err != nil {
		fields["error"] = v.SanitizeErrorForLogging(err)
	}

	return fields
}
