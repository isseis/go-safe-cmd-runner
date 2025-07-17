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
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// Error definitions
var (
	// ErrInvalidFilePermissions is returned when a regular file has inappropriate permissions
	// (e.g., world-writable, group-writable when not allowed, wrong file type)
	ErrInvalidFilePermissions = errors.New("invalid file permissions")

	// ErrInvalidDirPermissions is returned when a directory has inappropriate permissions
	// (e.g., world-writable, group-writable by non-root, writable by non-root user)
	ErrInvalidDirPermissions = errors.New("invalid directory permissions")

	// ErrUnsafeEnvironmentVar is returned when an environment variable contains
	// potentially dangerous patterns that could lead to command injection
	ErrUnsafeEnvironmentVar = errors.New("unsafe environment variable")

	// ErrCommandNotAllowed is returned when a command does not match any allowed pattern
	// in the security configuration
	ErrCommandNotAllowed = errors.New("command not allowed")

	// ErrInvalidPath is returned for path-related structural issues:
	// - Empty paths
	// - Relative paths (when absolute paths are required)
	// - Paths that exceed maximum length limits
	ErrInvalidPath = errors.New("invalid path")

	// ErrInvalidRegexPattern is returned when a regex pattern in the security configuration
	// cannot be compiled
	ErrInvalidRegexPattern = errors.New("invalid regex pattern")

	// ErrInsecurePathComponent is returned for structural security issues in path components:
	// - Path components that are symbolic links (symlink attack prevention)
	// - Path components that are not directories when they should be
	// - Failed to get system information for path components
	ErrInsecurePathComponent = errors.New("insecure path component")
)

// Constants for security configuration
const (
	// DefaultFilePermissions defines the default maximum allowed permissions for config files (rw-r--r--)
	DefaultFilePermissions = 0o644
	// DefaultDirectoryPermissions defines the default maximum allowed permissions for directories (rwxr-xr-x)
	DefaultDirectoryPermissions = 0o755
	// DefaultMaxPathLength defines the default maximum allowed path length
	DefaultMaxPathLength = 4096
)

// Constants for security configuration
const (
	UIDRoot = 0
	GIDRoot = 0
)

// Config holds security-related configuration
type Config struct {
	// AllowedCommands is a list of allowed command patterns (regex)
	AllowedCommands []string
	// RequiredFilePermissions defines the maximum allowed permissions for config files
	RequiredFilePermissions os.FileMode
	// RequiredDirectoryPermissions defines the maximum allowed permissions for directories
	RequiredDirectoryPermissions os.FileMode
	// SensitiveEnvVars is a list of environment variable patterns that should be sanitized
	SensitiveEnvVars []string
	// MaxPathLength is the maximum allowed path length
	MaxPathLength int
}

