//nolint:revive // VerificationError is descriptive and distinguishes from the base Error type
package verification

import (
	"errors"
	"fmt"
	"time"
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
	// ErrGlobalVerificationFailed is returned when global file verification fails
	ErrGlobalVerificationFailed = errors.New("global file verification failed")
	// ErrGroupVerificationFailed is returned when group file verification fails
	ErrGroupVerificationFailed = errors.New("group file verification failed")
	// ErrPathResolverNotInitialized is returned when path resolver is not initialized
	ErrPathResolverNotInitialized = errors.New("path resolver not initialized")
	// ErrCommandNotFound is returned when command is not found in PATH
	ErrCommandNotFound = errors.New("command not found in PATH")
)

// SecurityViolationError is the base error type for security-related violations
type SecurityViolationError struct {
	Op      string    // operation that was attempted
	Context string    // additional context about the violation
	Time    time.Time // when the violation occurred
}

// Error returns the error message
func (e *SecurityViolationError) Error() string {
	return fmt.Sprintf("security violation in %s: %s (at %s)", e.Op, e.Context, e.Time.Format(time.RFC3339))
}

// HashDirectorySecurityError is returned when hash directory security constraints are violated
type HashDirectorySecurityError struct {
	SecurityViolationError
	RequestedDir string // directory that was requested
	DefaultDir   string // the required default directory
	Reason       string // specific reason for rejection
}

// NewHashDirectorySecurityError creates a new HashDirectorySecurityError
func NewHashDirectorySecurityError(requestedDir, defaultDir, reason string) *HashDirectorySecurityError {
	return &HashDirectorySecurityError{
		SecurityViolationError: SecurityViolationError{
			Op:      "HashDirectoryValidation",
			Context: fmt.Sprintf("custom hash directory rejected: %s", reason),
			Time:    time.Now(),
		},
		RequestedDir: requestedDir,
		DefaultDir:   defaultDir,
		Reason:       reason,
	}
}

// Error returns the error message
func (e *HashDirectorySecurityError) Error() string {
	return fmt.Sprintf("hash directory security violation: requested '%s' but only '%s' allowed (%s) (at %s)",
		e.RequestedDir, e.DefaultDir, e.Reason, e.Time.Format(time.RFC3339))
}

// Error represents a verification error with context
type Error struct {
	Op   string // operation that failed
	Path string // file path (if applicable)
	Err  error  // underlying error
}

// Error returns the error message
func (e *Error) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s: %v", e.Path, e.Err)
	}
	return fmt.Sprintf("%s failed: %v", e.Op, e.Err)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target error
func (e *Error) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// VerificationError represents an error that occurred during file verification
//
//nolint:revive // VerificationError is descriptive and distinguishes from the base Error type
type VerificationError struct {
	Op            string   // operation that failed (e.g., "global", "group")
	Group         string   // group name (if applicable)
	Details       []string // details about the error (e.g., failed files)
	TotalFiles    int      // total number of files to verify
	VerifiedFiles int      // number of files successfully verified
	FailedFiles   int      // number of files that failed verification
	SkippedFiles  int      // number of files that were skipped
	Err           error    // underlying error
}

// Error returns the error message
func (e *VerificationError) Error() string {
	var base string
	if e.Group != "" {
		base = fmt.Sprintf("%s verification failed for group %s", e.Op, e.Group)
	} else {
		base = fmt.Sprintf("%s verification failed", e.Op)
	}

	// Include failed file details if available
	if len(e.Details) > 0 {
		return fmt.Sprintf("%s: %d of %d files failed: %v", base, e.FailedFiles, e.TotalFiles, e.Details)
	}

	return base
}

// Unwrap returns the underlying error
func (e *VerificationError) Unwrap() error {
	return e.Err
}

// Is checks if the error matches the target error
func (e *VerificationError) Is(target error) bool {
	return errors.Is(e.Err, target)
}

// Result represents the result of a file verification operation
type Result struct {
	TotalFiles    int           // total number of files to verify
	VerifiedFiles int           // number of files successfully verified
	FailedFiles   []string      // list of files that failed verification
	SkippedFiles  []string      // list of files that were skipped
	Duration      time.Duration // time taken for verification
}

// FileDetail represents detailed information about a single file verification
type FileDetail struct {
	Path         string        // original command/path
	ResolvedPath string        // resolved full path
	HashMatched  bool          // whether hash verification succeeded
	Error        error         // error if verification failed
	Duration     time.Duration // time taken for verification
}
