package verification

import (
	"errors"
	"fmt"
)

var (
	// ErrVerificationDisabled is returned when verification is disabled
	ErrVerificationDisabled = errors.New("verification is disabled")
	// ErrHashDirectoryEmpty is returned when hash directory is empty
	ErrHashDirectoryEmpty = errors.New("hash directory cannot be empty")
	// ErrHashDirectoryInvalid is returned when hash directory is invalid
	ErrHashDirectoryInvalid = errors.New("hash directory is invalid")
	// ErrConfigNil is returned when config is nil
	ErrConfigNil = errors.New("config cannot be nil")
	// ErrSecurityValidatorNotInitialized is returned when security validator is not initialized
	ErrSecurityValidatorNotInitialized = errors.New("security validator not initialized")
)

// Error represents a verification error with context
type Error struct {
	Op   string // operation that failed
	Path string // file path (if applicable)
	Err  error  // underlying error
}

// Error returns the error message
func (e *Error) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("verification error in %s for %s: %v", e.Op, e.Path, e.Err)
	}
	return fmt.Sprintf("verification error in %s: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target error
func (e *Error) Is(target error) bool {
	return errors.Is(e.Err, target)
}
