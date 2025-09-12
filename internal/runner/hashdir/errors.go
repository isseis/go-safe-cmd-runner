// Package hashdir provides secure hash directory validation and management.
package hashdir

import (
	"errors"
	"fmt"
)

// HashDirectoryErrorType represents different types of hash directory validation errors
type HashDirectoryErrorType int

const (
	// HashDirectoryErrorTypeRelativePath indicates a relative path was provided instead of absolute
	HashDirectoryErrorTypeRelativePath HashDirectoryErrorType = iota
	// HashDirectoryErrorTypeNotFound indicates the directory does not exist
	HashDirectoryErrorTypeNotFound
	// HashDirectoryErrorTypeNotDirectory indicates the path exists but is not a directory
	HashDirectoryErrorTypeNotDirectory
	// HashDirectoryErrorTypePermission indicates insufficient permissions to access the directory
	HashDirectoryErrorTypePermission
	// HashDirectoryErrorTypeSymlinkAttack indicates a potential symlink attack
	HashDirectoryErrorTypeSymlinkAttack
)

// HashDirectoryError represents an error in hash directory validation
type HashDirectoryError struct {
	Type  HashDirectoryErrorType
	Path  string
	Cause error
}

// Error implements the error interface for HashDirectoryError
func (e *HashDirectoryError) Error() string {
	switch e.Type {
	case HashDirectoryErrorTypeRelativePath:
		return fmt.Sprintf("hash directory must be absolute path, got relative path: %s", e.Path)
	case HashDirectoryErrorTypeNotFound:
		return fmt.Sprintf("hash directory not found: %s", e.Path)
	case HashDirectoryErrorTypeNotDirectory:
		return fmt.Sprintf("hash directory path is not a directory: %s", e.Path)
	case HashDirectoryErrorTypePermission:
		return fmt.Sprintf("insufficient permissions to access hash directory: %s", e.Path)
	case HashDirectoryErrorTypeSymlinkAttack:
		return fmt.Sprintf("potential symlink attack detected for hash directory: %s", e.Path)
	default:
		return fmt.Sprintf("unknown hash directory error for path: %s", e.Path)
	}
}

// Is implements error unwrapping for HashDirectoryError
func (e *HashDirectoryError) Is(target error) bool {
	if e.Cause != nil {
		return errors.Is(e.Cause, target)
	}
	return false
}

// Unwrap implements error unwrapping for HashDirectoryError
func (e *HashDirectoryError) Unwrap() error {
	return e.Cause
}
