// Package errors provides error classification and handling for the runner.
package errors

import "time"

// ErrorSeverity represents the severity level of an error
type ErrorSeverity int

const (
	// ErrorSeverityCritical indicates a security-critical error that should terminate execution
	ErrorSeverityCritical ErrorSeverity = iota
	// ErrorSeverityWarning indicates a non-critical issue that allows continued execution
	ErrorSeverityWarning
	// ErrorSeverityInfo indicates an informational message
	ErrorSeverityInfo
)

// ErrorType represents different categories of errors for classification
type ErrorType int

const (
	// ErrorTypeConfigVerification indicates configuration file verification errors
	ErrorTypeConfigVerification ErrorType = iota
	// ErrorTypeEnvironmentVerification indicates environment file verification errors
	ErrorTypeEnvironmentVerification
	// ErrorTypeHashDirectoryValidation indicates hash directory validation errors
	ErrorTypeHashDirectoryValidation
	// ErrorTypeGlobalVerification indicates global file verification errors
	ErrorTypeGlobalVerification
)

// ClassifiedError represents an error with severity and type classification
type ClassifiedError struct {
	Type      ErrorType
	Severity  ErrorSeverity
	Message   string
	Cause     error
	Component string
	FilePath  string
	Timestamp time.Time
}
