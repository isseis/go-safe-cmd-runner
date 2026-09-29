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

// ErrMessageFlattenPanic is returned when flattening an errmsg.Message panics,
// for example because a cause's Error() or StructuredMessage() panicked.
//
// It carries only the panic value's type. The panic value itself and the stack
// trace are deliberately not kept: the value can be built from the text being
// redacted, and this error is printed verbatim by ShutdownReporter.
type ErrMessageFlattenPanic struct {
	PanicType string
}

func (e *ErrMessageFlattenPanic) Error() string {
	return fmt.Sprintf("flattening a structured message panicked (panic type %s)", e.PanicType)
}

// ErrMessageRangeMismatch is returned when the ranges from redactedRanges do
// not reproduce RedactText on a structured message's unredacted rendering.
// It deliberately carries no text: the rendering may contain secrets.
type ErrMessageRangeMismatch struct {
	RangeCount int
}

func (e *ErrMessageRangeMismatch) Error() string {
	return fmt.Sprintf("structured message redaction: %d redacted ranges do not reproduce RedactText", e.RangeCount)
}
