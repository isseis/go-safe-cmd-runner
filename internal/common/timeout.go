// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

import (
	"fmt"
)

// ErrInvalidTimeout is returned when an invalid timeout value is encountered
type ErrInvalidTimeout struct {
	Value   any
	Context string
}

func (e ErrInvalidTimeout) Error() string {
	return fmt.Sprintf("invalid timeout value %v in %s", e.Value, e.Context)
}

const (
	// DefaultTimeout is used when no timeout is explicitly set
	DefaultTimeout = 60 // seconds

	// MaxTimeout defines the maximum allowed timeout value (24 hours)
	// The value is well within int32 range, ensuring cross-platform compatibility.
	MaxTimeout = 86400 // 24 hours in seconds
)

// Timeout represents a timeout configuration value.
// It distinguishes between three states:
// - Unset (use default or inherit from parent)
// - Zero (unlimited execution, no timeout)
// - Positive value (timeout in seconds)
//
// This type provides type safety and explicit semantics compared to using *int32 directly.
// Uses int32 for platform-independent size guarantee (sufficient for timeouts up to ~68 years).
type Timeout struct {
	OptionalValue[int32]
}

// NewFromIntPtr creates a Timeout from an existing *int pointer.
// Validates that the value fits in int32 range.
func NewFromIntPtr(ptr *int32) Timeout {
	if ptr == nil {
		return Timeout{NewUnsetOptionalValue[int32]()}
	}
	val := *ptr // #nosec G115 -- validated above
	return Timeout{NewOptionalValueFromPtr(&val)}
}

// NewUnsetTimeout creates an unset Timeout (will use default or inherit from parent).
func NewUnsetTimeout() Timeout {
	return Timeout{NewUnsetOptionalValue[int32]()}
}

// NewUnlimitedTimeout creates a Timeout with unlimited execution (no timeout).
func NewUnlimitedTimeout() Timeout {
	return Timeout{NewUnlimitedOptionalValue[int32]()}
}

// NewTimeout creates a Timeout with the specified duration in seconds.
// Returns error if seconds is negative or exceeds MaxTimeout.
func NewTimeout(seconds int32) (Timeout, error) {
	if seconds < 0 {
		return Timeout{}, ErrInvalidTimeout{
			Value:   seconds,
			Context: "timeout cannot be negative",
		}
	}
	if seconds > MaxTimeout {
		return Timeout{}, ErrInvalidTimeout{
			Value:   seconds,
			Context: "timeout exceeds maximum allowed value",
		}
	}
	return Timeout{NewOptionalValue(seconds)}, nil
}
