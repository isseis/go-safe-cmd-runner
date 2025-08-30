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
	IsTerminal() bool // Checks for terminal-like environment (TTY or heuristics)
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

// IsTerminal checks if the current environment supports terminal-like interaction.
// This includes both actual TTY connections and terminal-like environments such as
// IDE integrated terminals, Claude Code, and other development environments.
func (d *DefaultInteractiveDetector) IsTerminal() bool {
	// First, check for actual TTY connections on stdout OR stderr
	// Many integrated terminals and IDEs don't connect both streams as TTY
	// but we should still provide interactive output if at least one is available
	stdoutIsTTY := term.IsTerminal(int(os.Stdout.Fd()))
	stderrIsTTY := term.IsTerminal(int(os.Stderr.Fd()))

	// If either output stream is a TTY, we have a real terminal
	if stdoutIsTTY || stderrIsTTY {
		return true
	}

	// Apply heuristics for terminal-like environments that may not have TTY
	// If TERM is set to a meaningful value, likely running in a terminal-like environment
	// This handles cases like IDE integrated terminals, Claude Code, etc.
	termEnv := os.Getenv("TERM")
	if termEnv != "" && termEnv != "dumb" {
		return true
	}

	return false
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
