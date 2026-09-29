package logging

import (
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
)

// ExecutionError represents an error that occurs during command execution
// (as opposed to pre-execution errors like configuration parsing or file access)
type ExecutionError struct {
	Message     errmsg.Summary
	Component   string
	RunID       string
	GroupName   string // Optional: name of the group where error occurred
	CommandName string // Optional: name of the command where error occurred
	Err         error  // Wrapped error for better error context preservation
}

// contextParts returns the group and command names as parts. It is the single
// place that builds the context; ReportMessage and ContextString both use it.
func (e *ExecutionError) contextParts() []errmsg.Part {
	var parts []errmsg.Part
	if e.GroupName != "" {
		parts = append(parts, errmsg.Const("group: "), errmsg.Ident(e.GroupName))
	}
	if e.CommandName != "" {
		if len(parts) > 0 {
			parts = append(parts, errmsg.Const(", "))
		}
		parts = append(parts, errmsg.Const("command: "), errmsg.Ident(e.CommandName))
	}
	return parts
}

// ContextString returns the context information (group and command names) as a formatted string
// Returns empty string if no context is available
func (e *ExecutionError) ContextString() string {
	return errmsg.NewMessage(e.contextParts()...).String()
}

// ReportMessage returns the message HandleExecutionError reports: Message, the
// group/command context, and the cause, as a structured message.
func (e *ExecutionError) ReportMessage() errmsg.Message {
	parts := []errmsg.Part{e.Message.Part()}
	if context := e.contextParts(); len(context) > 0 {
		parts = append(parts, errmsg.Const(" ("))
		parts = append(parts, context...)
		parts = append(parts, errmsg.Const(")"))
	}
	if e.Err != nil {
		parts = append(parts, errmsg.Const(": "), errmsg.Cause(e.Err))
	}
	return errmsg.NewMessage(parts...)
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
