package logging

import (
	"errors"
	"fmt"
	"strings"
)

// ExecutionError represents an error that occurs during command execution
// (as opposed to pre-execution errors like configuration parsing or file access)
type ExecutionError struct {
	Message     string
	Component   string
	RunID       string
	GroupName   string // Optional: name of the group where error occurred
	CommandName string // Optional: name of the command where error occurred
	Err         error  // Wrapped error for better error context preservation
}

// CaptureErrorInterface defines the interface for output capture errors
// This allows us to detect output-related errors without importing the output package
type CaptureErrorInterface interface {
	error
	GetType() string
	GetPath() string
}

// IsOutputSizeLimitError checks if the error chain contains an output size limit error
// Returns (isLimitError, outputPath)
func IsOutputSizeLimitError(err error) (bool, string) {
	var captureErr CaptureErrorInterface
	if errors.As(err, &captureErr) {
		if captureErr.GetType() == "size limit exceeded" {
			return true, captureErr.GetPath()
		}
	}
	return false, ""
}

// ContextString returns the context information (group and command names) as a formatted string
// Returns empty string if no context is available
func (e *ExecutionError) ContextString() string {
	var parts []string
	if e.GroupName != "" {
		parts = append(parts, fmt.Sprintf("group: %s", e.GroupName))
	}
	if e.CommandName != "" {
		parts = append(parts, fmt.Sprintf("command: %s", e.CommandName))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// Error implements the error interface
func (e *ExecutionError) Error() string {
	contextInfo := e.ContextString()
	if contextInfo != "" {
		contextInfo += ", "
	}

	if e.Err != nil {
		return fmt.Sprintf("execution error: %s: %v (%scomponent: %s, run_id: %s)", e.Message, e.Err, contextInfo, e.Component, e.RunID)
	}
	return fmt.Sprintf("execution error: %s (%scomponent: %s, run_id: %s)", e.Message, contextInfo, e.Component, e.RunID)
}

// Unwrap implements error wrapping for errors.Unwrap
func (e *ExecutionError) Unwrap() error {
	return e.Err
}
