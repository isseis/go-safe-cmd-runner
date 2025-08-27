// Package terminal provides helpers for detecting terminal capabilities and
// determining whether the current process should be treated as interactive
// or running in a CI/non-interactive environment.
package terminal

import (
	"os"
	"strings"

	"golang.org/x/term"
)

// ciEnvVars contains common CI environment variables
var ciEnvVars = []string{
	"CI",                     // Generic CI indicator
	"CONTINUOUS_INTEGRATION", // Generic CI indicator
	"GITHUB_ACTIONS",         // GitHub Actions
	"TRAVIS",                 // Travis CI
	"CIRCLECI",               // Circle CI
	"JENKINS_URL",            // Jenkins
	"BUILD_NUMBER",           // Jenkins/TeamCity/etc
	"GITLAB_CI",              // GitLab CI
	"APPVEYOR",               // AppVeyor
	"BUILDKITE",              // Buildkite
	"DRONE",                  // Drone CI
	"TF_BUILD",               // Azure DevOps
}

// DetectorOptions contains options for controlling interactive detection
type DetectorOptions struct {
	ForceInteractive    bool // Force interactive mode regardless of environment
	ForceNonInteractive bool // Force non-interactive mode regardless of environment
}

// InteractiveDetector interface defines methods for detecting interactive terminal capabilities
type InteractiveDetector interface {
	IsInteractive() bool
	IsTerminal() bool
	IsCIEnvironment() bool
}

// DefaultInteractiveDetector implements InteractiveDetector
type DefaultInteractiveDetector struct {
	options DetectorOptions
}

// NewInteractiveDetector creates a new interactive detector with the given options
func NewInteractiveDetector(options DetectorOptions) InteractiveDetector {
	return &DefaultInteractiveDetector{
		options: options,
	}
}

// IsInteractive returns true if the current environment is interactive
func (d *DefaultInteractiveDetector) IsInteractive() bool {
	// Priority 1: Command line options (highest priority)
	if d.options.ForceInteractive {
		return true
	}
	if d.options.ForceNonInteractive {
		return false
	}

	// Priority 2: CI environment detection
	if d.IsCIEnvironment() {
		return false
	}

	// Priority 3: Terminal detection
	return d.IsTerminal()
}

// IsTerminal checks if stdout and stderr are connected to a terminal
func (d *DefaultInteractiveDetector) IsTerminal() bool {
	// Check if both stdout and stderr are connected to a terminal
	// This uses golang.org/x/term.IsTerminal() which is reliable on Unix systems
	return term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// IsCIEnvironment checks if the current environment is a CI/CD system
func (d *DefaultInteractiveDetector) IsCIEnvironment() bool {
	for _, envVar := range ciEnvVars {
		if value := os.Getenv(envVar); value != "" {
			// Special handling for CI variable - should be truthy
			if envVar == "CI" {
				return isCITruthy(value)
			}
			// For other CI variables, presence indicates CI environment
			return true
		}
	}

	return false
}

// isCITruthy checks if a CI environment variable value should be considered "true"
// CI=false or CI=0 should not be considered a CI environment
func isCITruthy(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return lower != "false" && lower != "0" && lower != "no"
}
