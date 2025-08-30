package terminal

import (
	"os"
)

// PreferenceOptions contains command-line options for terminal preferences
type PreferenceOptions struct {
	ForceColor   bool // Force color output regardless of environment
	DisableColor bool // Disable color output regardless of environment
}

// UserPreference manages user color preferences based on environment variables and options
type UserPreference struct {
	options PreferenceOptions
}

// NewUserPreference creates a new UserPreference instance
func NewUserPreference(options PreferenceOptions) *UserPreference {
	return &UserPreference{
		options: options,
	}
}

// SupportsColor returns true if color output should be enabled
func (p *UserPreference) SupportsColor() bool {
	// Priority 1: Command line arguments (highest priority)
	if p.options.ForceColor {
		return true
	}
	if p.options.DisableColor {
		return false
	}

	// Priority 2: CLICOLOR_FORCE=1 (overrides all other conditions)
	if cliColorForce := os.Getenv("CLICOLOR_FORCE"); cliColorForce != "" {
		if isTruthy(cliColorForce) {
			return true
		}
	}

	// Priority 3: NO_COLOR environment variable (any value, even empty)
	if _, exists := os.LookupEnv("NO_COLOR"); exists {
		return false
	}

	// Priority 4: Default behavior (no color)
	// Note: CLICOLOR is now handled in capabilities.go only for interactive mode
	return false
}

// HasExplicitPreference returns true if user has explicitly set a color preference
func (p *UserPreference) HasExplicitPreference() bool {
	// Command line options are explicit preferences
	if p.options.ForceColor || p.options.DisableColor {
		return true
	}

	// CLICOLOR_FORCE=1 is an explicit preference (only when truthy)
	if cliColorForce := os.Getenv("CLICOLOR_FORCE"); cliColorForce != "" {
		if isTruthy(cliColorForce) {
			return true
		}
		// CLICOLOR_FORCE=0 is not considered an explicit preference
	}

	// Any setting of NO_COLOR is explicit (even if empty)
	if _, exists := os.LookupEnv("NO_COLOR"); exists {
		return true
	}

	// NOTE: CLICOLOR is NOT considered an explicit preference
	// CLICOLOR should only apply when in interactive mode (TTY connected)
	// This follows standard Unix behavior where CLICOLOR is ignored for pipes

	return false
}
