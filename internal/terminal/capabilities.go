package terminal

import (
	"os"
	"strings"
)

// Options contains all terminal-related configuration options
type Options struct {
	// PreferenceOptions for color settings
	PreferenceOptions PreferenceOptions
	// DetectorOptions for interactive detection
	DetectorOptions DetectorOptions
}

// Capabilities provides a unified interface for terminal capability detection
// It integrates InteractiveDetector, ColorDetector, and UserPreference
type Capabilities interface {
	IsInteractive() bool
	SupportsColor() bool
	HasExplicitUserPreference() bool
}

// DefaultCapabilities implements the Capabilities interface by combining
// all the terminal detection components
type DefaultCapabilities struct {
	interactiveDetector InteractiveDetector
	colorDetector       ColorDetector
	userPreference      *UserPreference
}

// NewCapabilities creates a new Capabilities instance with the given options
func NewCapabilities(options Options) Capabilities {
	return &DefaultCapabilities{
		interactiveDetector: NewInteractiveDetector(options.DetectorOptions),
		colorDetector:       NewColorDetector(),
		userPreference:      NewUserPreference(options.PreferenceOptions),
	}
}

// IsInteractive returns true if the current environment should be treated as interactive
func (c *DefaultCapabilities) IsInteractive() bool {
	return c.interactiveDetector.IsInteractive()
}

// SupportsColor returns true if color output should be enabled
// This implements the priority logic specified in the implementation plan:
// 1. Command line arguments (highest priority)
// 2. CLICOLOR_FORCE=1 (overrides other conditions)
// 3. NO_COLOR environment variable
// 4. CLICOLOR environment variable (only applies in interactive mode)
// 5. Terminal capability auto-detection
func (c *DefaultCapabilities) SupportsColor() bool {
	// Check user preferences first (handles priorities 1-3: command line args, CLICOLOR_FORCE, NO_COLOR)
	if c.userPreference.HasExplicitPreference() {
		return c.userPreference.SupportsColor()
	}

	// Check if we are in interactive mode first
	if !c.IsInteractive() || !c.colorDetector.SupportsColor() {
		return false
	}

	// Priority 4: CLICOLOR environment variable (only applies in interactive mode)
	if cliColor := os.Getenv("CLICOLOR"); cliColor != "" {
		return isTruthy(cliColor)
	}

	// Priority 5: Default behavior when interactive and color-capable
	return true
}

// isTruthy checks if a string value should be considered "true"
// Supports: "1", "true", "yes" (case insensitive)
func isTruthy(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	switch lower {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// HasExplicitUserPreference returns true if the user has explicitly set
// a color preference through command line options or environment variables
func (c *DefaultCapabilities) HasExplicitUserPreference() bool {
	return c.userPreference.HasExplicitPreference()
}
