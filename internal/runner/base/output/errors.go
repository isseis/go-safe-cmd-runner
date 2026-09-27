// Package output provides functionality for capturing command output to files.
// It includes types for output management, path validation, and error handling.
package output

import (
	"errors"
	"fmt"
)

// ErrorType represents the type of output capture error
type ErrorType int

const (
	// ErrorTypePathValidation indicates path validation errors
	ErrorTypePathValidation ErrorType = iota
	// ErrorTypePermission indicates permission-related errors
	ErrorTypePermission
	// ErrorTypeFileSystem indicates filesystem operation errors
	ErrorTypeFileSystem
	// ErrorTypeSizeLimit indicates size limit exceeded errors
	ErrorTypeSizeLimit
	// ErrorTypeCleanup indicates cleanup operation errors
	ErrorTypeCleanup
)

// String returns a string representation of ErrorType
func (e ErrorType) String() string {
	switch e {
	case ErrorTypePathValidation:
		return "path validation"
	case ErrorTypePermission:
		return "permission denied"
	case ErrorTypeFileSystem:
		return "filesystem error"
	case ErrorTypeSizeLimit:
		return "size limit exceeded"
	case ErrorTypeCleanup:
		return "cleanup failed"
	default:
		return "unknown error"
	}
}

// ExecutionPhase represents the phase during which an error occurred
type ExecutionPhase int

const (
	// PhasePreparation indicates errors during preparation phase
	PhasePreparation ExecutionPhase = iota
	// PhaseExecution indicates errors during execution phase
	PhaseExecution
	// PhaseFinalization indicates errors during finalization phase
	PhaseFinalization
	// PhaseCleanup indicates errors during cleanup phase
	PhaseCleanup
)

// String returns a string representation of ExecutionPhase
func (p ExecutionPhase) String() string {
	switch p {
	case PhasePreparation:
		return "preparation phase"
	case PhaseExecution:
		return "execution phase"
	case PhaseFinalization:
		return "finalization phase"
	case PhaseCleanup:
		return "cleanup phase"
	default:
		return "unknown phase"
	}
}

// CaptureError represents an error that occurred during output capture. Its
// fields are unexported, so a value is built only through the constructors
// below; the compiler rejects construction from any other package.
type CaptureError struct {
	typ   ErrorType      // kind of error
	path  string         // file path related to the error
	phase ExecutionPhase // execution phase when the error occurred
	cause error          // underlying cause
	limit int64          // size limit in bytes; set only for ErrorTypeSizeLimit
}

// Error implements the error interface. A size-limit error names the phase,
// the output path and the limit that was exceeded, and does not append its
// cause: the cause is the ErrOutputSizeExceeded sentinel, which states the
// same fact. Every other kind keeps the generic
// "output capture error during <phase>: <type> for '<path>'" text with its
// cause appended.
func (e *CaptureError) Error() string {
	switch e.typ {
	case ErrorTypeSizeLimit:
		return fmt.Sprintf("output capture error during %s: output size limit exceeded for '%s' (limit: %d bytes)",
			e.phase.String(),
			e.path,
			e.limit)
	default:
		if e.cause == nil {
			return fmt.Sprintf("output capture error during %s: %s for '%s'",
				e.phase.String(),
				e.typ.String(),
				e.path)
		}

		return fmt.Sprintf("output capture error during %s: %s for '%s': %v",
			e.phase.String(),
			e.typ.String(),
			e.path,
			e.cause)
	}
}

// Unwrap implements the error unwrapping interface.
func (e *CaptureError) Unwrap() error {
	return e.cause
}

// newSizeLimitError builds the size-limit error. Its phase is PhaseExecution
// and its cause is always ErrOutputSizeExceeded, so errors.Is against the
// sentinel holds for every size-limit failure. It panics when limit is not
// positive: such a limit is a caller mistake, and accepting it would attach a
// value to the message that did not bound the write.
func newSizeLimitError(path string, limit int64) *CaptureError {
	if limit <= 0 {
		panic("newSizeLimitError: limit must be positive")
	}
	return &CaptureError{
		typ:   ErrorTypeSizeLimit,
		path:  path,
		phase: PhaseExecution,
		cause: ErrOutputSizeExceeded,
		limit: limit,
	}
}

// newFileSystemError builds the error for a failed write to the output file.
// Its phase is PhaseExecution and its cause is the underlying write error.
func newFileSystemError(path string, cause error) *CaptureError {
	return &CaptureError{
		typ:   ErrorTypeFileSystem,
		path:  path,
		phase: PhaseExecution,
		cause: cause,
	}
}

// Standard error values
var (
	// ErrOutputSizeExceeded is returned when output size exceeds the maximum limit
	ErrOutputSizeExceeded = errors.New("output size limit exceeded")
	// ErrOutputPathRequired is returned when output path is required but not provided
	ErrOutputPathRequired = errors.New("output path is required")
	// ErrInvalidMaxSize is returned when maximum size is invalid
	ErrInvalidMaxSize = errors.New("invalid maximum size")
)
