// Package common provides shared data types and constants used throughout the command runner.
//
//nolint:revive // "common" is an appropriate name for shared utilities package
package common

// DefaultOutputSizeLimit is the default output size limit when not specified (10MB)
const DefaultOutputSizeLimit = 10 * 1024 * 1024

// ResolveOutputSizeLimit resolves the effective output size limit following the hierarchy:
// 1. Command-level output_size_limit (if set)
// 2. Global-level output_size_limit (if set)
// 3. Default output size limit (10MB)
//
// Parameters:
//   - commandLimit: Command-level output_size_limit (*int64, can be nil)
//   - globalLimit: Global-level output_size_limit (*int64, can be nil)
//
// Returns:
//   - Resolved output size limit in bytes (0 means unlimited)
//
// Resolution logic:
//   - If commandLimit is not nil, use *commandLimit (can be 0 for unlimited)
//   - Otherwise, if globalLimit is not nil, use *globalLimit (can be 0 for unlimited)
//   - Otherwise, use DefaultOutputSizeLimit
func ResolveOutputSizeLimit(commandLimit *int64, globalLimit *int64) int64 {
	// Command-level takes precedence
	if commandLimit != nil {
		return *commandLimit
	}

	// Fall back to global-level
	if globalLimit != nil {
		return *globalLimit
	}

	// Use default
	return DefaultOutputSizeLimit
}
