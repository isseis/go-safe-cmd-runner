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

// ErrPathTooLong indicates the encoded path exceeds maximum length
type ErrPathTooLong struct {
	Path          string
	EncodedLength int
	MaxLength     int
}

func (e ErrPathTooLong) Error() string {
	return fmt.Sprintf("encoded path too long: %d characters (max: %d) for path: %s",
		e.EncodedLength, e.MaxLength, e.Path)
}

// ErrInvalidEncodedName indicates the encoded name format is invalid
type ErrInvalidEncodedName struct {
	EncodedName string
	Reason      string
}

func (e ErrInvalidEncodedName) Error() string {
	return fmt.Sprintf("invalid encoded name '%s': %s", e.EncodedName, e.Reason)
}
