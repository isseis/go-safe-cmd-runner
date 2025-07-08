// Package security provides security-related functionality for the command runner.
// It includes file permission validation, environment variable sanitization,
// and command whitelist verification.
package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Error definitions
var (
	ErrInvalidFilePermissions = errors.New("invalid file permissions")
	ErrUnsafeEnvironmentVar   = errors.New("unsafe environment variable")
	ErrCommandNotAllowed      = errors.New("command not allowed")
	ErrInvalidPath            = errors.New("invalid path")
)

// Constants for security configuration
const (
	// DefaultFilePermissions defines the default maximum allowed permissions for config files (rw-r--r--)
	DefaultFilePermissions = 0o644
	// DefaultMaxPathLength defines the default maximum allowed path length
	DefaultMaxPathLength = 4096
)

// Config holds security-related configuration
type Config struct {
	// AllowedCommands is a list of allowed command patterns (regex)
	AllowedCommands []string
	// RequiredFilePermissions defines the maximum allowed permissions for config files
	RequiredFilePermissions os.FileMode
	// SensitiveEnvVars is a list of environment variable patterns that should be sanitized
	SensitiveEnvVars []string
	// MaxPathLength is the maximum allowed path length
	MaxPathLength int
}

// DefaultConfig returns a default security configuration
func DefaultConfig() *Config {
	return &Config{
		AllowedCommands: []string{
			// System commands
			"^/bin/.*",
			"^/usr/bin/.*",
			"^/usr/local/bin/.*",
			// Common commands without full path
			"^echo$",
			"^cat$",
			"^ls$",
			"^pwd$",
			"^whoami$",
			"^date$",
			"^sleep$",
			"^true$",
			"^false$",
		},
		RequiredFilePermissions: DefaultFilePermissions,
		SensitiveEnvVars: []string{
			".*PASSWORD.*",
			".*SECRET.*",
			".*TOKEN.*",
			".*KEY.*",
			".*API.*",
		},
		MaxPathLength: DefaultMaxPathLength,
	}
}

// Validator provides security validation functionality
type Validator struct {
	config *Config
}

// NewValidator creates a new security validator with the given configuration.
// If config is nil, DefaultConfig() will be used.
func NewValidator(config *Config) *Validator {
	if config == nil {
		config = DefaultConfig()
	}
	return &Validator{
		config: config,
	}
}

// ValidateFilePermissions validates that a file has appropriate permissions
func (v *Validator) ValidateFilePermissions(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("%w: empty file path", ErrInvalidPath)
	}

	// Clean and validate the path
	cleanPath := filepath.Clean(filePath)
	if len(cleanPath) > v.config.MaxPathLength {
		return fmt.Errorf("%w: path too long (%d > %d)", ErrInvalidPath, len(cleanPath), v.config.MaxPathLength)
	}

	// Get file info
	fileInfo, err := os.Stat(cleanPath)
	if err != nil {
		return fmt.Errorf("failed to stat file %s: %w", cleanPath, err)
	}

	// Check if it's a regular file
	if !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrInvalidFilePermissions, cleanPath)
	}

	// Check permissions
	perm := fileInfo.Mode().Perm()
	if perm > v.config.RequiredFilePermissions {
		return fmt.Errorf("%w: file %s has permissions %o, maximum allowed is %o",
			ErrInvalidFilePermissions, cleanPath, perm, v.config.RequiredFilePermissions)
	}

	return nil
}

// SanitizeEnvironmentVariables removes or sanitizes sensitive environment variables
func (v *Validator) SanitizeEnvironmentVariables(envVars map[string]string) map[string]string {
	if envVars == nil {
		return make(map[string]string)
	}

	sanitized := make(map[string]string)

	for key, value := range envVars {
		if v.isSensitiveEnvVar(key) {
			// Replace sensitive values with a placeholder
			sanitized[key] = "[REDACTED]"
		} else {
			sanitized[key] = value
		}
	}

	return sanitized
}

// ValidateCommand validates that a command is allowed according to the whitelist
func (v *Validator) ValidateCommand(command string) error {
	if command == "" {
		return fmt.Errorf("%w: empty command", ErrCommandNotAllowed)
	}

	// Check against allowed command patterns
	for _, pattern := range v.config.AllowedCommands {
		matched, err := regexp.MatchString(pattern, command)
		if err != nil {
			// Log the error but continue checking other patterns
			continue
		}
		if matched {
			return nil
		}
	}

	return fmt.Errorf("%w: command %s does not match any allowed pattern", ErrCommandNotAllowed, command)
}

// isSensitiveEnvVar checks if an environment variable name matches sensitive patterns
func (v *Validator) isSensitiveEnvVar(name string) bool {
	upperName := strings.ToUpper(name)

	for _, pattern := range v.config.SensitiveEnvVars {
		matched, err := regexp.MatchString(pattern, upperName)
		if err != nil {
			continue
		}
		if matched {
			return true
		}
	}

	return false
}

// ValidateEnvironmentValue validates that an environment variable value is safe
func (v *Validator) ValidateEnvironmentValue(key, value string) error {
	// Check for potential command injection patterns
	dangerousPatterns := []string{
		`;`,    // Command separator
		`\|`,   // Pipe
		`&&`,   // AND operator
		`\|\|`, // OR operator
		`\$\(`, // Command substitution
		"`",    // Command substitution (backticks)
		`>`,    // Redirect
		`<`,    // Redirect
	}

	for _, pattern := range dangerousPatterns {
		matched, err := regexp.MatchString(pattern, value)
		if err != nil {
			continue
		}
		if matched {
			return fmt.Errorf("%w: environment variable %s contains potentially dangerous pattern: %s",
				ErrUnsafeEnvironmentVar, key, pattern)
		}
	}

	return nil
}

// ValidateAllEnvironmentVars validates all environment variables for safety
func (v *Validator) ValidateAllEnvironmentVars(envVars map[string]string) error {
	for key, value := range envVars {
		if err := v.ValidateEnvironmentValue(key, value); err != nil {
			return err
		}
	}
	return nil
}
