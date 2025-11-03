package errors

import (
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// ClassifyVerificationError creates a ClassifiedError for verification-related errors with configurable severity
//
//nolint:unparam // severity parameter is intentionally configurable for future flexibility, even though currently all calls use ErrorSeverityCritical
func ClassifyVerificationError(errorType ErrorType, severity ErrorSeverity, message, filePath string, cause error) *ClassifiedError {
	return &ClassifiedError{
		Type:      errorType,
		Severity:  severity,
		Message:   message,
		Cause:     cause,
		Component: string(resource.ComponentVerification), // Always verification for this helper function
		FilePath:  filePath,
		Timestamp: time.Now(),
	}
}
