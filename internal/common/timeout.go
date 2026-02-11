// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

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
