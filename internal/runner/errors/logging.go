//nolint:revive // package name conflicts with standard library
package errors

import (
	"fmt"
	"log/slog"
	"os"
	"time"
)

// LogCriticalToStderr logs critical security-related errors to stderr regardless of log level
func LogCriticalToStderr(component, message string, err error) {
	timestamp := time.Now().Format("2006-01-02T15:04:05Z07:00")
	fmt.Fprintf(os.Stderr, "[%s] CRITICAL: %s - Component: %s, Error: %v\n", timestamp, message, component, err)
}

// LogClassifiedError logs errors based on their classification with appropriate severity handling
func LogClassifiedError(classifiedErr *ClassifiedError) {
	switch classifiedErr.Severity {
	case ErrorSeverityCritical:
		LogCriticalToStderr(classifiedErr.Component, classifiedErr.Message, classifiedErr.Cause)
		slog.Error("CRITICAL: Security error detected",
			"error_type", classifiedErr.Type,
			"message", classifiedErr.Message,
			"component", classifiedErr.Component,
			"file_path", classifiedErr.FilePath,
			"cause", classifiedErr.Cause)
	case ErrorSeverityWarning:
		slog.Warn("Security warning",
			"error_type", classifiedErr.Type,
			"message", classifiedErr.Message,
			"component", classifiedErr.Component,
			"file_path", classifiedErr.FilePath,
			"cause", classifiedErr.Cause)
	case ErrorSeverityInfo:
		slog.Info("Security information",
			"error_type", classifiedErr.Type,
			"message", classifiedErr.Message,
			"component", classifiedErr.Component,
			"file_path", classifiedErr.FilePath,
			"cause", classifiedErr.Cause)
	}
}
