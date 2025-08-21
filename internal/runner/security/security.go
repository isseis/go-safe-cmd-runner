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
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
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

	// ErrSymlinkDepthExceeded is returned when symbolic link resolution exceeds MaxSymlinkDepth
	ErrSymlinkDepthExceeded = errors.New("symbolic link depth exceeded")

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

	// ErrVariableNameEmpty is returned when a variable name is empty
	ErrVariableNameEmpty = errors.New("variable name cannot be empty")

	// ErrVariableNameInvalidStart is returned when a variable name starts with an invalid character
	ErrVariableNameInvalidStart = errors.New("variable name must start with a letter or underscore")

	// ErrVariableNameInvalidChar is returned when a variable name contains an invalid character
	ErrVariableNameInvalidChar = errors.New("variable name contains invalid character")
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

	// Symbolic link resolution limits
	// SYMLOOP_MAX is typically 40 on Linux systems (POSIX.1-2008 minimum: 8)
	// This value matches what Go's filepath.EvalSymlinks uses internally
	// This prevents infinite loops when resolving symbolic links
	MaxSymlinkDepth = 40

	// Logging configuration constants
	DefaultErrorMessageLength = 200  // Reasonable limit for error messages
	DefaultStdoutLength       = 100  // Very limited stdout in logs
	VerboseErrorMessageLength = 1000 // Longer error messages for debugging
	VerboseStdoutLength       = 500  // More stdout for debugging
)

// LoggingOptions controls how sensitive information is handled in logs
type LoggingOptions struct {
	// IncludeErrorDetails controls whether full error messages are logged
	IncludeErrorDetails bool `json:"include_error_details"`

	// MaxErrorMessageLength limits the length of error messages in logs
	MaxErrorMessageLength int `json:"max_error_message_length"`

	// RedactSensitiveInfo enables automatic redaction of potentially sensitive patterns
	RedactSensitiveInfo bool `json:"redact_sensitive_info"`

	// TruncateStdout controls whether stdout is truncated in error logs
	TruncateStdout bool `json:"truncate_stdout"`

	// MaxStdoutLength limits the length of stdout in error logs
	MaxStdoutLength int `json:"max_stdout_length"`
}

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
	// DangerousPrivilegedCommands is a list of potentially dangerous commands when run with privileges
	DangerousPrivilegedCommands []string
	// ShellCommands is a list of shell commands
	ShellCommands []string
	// ShellMetacharacters is a list of shell metacharacters that require careful handling
	ShellMetacharacters []string
	// LoggingOptions controls sensitive information handling in logs
	LoggingOptions LoggingOptions
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
		LoggingOptions: LoggingOptions{
			IncludeErrorDetails:   false,                     // Secure default: don't include full error details
			MaxErrorMessageLength: DefaultErrorMessageLength, // Reasonable limit for error messages
			RedactSensitiveInfo:   true,                      // Enable automatic redaction
			TruncateStdout:        true,                      // Truncate stdout for security
			MaxStdoutLength:       DefaultStdoutLength,       // Very limited stdout in logs
		},
		DangerousPrivilegedCommands: []string{
			// Shell executables
			"/bin/sh", "/bin/bash", "/usr/bin/sh", "/usr/bin/bash",
			"/bin/zsh", "/usr/bin/zsh", "/bin/csh", "/usr/bin/csh",

			// Privilege escalation tools
			"/bin/su", "/usr/bin/su", "/usr/bin/sudo", "/usr/bin/doas",

			// System administration tools that require careful use
			"/sbin/init", "/usr/sbin/init",
			"/bin/rm", "/usr/bin/rm", // without argument validation
			"/bin/dd", "/usr/bin/dd", // can be destructive
			"/bin/mount", "/usr/bin/mount",
			"/bin/umount", "/usr/bin/umount",

			// Package management
			"/usr/bin/apt", "/usr/bin/apt-get",
			"/usr/bin/yum", "/usr/bin/dnf", "/usr/bin/rpm",

			// Service management
			"/bin/systemctl", "/usr/bin/systemctl",
			"/sbin/service", "/usr/sbin/service",
		},
		ShellCommands: []string{
			"/bin/sh", "/bin/bash", "/usr/bin/sh", "/usr/bin/bash",
			"/bin/zsh", "/usr/bin/zsh", "/bin/csh", "/usr/bin/csh",
			"/bin/fish", "/usr/bin/fish",
			"/bin/dash", "/usr/bin/dash",
		},
		ShellMetacharacters: []string{
			";", "&", "|", "&&", "||",
			"$", "`", "$(", "${",
			">", "<", ">>", "<<",
			"*", "?", "[", "]",
			"~", "!",
		},
	}
}

