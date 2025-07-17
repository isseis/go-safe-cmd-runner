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

// VerificationError represents an error that occurred during file verification
//
//nolint:revive // VerificationError is descriptive and distinguishes from the base Error type
type VerificationError struct {
	Op      string   // operation that failed (e.g., "global", "group")
	Group   string   // group name (if applicable)
	Details []string // details about the error (e.g., failed files)
	Err     error    // underlying error
}

// Error returns the error message
func (e *VerificationError) Error() string {
	if e.Group != "" {
		return fmt.Sprintf("verification error in %s for group %s: %v", e.Op, e.Group, e.Err)
	}
	return fmt.Sprintf("verification error in %s: %v", e.Op, e.Err)
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
