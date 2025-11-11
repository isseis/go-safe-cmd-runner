// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// ResolveOutputSizeLimit resolves the effective output size limit following the hierarchy:
// 1. Command-level output_size_limit (if set)
// 2. Global-level output_size_limit (if set)
// 3. Default output size limit (10MB)
//
// Parameters:
//   - commandLimit: Command-level output_size_limit
//   - globalLimit: Global-level output_size_limit
//
// Returns:
//   - Resolved OutputSizeLimit (use IsUnlimited() to check for unlimited, Value() to get bytes)
//
// Resolution logic:
//   - If commandLimit is set, use its value (can be 0 for unlimited)
//   - Otherwise, if globalLimit is set, use its value (can be 0 for unlimited)
//   - Otherwise, use DefaultOutputSizeLimit
func ResolveOutputSizeLimit(commandLimit OutputSizeLimit, globalLimit OutputSizeLimit) OutputSizeLimit {
	// Command-level takes precedence
	if commandLimit.IsSet() {
		return commandLimit
	}

	// Fall back to global-level
	if globalLimit.IsSet() {
		return globalLimit
	}

	// Use default
	return NewOutputSizeLimit(DefaultOutputSizeLimit)
}