// DefaultLoggingOptions returns secure default logging options
func DefaultLoggingOptions() LoggingOptions {
	return LoggingOptions{
		IncludeErrorDetails:   false,                     // Secure default: don't include full error details
		MaxErrorMessageLength: DefaultErrorMessageLength, // Reasonable limit for error messages
		RedactSensitiveInfo:   true,                      // Enable automatic redaction
		TruncateStdout:        true,                      // Truncate stdout for security
		MaxStdoutLength:       DefaultStdoutLength,       // Very limited stdout in logs
	}
}

// VerboseLoggingOptions returns options suitable for debugging (less secure)
func VerboseLoggingOptions() LoggingOptions {
	return LoggingOptions{
		IncludeErrorDetails:   true,                      // Include full error details for debugging
		MaxErrorMessageLength: VerboseErrorMessageLength, // Longer error messages
		RedactSensitiveInfo:   true,                      // Still redact sensitive patterns
		TruncateStdout:        true,                      // Still truncate stdout
		MaxStdoutLength:       VerboseStdoutLength,       // More stdout for debugging
	}
}

// Validator provides security validation functionality
type Validator struct {
	config                      *Config
	fs                          common.FileSystem
	allowedCommandRegexps       []*regexp.Regexp
	sensitiveEnvRegexps         []*regexp.Regexp
	dangerousEnvRegexps         []*regexp.Regexp
	dangerousPrivilegedCommands map[string]struct{}
	shellCommands               map[string]struct{}
	// Common redaction functionality
	redactionConfig   *redaction.Config
	sensitivePatterns *redaction.SensitivePatterns
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

	// Initialize common redaction functionality
	sensitivePatterns := redaction.DefaultSensitivePatterns()
	redactionConfig := redaction.DefaultConfig()

	v := &Validator{
		config:            config,
		fs:                fs,
		sensitivePatterns: sensitivePatterns,
		redactionConfig:   redactionConfig,
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
		`;`,        // Command separator
		`\|`,       // Pipe
		`&&`,       // AND operator
		`\|\|`,     // OR operator
		`\$\(`,     // Command substitution
		"`",        // Command substitution (backticks)
		`>`,        // Redirect
		`<`,        // Redirect
		`rm `,      // Dangerous rm command
		`del `,     // Dangerous del command
		`format `,  // Dangerous format command
		`mkfs `,    // Dangerous mkfs command
		`mkfs\.`,   // Dangerous mkfs. command
		`dd if=`,   // Dangerous dd input
		`dd of=`,   // Dangerous dd output
		`exec `,    // Code execution
		`exec\(`,   // Code execution (function call)
		`system `,  // System call
		`system\(`, // System call (function call)
		`eval `,    // Code evaluation
		`eval\(`,   // Code evaluation (function call)
	}
	v.dangerousEnvRegexps = make([]*regexp.Regexp, len(dangerousPatterns))
	for i, pattern := range dangerousPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid dangerous env pattern %q: %w", ErrInvalidRegexPattern, pattern, err)
		}
		v.dangerousEnvRegexps[i] = re
	}

	// Initialize dangerous commands map
	v.dangerousPrivilegedCommands = make(map[string]struct{})
	for _, cmd := range config.DangerousPrivilegedCommands {
		v.dangerousPrivilegedCommands[cmd] = struct{}{}
	}

	// Initialize shell commands map
	v.shellCommands = make(map[string]struct{})
	for _, cmd := range config.ShellCommands {
		v.shellCommands[cmd] = struct{}{}
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
	// Use the new common functionality first
	if v.sensitivePatterns.IsSensitiveEnvVar(name) {
		return true
	}

	// Fallback to the legacy regex patterns for backward compatibility
	upperName := strings.ToUpper(name)
	for _, re := range v.sensitiveEnvRegexps {
		if re.MatchString(upperName) {
			return true
		}
	}

	return false
}

// SanitizeErrorForLogging sanitizes an error message for safe logging
func (v *Validator) SanitizeErrorForLogging(err error) string {
	if err == nil {
		return ""
	}

	errMsg := err.Error()

	// If error details should not be included, return a generic message
	if !v.config.LoggingOptions.IncludeErrorDetails {
		return "[error details redacted for security]"
	}

	// Redact sensitive information if enabled
	if v.config.LoggingOptions.RedactSensitiveInfo {
		errMsg = v.redactSensitivePatterns(errMsg)
	}

	// Truncate if too long
	if v.config.LoggingOptions.MaxErrorMessageLength > 0 && len(errMsg) > v.config.LoggingOptions.MaxErrorMessageLength {
		errMsg = errMsg[:v.config.LoggingOptions.MaxErrorMessageLength] + "...[truncated]"
	}

	return errMsg
}

