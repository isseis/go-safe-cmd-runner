// Package redaction provides error types for redaction operations.
package redaction

import "fmt"

// ErrLogValuePanic is returned when LogValue() panics
type ErrLogValuePanic struct {
	Key        string
	PanicValue any
	StackTrace string
}

func (e *ErrLogValuePanic) Error() string {
	return fmt.Sprintf("LogValue() panicked for attribute %q: %v", e.Key, e.PanicValue)
}

// ErrRegexCompilationFailed is returned when regex compilation fails
type ErrRegexCompilationFailed struct {
	Pattern string
	Err     error
}

func (e *ErrRegexCompilationFailed) Error() string {
	return fmt.Sprintf("failed to compile regex pattern %q: %v", e.Pattern, e.Err)
}
