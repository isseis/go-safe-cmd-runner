// Package terminal provides terminal capability detection for interactive UI features.
// This package includes color support detection, interactive terminal detection,
// and user preference management for terminal display options.
package terminal

import (
	"os"
	"strings"
)

// colorTerminals lists TERM values (or prefixes) that are known to support
// basic terminal colors. Declared at package scope to avoid reallocating the
// slice on every SupportsColor call.
var colorTerminals = []string{
	"xterm",
	"screen",
	"tmux",
	"rxvt",
	"vt100",
	"vt220",
	"ansi",
	"linux",
	"cygwin",
	"putty",
}

// ColorDetector interface defines methods for detecting color support
type ColorDetector interface {
	SupportsColor() bool
}

// DefaultColorDetector implements ColorDetector with simple terminal-based detection
type DefaultColorDetector struct{}

// NewColorDetector creates a new color detector
func NewColorDetector() ColorDetector {
	return &DefaultColorDetector{}
}

// SupportsColor returns true if the terminal supports basic color output
// This is a simple implementation that checks the TERM environment variable
func (d *DefaultColorDetector) SupportsColor() bool {
	term := os.Getenv("TERM")
	if term == "" {
		return false
	}

	// Convert to lowercase for case-insensitive comparison
	term = strings.ToLower(strings.TrimSpace(term))

	// Terminals that definitely don't support color
	if term == "dumb" {
		return false
	}

	// Check for exact matches or prefixes using package-level list
	for _, colorTerm := range colorTerminals {
		if term == colorTerm || strings.HasPrefix(term, colorTerm+"-") {
			return true
		}
	}

	// For unknown terminals, default to no color for safety
	// This is a conservative approach - better to miss color support
	// than to output escape sequences to terminals that don't support them
	return false
}
