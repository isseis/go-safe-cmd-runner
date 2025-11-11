// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// IntPtr returns a pointer to the given int value.
// This is a convenience function for creating timeout values.
func Int32Ptr(v int32) *int32 {
	return &v
}

// Int64Ptr returns a pointer to the given int64 value.
// This is a convenience function for creating output size limit values.
func Int64Ptr(v int64) *int64 {
	return &v
}

// BoolPtr returns a pointer to the given bool value.
// This is a convenience function for creating pointer values in tests and configuration.
func BoolPtr(v bool) *bool {
	return &v
}
