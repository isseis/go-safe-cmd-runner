// Package output provides functionality for capturing command output to files.
// It includes types for output management, path validation, and error handling.
package output

import (
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

// CaptureError represents an error that occurred during output capture
type CaptureError struct {
	Type  ErrorType      // Type of error
	Path  string         // File path related to the error
	Phase ExecutionPhase // Execution phase when error occurred
	Cause error          // Underlying cause of the error
}

// Error implements the error interface
func (e CaptureError) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("output capture error during %s: %s for '%s'",
			e.Phase.String(),
			e.Type.String(),
			e.Path)
	}

	return fmt.Sprintf("output capture error during %s: %s for '%s': %v",
		e.Phase.String(),
		e.Type.String(),
		e.Path,
		e.Cause)
}

// Unwrap implements the error unwrapping interface
func (e CaptureError) Unwrap() error {
	return e.Cause
}
