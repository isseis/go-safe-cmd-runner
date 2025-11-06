package logging

import "fmt"

// ExecutionError represents an error that occurs during command execution
// (as opposed to pre-execution errors like configuration parsing or file access)
type ExecutionError struct {
	Message   string
	Component string
	RunID     string
	Err       error // Wrapped error for better error context preservation
}

// Error implements the error interface
func (e *ExecutionError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("execution error: %s: %v (component: %s, run_id: %s)", e.Message, e.Err, e.Component, e.RunID)
	}
	return fmt.Sprintf("execution error: %s (component: %s, run_id: %s)", e.Message, e.Component, e.RunID)
}

// Unwrap implements error wrapping for errors.Unwrap
func (e *ExecutionError) Unwrap() error {
	return e.Err
}
