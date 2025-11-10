package logging

import "fmt"

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

// Error implements the error interface
func (e *ExecutionError) Error() string {
	var contextInfo string
	switch {
	case e.GroupName != "" && e.CommandName != "":
		contextInfo = fmt.Sprintf("group: %s, command: %s, ", e.GroupName, e.CommandName)
	case e.GroupName != "":
		contextInfo = fmt.Sprintf("group: %s, ", e.GroupName)
	case e.CommandName != "":
		contextInfo = fmt.Sprintf("command: %s, ", e.CommandName)
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