// SanitizeOutputForLogging sanitizes command output for safe logging
func (v *Validator) SanitizeOutputForLogging(output string) string {
	if output == "" {
		return ""
	}

	// Redact sensitive information if enabled
	if v.config.LoggingOptions.RedactSensitiveInfo {
		output = v.redactSensitivePatterns(output)
	}

	// Truncate stdout if configured
	if v.config.LoggingOptions.TruncateStdout && v.config.LoggingOptions.MaxStdoutLength > 0 && len(output) > v.config.LoggingOptions.MaxStdoutLength {
		output = output[:v.config.LoggingOptions.MaxStdoutLength] + "...[truncated for security]"
	}

	return output
}

// redactSensitivePatterns removes or redacts potentially sensitive information
func (v *Validator) redactSensitivePatterns(text string) string {
	// Use the new common redaction functionality
	return v.redactionConfig.RedactText(text)
}

// CreateSafeLogFields creates log fields with sensitive data redaction
func (v *Validator) CreateSafeLogFields(fields map[string]any) map[string]any {
	if !v.config.LoggingOptions.RedactSensitiveInfo {
		return fields
	}

	safeFields := make(map[string]any)
	for k, value := range fields {
		switch val := value.(type) {
		case string:
			safeFields[k] = v.SanitizeOutputForLogging(val)
		case error:
			safeFields[k] = v.SanitizeErrorForLogging(val)
		default:
			// For non-string, non-error types, include as-is
			safeFields[k] = value
		}
	}

	return safeFields
}

