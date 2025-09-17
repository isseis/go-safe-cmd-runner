package encoding

import (
	"errors"
	"fmt"
)

// Static errors for common invalid path cases
var (
	// ErrEmptyPath indicates an empty path was provided
	ErrEmptyPath = errors.New("empty path")
	// ErrNotAbsoluteOrNormalized indicates the path is not absolute or normalized
	ErrNotAbsoluteOrNormalized = errors.New("path is not absolute or normalized")
)

// ErrFallbackNotReversible indicates a fallback encoding cannot be decoded
type ErrFallbackNotReversible struct {
	EncodedName string
}

func (e ErrFallbackNotReversible) Error() string {
	return fmt.Sprintf("fallback encoding '%s' cannot be decoded to original path", e.EncodedName)
}

// ErrInvalidPath represents an error for invalid file paths during encoding operations
type ErrInvalidPath struct {
	Path string // The invalid path
	Err  error  // The underlying error, if any
}

func (e ErrInvalidPath) Error() string {
	return fmt.Sprintf("invalid path: %s (error: %v)", e.Path, e.Err)
}

func (e *ErrInvalidPath) Unwrap() error {
	return e.Err
}
