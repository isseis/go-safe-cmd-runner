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

// UserFriendlyError defines the interface for errors that can provide user-friendly messages
// This allows specific error types to customize how they appear in error summaries
type UserFriendlyError interface {
	error
	// UserMessage returns a user-friendly description of the error
	// Returns empty string if no special message is needed
	UserMessage() string
}

// GetUserFriendlyMessage extracts a user-friendly message from an error chain
// Returns empty string if no user-friendly message is available
func GetUserFriendlyMessage(err error) string {
	if friendlyErr, ok := errors.AsType[UserFriendlyError](err); ok {
		return friendlyErr.UserMessage()
	}
	return ""
}

// formatCause renders an error cause for a user-facing report: each error's
// UserMessage when it has one, otherwise its raw text. A multi-error (such as
// the errors.Join of every failed group) is split into its children first,
// because GetUserFriendlyMessage walks every branch of the join and would
// replace the whole report with the one friendly child, dropping its
// siblings. Children are joined with "\n", the separator errors.Join itself
// uses, so the report stays one line per error.
func formatCause(err error) string {
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		children := multi.Unwrap()
		parts := make([]string, 0, len(children))
		for _, child := range children {
			parts = append(parts, formatCause(child))
		}
		return strings.Join(parts, "\n")
	}
	if userMsg := GetUserFriendlyMessage(err); userMsg != "" {
		return userMsg
	}
	return err.Error()
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
