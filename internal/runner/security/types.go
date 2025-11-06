// Package security provides security-related functionality for the command runner.
// It includes file permission validation, environment variable sanitization,
// and command whitelist verification.
package security

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
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

	// ErrHashValidationFailed is returned when file hash validation fails
	ErrHashValidationFailed = errors.New("hash validation failed")

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

	// ErrNoExistingDirectoryInPathHierarchy is returned when traversing up the
	// directory hierarchy to find an existing directory reaches the filesystem
	// root without finding any existing directory. Use this static error so
	// callers can compare with errors.Is instead of relying on dynamic error
	// strings.
	ErrNoExistingDirectoryInPathHierarchy = errors.New("no existing directory found in path hierarchy")
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
	DefaultErrorMessageLength = 200 // Reasonable limit for error messages
	DefaultStdoutLength       = 100 // Very limited stdout in logs
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
	// DangerousRootPatterns is a list of potentially destructive command names when running as root.
	// These patterns are matched using EXACT command name matching (case-insensitive).
	// Each entry should be a simple command name without paths, wildcards, or regex patterns.
	// Valid examples: "rm", "dd", "mkfs"
	// Invalid examples: "/bin/rm" (contains path), "rm*" (contains wildcard), ".*rm" (regex pattern)
	// The validator will reject configurations with invalid patterns at initialization time.
	DangerousRootPatterns []string
	// DangerousRootArgPatterns is a list of potentially destructive argument patterns when running as root
	DangerousRootArgPatterns []string
	// SystemCriticalPaths is a list of system-critical paths that require extra caution
	// Used for both command argument validation and general path security checks
	// These paths are checked without trailing slashes for flexible matching
	SystemCriticalPaths []string
	// LoggingOptions controls sensitive information handling in logs
	LoggingOptions LoggingOptions
	// outputCriticalPathPatterns defines critical system paths that pose maximum security risk
	OutputCriticalPathPatterns []string
	// outputHighRiskPathPatterns defines high-risk system paths that require extra caution
	OutputHighRiskPathPatterns []string
	// SuspiciousExtensions defines file extensions that pose security risks for output files
	SuspiciousExtensions []string
	// testPermissiveMode is only available in test builds and allows relaxed directory permissions
	testPermissiveMode bool
	// testSkipHashValidation is only available in test builds and allows skipping hash validation
	testSkipHashValidation bool
}

// DangerousCommandPattern represents a dangerous command pattern with its risk level
type DangerousCommandPattern struct {
	Pattern   []string // Full command pattern including command name and arguments
	RiskLevel runnertypes.RiskLevel
	Reason    string
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
			"^/snap/bin/.*",
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
		MaxPathLength:  DefaultMaxPathLength,
		LoggingOptions: DefaultLoggingOptions(),
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
		DangerousRootPatterns: []string{
			"rm", "rmdir", "del", "delete",
			"format", "mkfs", "dd",
			"chmod", "chown", "chgrp",
			"mount", "umount",
			"fdisk", "parted", "gdisk",
		},
		DangerousRootArgPatterns: []string{
			"rf", "force", "recursive", "all",
		},
		SystemCriticalPaths: []string{
			"/", "/bin", "/sbin", "/usr", "/usr/bin", "/usr/sbin", "/etc", "/var",
			"/var/log", "/boot", "/sys", "/proc", "/dev", "/lib", "/lib64", "/root",
		},
		OutputCriticalPathPatterns: []string{
			// Specific critical system files
			"/etc/passwd", "/etc/shadow", "/etc/sudoers",
			// Critical system directories
			"/boot/", "/sys/", "/proc/", "/root/",
			"/etc/", "/usr/bin/", "/usr/sbin/",
			"/bin/", "/sbin/", "/lib/", "/lib64/",
			// SSH and authentication files
			"authorized_keys", "id_rsa", "id_ed25519",
			".ssh/", "private_key", "secret_key",
			// Shell configuration files
			".bashrc", ".zshrc", ".login", ".profile",
			// Security-sensitive application configs
			".gnupg/", ".aws/credentials", ".kube/config", ".docker/config.json",
			// Cryptocurrency and keystore files
			"wallet.dat", "keystore",
		},
		OutputHighRiskPathPatterns: []string{
			// System log directory (less critical than system binaries)
			"/var/log/",
			// Version control and environment files (sensitive but not system-critical)
			".git/", ".env",
		},
		SuspiciousExtensions: []string{
			".exe", ".bat", ".cmd", ".com", ".scr", ".vbs", ".js", ".jar",
			".sh", ".py", ".pl", ".rb", ".php", ".asp", ".jsp",
		},
	}
}

// GetPathPatternsByRisk returns path patterns based on risk level
func (c *Config) GetPathPatternsByRisk(level runnertypes.RiskLevel) []string {
	switch level {
	case runnertypes.RiskLevelCritical:
		return c.OutputCriticalPathPatterns
	case runnertypes.RiskLevelHigh:
		return c.OutputHighRiskPathPatterns
	default:
		return []string{}
	}
}

// GetSuspiciousFilePatterns returns patterns for suspicious files that should be flagged
// This is derived dynamically from OutputCriticalPathPatterns to maintain consistency
func (c *Config) GetSuspiciousFilePatterns() []string {
	patterns := make(map[string]bool)

	// Extract file names from OutputCriticalPathPatterns
	for _, pattern := range c.OutputCriticalPathPatterns {
		// Handle absolute paths like "/etc/passwd" -> "passwd"
		if strings.HasPrefix(pattern, "/") && !strings.HasSuffix(pattern, "/") {
			// Extract basename from absolute paths
			basename := filepath.Base(pattern)
			if basename != "" && basename != "." && basename != "/" {
				patterns[basename] = true
			}
		} else if !strings.HasSuffix(pattern, "/") {
			// Pattern is already a filename or relative path (not ending with "/")
			patterns[pattern] = true
		}
		// Skip directory patterns ending with "/"
	}

	// Convert map to sorted slice for consistent results
	var result []string
	for pattern := range patterns {
		result = append(result, pattern)
	}

	// Sort for consistent ordering in tests
	sort.Strings(result)
	return result
}

// GetSuspiciousExtensions returns file extensions that pose security risks for output files
func (c *Config) GetSuspiciousExtensions() []string {
	return c.SuspiciousExtensions
}

// GetSystemCriticalPaths returns system-critical paths for command argument validation
func (c *Config) GetSystemCriticalPaths() []string {
	return c.SystemCriticalPaths
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