// DefaultConfig returns a default security configuration
func DefaultConfig() *Config {
	return &Config{
		AllowedCommands: []string{
			// System commands. The regex pattern is used to match the full path of the command
			// after resolving the path using the PATH environment variable.
			"^/bin/.*",
			"^/usr/bin/.*",
			"^/usr/sbin/.*",
			"^/usr/local/bin/.*",
		},
		RequiredFilePermissions:      DefaultFilePermissions,
		RequiredDirectoryPermissions: DefaultDirectoryPermissions,
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

// validatePathAndGetInfo validates and cleans a path, then returns its file info
func (v *Validator) validatePathAndGetInfo(path, pathType string) (string, os.FileInfo, error) {
	if path == "" {
		slog.Error("Empty " + pathType + " path provided for permission validation")
		return "", nil, fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	if !filepath.IsAbs(path) {
		err := fmt.Errorf("%w: path must be absolute, got relative path: %s", ErrInvalidPath, path)
		slog.Error("Path validation failed", "path", path, "error", err)
		return "", nil, err
	}

	// Clean and validate the path
	cleanPath := filepath.Clean(path)
	slog.Debug("Validating "+pathType+" permissions", "path", cleanPath)

	if len(cleanPath) > v.config.MaxPathLength {
		err := fmt.Errorf("%w: path too long (%d > %d)", ErrInvalidPath, len(cleanPath), v.config.MaxPathLength)
		slog.Error("Path validation failed", "path", cleanPath, "error", err, "max_length", v.config.MaxPathLength)
		return "", nil, err
	}

	// Get file info
	fileInfo, err := v.fs.Lstat(cleanPath)
	if err != nil {
		slog.Error("Failed to get "+pathType+" info", "path", cleanPath, "error", err)
		return "", nil, fmt.Errorf("failed to stat %s: %w", cleanPath, err)
	}

	return cleanPath, fileInfo, nil
}

// ValidateFilePermissions validates that a file has appropriate permissions
func (v *Validator) ValidateFilePermissions(filePath string) error {
	cleanPath, fileInfo, err := v.validatePathAndGetInfo(filePath, "file")
	if err != nil {
		return err
	}

	// Check if it's a regular file
	if !fileInfo.Mode().IsRegular() {
		err := fmt.Errorf("%w: %s is not a regular file", ErrInvalidFilePermissions, cleanPath)
		slog.Warn("Invalid file type", "path", cleanPath, "mode", fileInfo.Mode().String())
		return err
	}

	perm := fileInfo.Mode().Perm()
	requiredPerms := v.config.RequiredFilePermissions
	pathType := "file"

	slog.Debug("Checking "+pathType+" permissions", "path", cleanPath, "current_permissions", fmt.Sprintf("%04o", perm), "max_allowed", fmt.Sprintf("%04o", requiredPerms))

	disallowedBits := perm &^ requiredPerms
	if disallowedBits != 0 {
		err := fmt.Errorf(
			"%w: %s %s has permissions %o with disallowed bits %o, maximum allowed is %o",
			ErrInvalidFilePermissions, pathType, cleanPath, perm, disallowedBits, requiredPerms)

		slog.Warn(
			"Insecure "+pathType+" permissions detected",
			"path", cleanPath,
			"current_permissions", fmt.Sprintf("%04o", perm),
			"disallowed_bits", fmt.Sprintf("%04o", disallowedBits),
			"max_allowed", fmt.Sprintf("%04o", requiredPerms))

		return err
	}

	slog.Debug(pathType+" permissions validated successfully", "path", cleanPath, "permissions", fmt.Sprintf("%04o", perm))
	return nil
}

// ValidateDirectoryPermissions validates that a directory has appropriate permissions
// and checks the complete path from root to target for security
func (v *Validator) ValidateDirectoryPermissions(dirPath string) error {
	cleanDir, dirInfo, err := v.validatePathAndGetInfo(dirPath, "directory")
	if err != nil {
		return err
	}

	// Check if it's a directory
	if !dirInfo.Mode().IsDir() {
		err := fmt.Errorf("%w: %s is not a directory", ErrInvalidDirPermissions, dirPath)
		slog.Warn("Invalid directory type", "path", dirPath, "mode", dirInfo.Mode().String())
		return err
	}

	// SECURITY: Validate complete path from root to target directory
	// This prevents attacks through compromised intermediate directories
	return v.validateCompletePath(cleanDir, dirPath)
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

// validateCompletePath validates the security of the complete path from root to target
// This prevents attacks through compromised intermediate directories
// cleanDir must be absolute and cleaned.
func (v *Validator) validateCompletePath(cleanPath string, originalPath string) error {
	slog.Debug("Validating complete path security", "target_path", originalPath)

	// Validate each directory component from target to root
	for currentPath := cleanPath; ; {
		slog.Debug("Validating path component", "component_path", currentPath)

		info, err := v.fs.Lstat(currentPath)
		if err != nil {
			slog.Error("Failed to stat path component", "path", currentPath, "error", err)
			return fmt.Errorf("failed to stat path component %s: %w", currentPath, err)
		}

		if err := v.validateDirectoryComponentMode(currentPath, info); err != nil {
			return err
		}
		if err := v.validateDirectoryComponentPermissions(currentPath, info); err != nil {
			return err
		}

		// Move to parent directory, or break if we reached root
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			break
		}
		currentPath = parentPath
	}

	slog.Debug("Complete path validation successful", "original_path", originalPath, "final_path", cleanPath)
	return nil
}

// validateDirectoryComponentMode validates that a directory component is a directory and not a symlink
func (v *Validator) validateDirectoryComponentMode(dirPath string, info os.FileInfo) error {
	// Check if the component is not a symlink
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: path component %s is a symlink", ErrInsecurePathComponent, dirPath)
	}

	// Ensure the component is a directory
	if !info.Mode().IsDir() {
		return fmt.Errorf("%w: path component %s is not a directory", ErrInsecurePathComponent, dirPath)
	}
	return nil
}

// validateDirectoryComponentPermissions validates that a directory component has secure permissions
// info parameter should be the FileInfo for the directory at dirPath to avoid redundant filesystem calls
func (v *Validator) validateDirectoryComponentPermissions(dirPath string, info os.FileInfo) error {
	// Get system-level file info for ownership checks
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: failed to get system info for directory %s", ErrInsecurePathComponent, dirPath)
	}

	perm := info.Mode().Perm()

	// Check that other users cannot write (world-writable check)
	if perm&0o002 != 0 {
		slog.Error("Directory writable by others detected",
			"path", dirPath,
			"permissions", fmt.Sprintf("%04o", perm))
		return fmt.Errorf("%w: directory %s is writable by others (%04o)",
			ErrInvalidDirPermissions, dirPath, perm)
	}

	// Check that group cannot write unless owned by root
	if perm&0o020 != 0 {
		slog.Error("Directory has group write permissions",
			"path", dirPath,
			"permissions", fmt.Sprintf("%04o", perm),
			"owner_uid", stat.Uid,
			"owner_gid", stat.Gid)
		// Only allow group write if owned by root (uid=0) and group (gid=0)
		if stat.Uid != UIDRoot || stat.Gid != GIDRoot {
			return fmt.Errorf("%w: directory %s has group write permissions (%04o) but is not owned by root (uid=%d, gid=%d)",
				ErrInvalidDirPermissions, dirPath, perm, stat.Uid, stat.Gid)
		}
	}

	// Check that only root can write to the directory
	if perm&0o200 != 0 && stat.Uid != UIDRoot {
		return fmt.Errorf("%w: directory %s is writable by non-root user (uid=%d)",
			ErrInvalidDirPermissions, dirPath, stat.Uid)
	}

	return nil
}
