// Package encoding provides error types for hybrid hash filename encoding operations.
//
// This file defines all error types used throughout the encoding package,
// providing structured error handling with detailed context information.
package encoding

import (
	"errors"
	"fmt"
)

// Static errors for common invalid path cases.
//
// These sentinel errors can be used with errors.Is() for error handling:
//
//	if errors.Is(err, encoding.ErrEmptyPath) {
//	    // Handle empty path case
//	}
var (
	// ErrEmptyPath indicates an empty path was provided to encoding functions.
	// Empty paths cannot be encoded as they represent invalid filesystem paths.
	ErrEmptyPath = errors.New("empty path")

	// ErrNotAbsoluteOrNormalized indicates the path is not absolute or normalized.
	// The encoder requires absolute paths that have been cleaned with filepath.Clean()
	// to ensure consistent and predictable encoding results.
	ErrNotAbsoluteOrNormalized = errors.New("path is not absolute or normalized")
)

// ErrFallbackNotReversible indicates a fallback encoding cannot be decoded.
//
// SHA256 fallback encodings are one-way transformations that cannot be reversed
// to recover the original path. This error is returned when attempting to decode
// a filename that uses SHA256 fallback format.
//
// The error includes the encoded name to help with debugging and user feedback.
//
// Example usage:
//
//	_, err := encoder.Decode("AbCdEf123456.json")
//	var fallbackErr ErrFallbackNotReversible
//	if errors.As(err, &fallbackErr) {
//	    fmt.Printf("Cannot decode fallback encoding: %s", fallbackErr.EncodedName)
//	}
type ErrFallbackNotReversible struct {
	// EncodedName is the filename that uses SHA256 fallback format
	EncodedName string
}

// Error returns a descriptive error message including the encoded filename.
func (e ErrFallbackNotReversible) Error() string {
	return fmt.Sprintf("fallback encoding '%s' cannot be decoded to original path", e.EncodedName)
}

// ErrInvalidPath represents an error for invalid file paths during encoding operations.
//
// This error wraps path validation failures with context about the invalid path
// and the underlying cause. It supports error unwrapping for compatibility with
// errors.Is() and errors.As() functions.
//
// Common causes include:
//   - Empty paths (wrapped ErrEmptyPath)
//   - Relative paths (wrapped ErrNotAbsoluteOrNormalized)
//   - Non-normalized paths (wrapped ErrNotAbsoluteOrNormalized)
//   - Invalid characters or encoding issues
//
// Example usage:
//
//	_, err := encoder.Encode("relative/path")
//	var pathErr ErrInvalidPath
//	if errors.As(err, &pathErr) {
//	    fmt.Printf("Invalid path '%s': %v", pathErr.Path, pathErr.Err)
//	}
//
//	// Check for specific underlying errors:
//	if errors.Is(err, ErrNotAbsoluteOrNormalized) {
//	    // Handle non-absolute path case
//	}
type ErrInvalidPath struct {
	// Path is the invalid file path that caused the error
	Path string
	// Err is the underlying error that describes why the path is invalid
	Err error
}

// Error returns a descriptive error message including the path and underlying cause.
func (e ErrInvalidPath) Error() string {
	return fmt.Sprintf("invalid path: %s (error: %v)", e.Path, e.Err)
}

// Unwrap returns the underlying error for use with errors.Is() and errors.As().
func (e *ErrInvalidPath) Unwrap() error {
	return e.Err
}
