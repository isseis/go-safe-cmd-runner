// Package security provides security-related functionality for the command runner.
// It includes file permission validation, environment variable sanitization,
// and command whitelist verification.
package security

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// Error definitions
var (
	ErrInvalidFilePermissions = errors.New("invalid file permissions")
	ErrUnsafeEnvironmentVar   = errors.New("unsafe environment variable")
	ErrCommandNotAllowed      = errors.New("command not allowed")
	ErrInvalidPath            = errors.New("invalid path")
	ErrInvalidRegexPattern    = errors.New("invalid regex pattern")
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
	config                *Config
	fs                    common.FileSystem
	allowedCommandRegexps []*regexp.Regexp
	sensitiveEnvRegexps   []*regexp.Regexp
	dangerousEnvRegexps   []*regexp.Regexp
}

// NewValidator creates a new security validator with the given configuration.
// If config is nil, DefaultConfig() will be used.
// Returns an error if any regex patterns in the config are invalid.
func NewValidator(config *Config) (*Validator, error) {
	return NewValidatorWithFS(config, common.NewDefaultFileSystem())
}

// NewValidatorWithFS creates a new security validator with the given configuration and FileSystem.
// If config is nil, DefaultConfig() will be used.
// Returns an error if any regex patterns in the config are invalid.
func NewValidatorWithFS(config *Config, fs common.FileSystem) (*Validator, error) {
	if config == nil {
		config = DefaultConfig()
	}

	v := &Validator{
		config: config,
		fs:     fs,
	}

	// Compile allowed command patterns
	v.allowedCommandRegexps = make([]*regexp.Regexp, len(config.AllowedCommands))
	for i, pattern := range config.AllowedCommands {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid allowed command pattern %q: %w", ErrInvalidRegexPattern, pattern, err)
		}
		v.allowedCommandRegexps[i] = re
	}

	// Compile sensitive environment variable patterns
	v.sensitiveEnvRegexps = make([]*regexp.Regexp, len(config.SensitiveEnvVars))
	for i, pattern := range config.SensitiveEnvVars {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid sensitive env var pattern %q: %w", ErrInvalidRegexPattern, pattern, err)
		}
		v.sensitiveEnvRegexps[i] = re
	}

	// Compile dangerous environment value patterns
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
	v.dangerousEnvRegexps = make([]*regexp.Regexp, len(dangerousPatterns))
	for i, pattern := range dangerousPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid dangerous env pattern %q: %w", ErrInvalidRegexPattern, pattern, err)
		}
		v.dangerousEnvRegexps[i] = re
	}

	return v, nil
}

// ValidateFilePermissions validates that a file has appropriate permissions
func (v *Validator) ValidateFilePermissions(filePath string) error {
	if filePath == "" {
		slog.Error("Empty file path provided for permission validation")
		return fmt.Errorf("%w: empty file path", ErrInvalidPath)
	}

	// Clean and validate the path
	cleanPath := filepath.Clean(filePath)
	slog.Debug("Validating file permissions", "path", cleanPath)

	if len(cleanPath) > v.config.MaxPathLength {
		err := fmt.Errorf("%w: path too long (%d > %d)", ErrInvalidPath, len(cleanPath), v.config.MaxPathLength)
		slog.Error("Path validation failed", "path", cleanPath, "error", err, "max_length", v.config.MaxPathLength)
		return err
	}

	// Get file info
	fileInfo, err := v.fs.Stat(cleanPath)
	if err != nil {
		slog.Error("Failed to get file info", "path", cleanPath, "error", err)
		return fmt.Errorf("failed to stat file %s: %w", cleanPath, err)
	}

	// Check if it's a regular file
	if !fileInfo.Mode().IsRegular() {
		err := fmt.Errorf("%w: %s is not a regular file", ErrInvalidFilePermissions, cleanPath)
		slog.Warn("Invalid file type", "path", cleanPath, "mode", fileInfo.Mode().String())
		return err
	}

	// Check permissions using bitwise operations to ensure file permissions are a subset of allowed permissions
	perm := fileInfo.Mode().Perm()
	slog.Debug("Checking file permissions", "path", cleanPath, "current_permissions", fmt.Sprintf("%04o", perm), "max_allowed", fmt.Sprintf("%04o", v.config.RequiredFilePermissions))

	// SECURITY: Use bitwise AND with complement to check for disallowed permission bits.
	// This prevents security vulnerabilities where files with dangerous permissions like 0o077 (---rwxrwx)
	// would be incorrectly allowed under a simple numeric comparison (0o077 < 0o644).
	// The bitwise operation ensures that only files with permissions that are a true subset
	// of the allowed permissions are accepted.
	//
	// Example: If RequiredFilePermissions = 0o644 (rw-r--r--):
	// - 0o600 (rw-------): disallowedBits = 0o600 &^ 0o644 = 0o000 ✓ (allowed)
	// - 0o644 (rw-r--r--): disallowedBits = 0o644 &^ 0o644 = 0o000 ✓ (allowed)
	// - 0o777 (rwxrwxrwx): disallowedBits = 0o777 &^ 0o644 = 0o133 ✗ (rejected)
	// - 0o077 (---rwxrwx): disallowedBits = 0o077 &^ 0o644 = 0o033 ✗ (rejected, security fix!)
	disallowedBits := perm &^ v.config.RequiredFilePermissions
	if disallowedBits != 0 {
		err := fmt.Errorf("%w: file %s has permissions %o with disallowed bits %o, maximum allowed is %o",
			ErrInvalidFilePermissions, cleanPath, perm, disallowedBits, v.config.RequiredFilePermissions)
		slog.Warn("Insecure file permissions detected",
			"path", cleanPath,
			"current_permissions", fmt.Sprintf("%04o", perm),
			"disallowed_bits", fmt.Sprintf("%04o", disallowedBits),
			"max_allowed", fmt.Sprintf("%04o", v.config.RequiredFilePermissions))
		return err
	}

	slog.Debug("File permissions validated successfully", "path", cleanPath, "permissions", fmt.Sprintf("%04o", perm))
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

	// Check against compiled allowed command patterns
	for _, re := range v.allowedCommandRegexps {
		if re.MatchString(command) {
			return nil
		}
	}

	return fmt.Errorf("%w: command %s does not match any allowed pattern", ErrCommandNotAllowed, command)
}

// isSensitiveEnvVar checks if an environment variable name matches sensitive patterns
func (v *Validator) isSensitiveEnvVar(name string) bool {
	upperName := strings.ToUpper(name)

	// Check against compiled sensitive environment variable patterns
	for _, re := range v.sensitiveEnvRegexps {
		if re.MatchString(upperName) {
			return true
		}
	}

	return false
}

// ValidateEnvironmentValue validates that an environment variable value is safe
func (v *Validator) ValidateEnvironmentValue(key, value string) error {
	// Check for potential command injection patterns using compiled regexes
	for _, re := range v.dangerousEnvRegexps {
		if re.MatchString(value) {
			return fmt.Errorf("%w: environment variable %s contains potentially dangerous pattern: %s",
				ErrUnsafeEnvironmentVar, key, re.String())
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
