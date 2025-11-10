// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// DefaultOutputSizeLimit is the default output size limit when not specified (10MB)
const DefaultOutputSizeLimit = 10 * 1024 * 1024

// OutputSizeLimit represents an output size limit configuration value.
// It distinguishes between three states:
// - Unset (use default or inherit from parent)
// - Zero (unlimited output)
// - Positive value (size limit in bytes)
//
// This type provides type safety and explicit semantics compared to using *int64 directly.
type OutputSizeLimit struct {
	OptionalValue[int64]
}

// NewOutputSizeLimitFromPtr creates an OutputSizeLimit from an existing *int64 pointer.
func NewOutputSizeLimitFromPtr(ptr *int64) OutputSizeLimit {
	return OutputSizeLimit{NewOptionalValueFromPtr(ptr)}
}

// NewUnsetOutputSizeLimit creates an unset OutputSizeLimit (will use default or inherit from parent).
func NewUnsetOutputSizeLimit() OutputSizeLimit {
	return OutputSizeLimit{NewUnsetOptionalValue[int64]()}
}

// NewUnlimitedOutputSizeLimit creates an OutputSizeLimit with unlimited output (no limit).
func NewUnlimitedOutputSizeLimit() OutputSizeLimit {
	return OutputSizeLimit{NewUnlimitedOptionalValue[int64]()}
}

// NewOutputSizeLimit creates an OutputSizeLimit with the specified size in bytes.
func NewOutputSizeLimit(bytes int64) OutputSizeLimit {
	return OutputSizeLimit{NewOptionalValue(bytes)}
}
