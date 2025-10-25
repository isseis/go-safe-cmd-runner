package debug

// Package-level constants for display formatting in debug output.
// These constants ensure consistent truncation behavior across different
// debug functions when displaying values.
const (
	// MaxDisplayLength is the maximum length for displaying values in debug output.
	// Values longer than this will be truncated with an ellipsis.
	MaxDisplayLength = 60

	// EllipsisLength is the length of the ellipsis string ("...") used for truncation.
	EllipsisLength = 3
)