// LogFieldsWithError creates safe log fields including a sanitized error
func (v *Validator) LogFieldsWithError(baseFields map[string]any, err error) map[string]any {
	fields := make(map[string]any)

	// Copy base fields with sanitization
	for k, value := range baseFields {
		switch val := value.(type) {
		case string:
			fields[k] = v.SanitizeOutputForLogging(val)
		case error:
			fields[k] = v.SanitizeErrorForLogging(val)
		default:
			fields[k] = value
		}
	}

	// Add sanitized error
	if err != nil {
		fields["error"] = v.SanitizeErrorForLogging(err)
	}

	return fields
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

// ValidateVariableValue validates that a variable value contains no dangerous patterns
// This is a convenience function that wraps ValidateEnvironmentValue for use by other packages
func (v *Validator) ValidateVariableValue(value string) error {
	// Use a dummy key name for the validation since we only care about the value
	return v.ValidateEnvironmentValue("VAR", value)
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

// ValidateVariableName validates that a variable name is safe and well-formed
// This is a global convenience function for validating environment variable names
func ValidateVariableName(name string) error {
	if name == "" {
		return ErrVariableNameEmpty
	}

	// Check first character - must be a letter or underscore
	firstChar := name[0]
	if !isLetterOrUnderscore(firstChar) {
		return ErrVariableNameInvalidStart
	}

	// Check remaining characters - must be letter, digit, or underscore
	for i := 1; i < len(name); i++ {
		char := name[i]
		if !isLetterOrUnderscoreOrDigit(char) {
			return fmt.Errorf("%w: '%c'", ErrVariableNameInvalidChar, char)
		}
	}

	return nil
}

// isLetterOrUnderscore checks if a byte is a letter (A-Z, a-z) or underscore
func isLetterOrUnderscore(char byte) bool {
	return (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') || char == '_'
}

// isLetterOrUnderscoreOrDigit checks if a byte is a letter, digit, or underscore
func isLetterOrUnderscoreOrDigit(char byte) bool {
	return isLetterOrUnderscore(char) || (char >= '0' && char <= '9')
}

// IsVariableValueSafe validates that a variable value contains no dangerous patterns
// This is a global convenience function that creates a default validator to check variable values
func IsVariableValueSafe(value string) error {
	validator, err := NewValidator(nil) // Use default config
	if err != nil {
		return fmt.Errorf("failed to create validator: %w", err)
	}
	return validator.ValidateVariableValue(value)
}

// IsDangerousPrivilegedCommand checks if a command path is potentially dangerous when run with privileges
func (v *Validator) IsDangerousPrivilegedCommand(cmdPath string) bool {
	_, exists := v.dangerousPrivilegedCommands[cmdPath]
	return exists
}

// IsShellCommand checks if a command is a shell command
func (v *Validator) IsShellCommand(cmdPath string) bool {
	_, exists := v.shellCommands[cmdPath]
	return exists
}

// HasShellMetacharacters checks if any argument contains shell metacharacters
func (v *Validator) HasShellMetacharacters(args []string) bool {
	for _, arg := range args {
		for _, meta := range v.config.ShellMetacharacters {
			if strings.Contains(arg, meta) {
				return true
			}
		}
	}
	return false
}

// RiskLevel represents the security risk level of a command pattern
type RiskLevel string

const (
	// RiskLevelNone indicates no security risk
	RiskLevelNone RiskLevel = ""
	// RiskLevelLow indicates low security risk
	RiskLevelLow RiskLevel = "low"
	// RiskLevelMedium indicates medium security risk
	RiskLevelMedium RiskLevel = "medium"
	// RiskLevelHigh indicates high security risk
	RiskLevelHigh RiskLevel = "high"
)

// String returns the string representation of the risk level
func (r RiskLevel) String() string {
	return string(r)
}

// IsValid checks if the risk level is valid
func (r RiskLevel) IsValid() bool {
	switch r {
	case RiskLevelNone, RiskLevelLow, RiskLevelMedium, RiskLevelHigh:
		return true
	default:
		return false
	}
}

// DangerousCommandPattern represents a dangerous command pattern with its risk level
type DangerousCommandPattern struct {
	Pattern   []string // Full command pattern including command name and arguments
	RiskLevel RiskLevel
	Reason    string
}

// Pre-sorted patterns by risk level for efficient lookup
var (
	highRiskPatterns   []DangerousCommandPattern
	mediumRiskPatterns []DangerousCommandPattern
)

// init initializes the pre-sorted pattern lists for efficient lookup
func init() {
	patterns := GetDangerousCommandPatterns()
	for _, p := range patterns {
		switch p.RiskLevel {
		case RiskLevelHigh:
			highRiskPatterns = append(highRiskPatterns, p)
		case RiskLevelMedium:
			mediumRiskPatterns = append(mediumRiskPatterns, p)
		case RiskLevelLow, RiskLevelNone:
			// Skip low and none risk patterns as they don't need checking
			continue
		default:
			// Skip invalid risk levels
			continue
		}
	}
}

// GetDangerousCommandPatterns returns a list of dangerous command patterns for security analysis
func GetDangerousCommandPatterns() []DangerousCommandPattern {
	return []DangerousCommandPattern{
		// File system destruction
		{[]string{"rm", "-rf"}, RiskLevelHigh, "Recursive file removal"},
		{[]string{"sudo", "rm"}, RiskLevelHigh, "Privileged file removal"},
		{[]string{"format"}, RiskLevelHigh, "Disk formatting"},
		{[]string{"mkfs"}, RiskLevelHigh, "File system creation"},
		{[]string{"fdisk"}, RiskLevelHigh, "Disk partitioning"},

		// Data manipulation
		{[]string{"dd", "if="}, RiskLevelHigh, "Low-level disk operations"},
		{[]string{"chmod", "777"}, RiskLevelMedium, "Overly permissive file permissions"},
		{[]string{"chown", "root"}, RiskLevelMedium, "Ownership change to root"},

		// Network operations
		{[]string{"wget"}, RiskLevelMedium, "File download"},
		{[]string{"curl"}, RiskLevelMedium, "Network request"},
		{[]string{"nc", "-"}, RiskLevelMedium, "Network connection"},
		{[]string{"netcat"}, RiskLevelMedium, "Network connection"},
	}
}

// checkCommandPatterns checks if a command matches any patterns in the given list
func checkCommandPatterns(cmdName string, cmdArgs []string, patterns []DangerousCommandPattern) (RiskLevel, string, string) {
	for _, pattern := range patterns {
		if matchesPattern(cmdName, cmdArgs, pattern.Pattern) {
			displayPattern := strings.Join(pattern.Pattern, " ")
			return pattern.RiskLevel, displayPattern, pattern.Reason
		}
	}
	return RiskLevelNone, "", ""
}

// IsSudoCommand checks if the given command is sudo, considering symbolic links
// Returns (isSudo, error) where error indicates if symlink depth was exceeded
func IsSudoCommand(cmdName string) (bool, error) {
	commandNames, exceededDepth := extractAllCommandNames(cmdName)
	if exceededDepth {
		return false, ErrSymlinkDepthExceeded
	}
	_, isSudo := commandNames["sudo"]
	return isSudo, nil
}

// AnalyzeCommandSecurity analyzes a command with its arguments for dangerous patterns
func AnalyzeCommandSecurity(cmdName string, args []string) (riskLevel RiskLevel, detectedPattern string, reason string) {
	// First, check if symlink depth is exceeded (highest priority security concern)
	if _, exceededDepth := extractAllCommandNames(cmdName); exceededDepth {
		return RiskLevelHigh, cmdName, "Symbolic link depth exceeds security limit (potential symlink attack)"
	}

	// Check high risk patterns
	if riskLevel, pattern, reason := checkCommandPatterns(cmdName, args, highRiskPatterns); riskLevel != RiskLevelNone {
		return riskLevel, pattern, reason
	}

	// Then check medium risk patterns
	if riskLevel, pattern, reason := checkCommandPatterns(cmdName, args, mediumRiskPatterns); riskLevel != RiskLevelNone {
		return riskLevel, pattern, reason
	}

	return RiskLevelNone, "", ""
}

// extractAllCommandNames extracts all possible command names for matching:
// 1. The original command name (could be full path or just filename)
// 2. Just the base filename from the original command
// 3. All symbolic link names in the chain (if any)
// 4. The final target filename after resolving all symbolic links
// Returns a map for O(1) lookup performance and a boolean indicating if symlink depth was exceeded.
func extractAllCommandNames(cmdName string) (map[string]struct{}, bool) {
	// Handle error case: empty command name (programming error or TOML file mistake)
	if cmdName == "" {
		return make(map[string]struct{}), false
	}

	seen := make(map[string]struct{})

	// Add original command name
	seen[cmdName] = struct{}{}

	// Add base filename (no-op if cmdName is already just a filename)
	seen[filepath.Base(cmdName)] = struct{}{}

	// Resolve symbolic links iteratively to handle multi-level links
	current := cmdName
	exceededDepth := false

	for depth := range MaxSymlinkDepth {
		// Check if current path is a symbolic link
		fileInfo, err := os.Lstat(current)
		if err != nil {
			// If we can't stat the file, stop here
			break
		}

		// If it's not a symbolic link, we're done
		if fileInfo.Mode()&os.ModeSymlink == 0 {
			break
		}

		// If we're at the last iteration and still have a symlink, we exceeded the limit
		if depth == MaxSymlinkDepth-1 {
			exceededDepth = true
			break
		}

		// Resolve the symbolic link
		target, err := os.Readlink(current)
		if err != nil {
			break
		}

		// If target is relative, make it relative to the current directory
		if !filepath.IsAbs(target) {
			current = filepath.Join(filepath.Dir(current), target)
		} else {
			current = target
		}

		// Add the target name (both full path and base name)
		seen[current] = struct{}{}
		seen[filepath.Base(current)] = struct{}{}
	}

	return seen, exceededDepth
}

// matchesPattern checks if the command matches the dangerous pattern.
//
// Pattern matching rules:
//  1. Empty commands are invalid (programming error) and always return false.
//  2. Empty patterns match all valid commands.
//  3. Command names (index 0): Matches against filename only, supporting full paths and symbolic links.
//  4. Argument matching is order-independent.
//  5. Argument count matching: Subset matching (command can have more arguments than pattern).
//  6. Argument patterns ending with "=": Use prefix matching (e.g., "if="
//     matches "if=/dev/zero").
//  7. Other arguments: Require exact string match.
func matchesPattern(cmdName string, cmdArgs []string, pattern []string) bool {
	// If command itself is empty, it's a programming error that should be caught early
	if cmdName == "" {
		return false
	}

	// Empty pattern never matches any command
	if len(pattern) == 0 {
		return false
	}

	// Extract all possible command names (original, base filename, symlink targets)
	commandNames, _ := extractAllCommandNames(cmdName)

	// Check if any of the extracted command names match the pattern[0]
	if _, exists := commandNames[pattern[0]]; !exists {
		return false
	}

	patternArgs := pattern[1:]

	// Default: subset match, require command to have at least as many args as pattern
	if len(cmdArgs) < len(patternArgs) {
		return false
	}

	// Order-independent matching with one-time use of command args
	matchedCommandArgs := make([]bool, len(cmdArgs))
	for _, patternArg := range patternArgs {
		foundMatch := false

		// Prefix pattern when ending with '=' (e.g., "if=")
		if strings.HasSuffix(patternArg, "=") {
			for i, commandArg := range cmdArgs {
				if matchedCommandArgs[i] {
					continue
				}
				if strings.HasPrefix(commandArg, patternArg) {
					matchedCommandArgs[i] = true
					foundMatch = true
					break
				}
			}
		} else {
			// Exact match
			for i, commandArg := range cmdArgs {
				if matchedCommandArgs[i] {
					continue
				}
				if commandArg == patternArg {
					matchedCommandArgs[i] = true
					foundMatch = true
					break
				}
			}
		}

		if !foundMatch {
			return false
		}
	}

	return true
}
