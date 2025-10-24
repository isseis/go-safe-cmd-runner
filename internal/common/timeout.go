// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// Timeout represents a timeout configuration value.
// It distinguishes between three states:
// - Unset (use default or inherit from parent)
// - Zero (unlimited execution, no timeout)
// - Positive value (timeout in seconds)
//
// This type provides type safety and explicit semantics compared to using *int directly.
type Timeout struct {
	value *int
}

// NewFromIntPtr creates a Timeout from an existing *int pointer.
func NewFromIntPtr(ptr *int) Timeout {
	return Timeout{value: ptr}
}

// NewUnsetTimeout creates an unset Timeout (will use default or inherit from parent).
func NewUnsetTimeout() Timeout {
	return Timeout{value: nil}
}

// NewUnlimitedTimeout creates a Timeout with unlimited execution (no timeout).
func NewUnlimitedTimeout() Timeout {
	zero := 0
	return Timeout{value: &zero}
}

// NewTimeout creates a Timeout with the specified duration in seconds.
// Returns error if seconds is negative or exceeds MaxTimeout.
func NewTimeout(seconds int) (Timeout, error) {
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
	return Timeout{value: &seconds}, nil
}

// IsSet returns true if the timeout has been explicitly set (non-nil).
func (t Timeout) IsSet() bool {
	return t.value != nil
}

// IsUnlimited returns true if the timeout is explicitly set to unlimited (0).
// Returns false if the timeout is unset (nil).
func (t Timeout) IsUnlimited() bool {
	return t.value != nil && *t.value == 0
}

// Value returns the timeout value in seconds.
// Returns 0 if unset (caller should apply default or inherit from parent).
// For unlimited timeout, returns 0 (use IsUnlimited to distinguish from unset).
func (t Timeout) Value() int {
	if t.value == nil {
		return 0
	}
	return *t.value
}
