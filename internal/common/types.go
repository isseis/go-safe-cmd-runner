// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// IntPtr returns a pointer to the given int value.
// This is a convenience function for creating timeout values.
func IntPtr(v int) *int {
	return &v
}

// BoolPtr returns a pointer to the given bool value.
// This is a convenience function for creating pointer values in tests and configuration.
func BoolPtr(v bool) *bool {
	return &v
}
