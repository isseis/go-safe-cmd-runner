// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// TimeoutValue represents a timeout setting that can be unset, zero, or positive
type TimeoutValue *int

const (
	// DefaultTimeout is used when no timeout is explicitly set
	DefaultTimeout = 60 // seconds

	// MaxTimeout defines the maximum allowed timeout value (24 hours)
	// Using int32 max to ensure cross-platform compatibility
	MaxTimeout = 86400 // 24 hours in seconds
)

// Helper functions for creating timeout value pointers

// IntPtr returns a pointer to the given int value.
// This is a convenience function for creating timeout values.
func IntPtr(v int) *int {
	return &v
}

// TimeoutPtr returns a TimeoutValue (pointer to int) for the given value.
// This is a convenience function for creating timeout values.
func TimeoutPtr(v int) TimeoutValue {
	return &v
}

// IsTimeoutUnset returns true if the timeout value is unset (nil).
func IsTimeoutUnset(timeout *int) bool {
	return timeout == nil
}

// IsTimeoutUnlimited returns true if the timeout value is explicitly set to 0 (unlimited).
func IsTimeoutUnlimited(timeout *int) bool {
	return timeout != nil && *timeout == 0
}

// GetTimeoutValue safely returns the timeout value, returning DefaultTimeout if unset.
// For unlimited timeout (0), it returns 0.
func GetTimeoutValue(timeout *int) int {
	if timeout == nil {
		return DefaultTimeout
	}
	return *timeout
}
