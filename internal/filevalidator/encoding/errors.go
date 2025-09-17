package encoding

import "errors"

var (
	// ErrFallbackNotReversible is returned when attempting to decode a fallback-encoded name
	ErrFallbackNotReversible = errors.New("fallback encoding cannot be decoded to original path")

	// ErrPathTooLong is returned when the encoded path exceeds maximum length
	ErrPathTooLong = errors.New("encoded path too long")

	// ErrInvalidEncodedName is returned when the encoded name format is invalid
	ErrInvalidEncodedName = errors.New("invalid encoded name format")

	// ErrEmptyPath is returned when an empty path is provided
	ErrEmptyPath = errors.New("path cannot be empty")
)
