// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// IntPtr returns a pointer to the given int value.
// This is a convenience function for creating timeout values.
func IntPtr(v int) *int {
	return &v
}
