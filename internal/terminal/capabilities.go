package terminal

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
// 4. CLICOLOR environment variable
// 5. Terminal capability auto-detection
func (c *DefaultCapabilities) SupportsColor() bool {
	// Check user preferences first (handles priorities 1-4)
	if c.userPreference.HasExplicitPreference() {
		return c.userPreference.SupportsColor()
	}

	// If no explicit user preference, fall back to terminal capability detection
	// Only enable color if both interactive and terminal supports color
	return c.IsInteractive() && c.colorDetector.SupportsColor()
}

// HasExplicitUserPreference returns true if the user has explicitly set
// a color preference through command line options or environment variables
func (c *DefaultCapabilities) HasExplicitUserPreference() bool {
	return c.userPreference.HasExplicitPreference()
}
