// Package color provides small helpers for coloring terminal output using
// ANSI escape sequences. It is intended for internal example and logging
// use; functions here return formatted strings and helpers to conditionally
// enable/disable color output.
//
//nolint:revive // package name conflicts with standard library
package color

// ANSI color codes
const (
	resetCode  = "\033[0m"
	grayCode   = "\033[90m" // Bright black/gray
	greenCode  = "\033[32m"
	yellowCode = "\033[33m"
	redCode    = "\033[31m"
	blueCode   = "\033[34m"
	purpleCode = "\033[35m"
	cyanCode   = "\033[36m"
	whiteCode  = "\033[37m"
)

// Color represents a color function that wraps text with ANSI escape
// sequences.
type Color func(text string) string

// NewColor creates a color function with the specified ANSI code.
func NewColor(ansiCode string) Color {
	return func(text string) string {
		return ansiCode + text + resetCode
	}
}

// Predefined color functions
var (
	// Gray colors text in gray (bright black)
	Gray = NewColor(grayCode)

	// Green colors text in green
	Green = NewColor(greenCode)

	// Yellow colors text in yellow
	Yellow = NewColor(yellowCode)

	// Red colors text in red
	Red = NewColor(redCode)

	// Blue colors text in blue
	Blue = NewColor(blueCode)

	// Purple colors text in purple
	Purple = NewColor(purpleCode)

	// Cyan colors text in cyan
	Cyan = NewColor(cyanCode)

	// White colors text in white
	White = NewColor(whiteCode)
)
