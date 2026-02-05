// Package security provides security validation functionality for the command runner.
//
// # Validator Construction
//
// The Validator uses the Functional Options Pattern for flexible configuration.
// This pattern allows you to customize the validator by passing optional configuration
// functions, making it easy to add new options without breaking existing code.
//
// Basic usage:
//
//	validator, err := security.NewValidator(nil)
//	if err != nil {
//	    return err
//	}
//
// With custom file system (useful for testing):
//
//	validator, err := security.NewValidator(config,
//	    security.WithFileSystem(mockFS))
//
// With group membership checker (for permission validation):
//
//	validator, err := security.NewValidator(config,
//	    security.WithGroupMembership(gm))
//
// With multiple options:
//
//	validator, err := security.NewValidator(config,
//	    security.WithFileSystem(mockFS),
//	    security.WithGroupMembership(gm))
//
// # Available Options
//
// The following options are available for customizing the Validator:
//
//   - WithFileSystem(fs common.FileSystem): Use a custom file system implementation (useful for testing)
//   - WithGroupMembership(gm *groupmembership.GroupMembership): Add group membership checking for permission validation
package security

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
)

// Validator provides security validation functionality
type Validator struct {
	config                      *Config
	fs                          common.FileSystem
	allowedCommandRegexps       []*regexp.Regexp
	sensitiveEnvRegexps         []*regexp.Regexp
	dangerousEnvRegexps         []*regexp.Regexp
	dangerousPrivilegedCommands map[string]struct{}
	shellCommands               map[string]struct{}
	// Group membership checker for permission validation
	groupMembership *groupmembership.GroupMembership
	// Common redaction functionality
	redactionConfig   *redaction.Config
	sensitivePatterns *redaction.SensitivePatterns
}

// Option is a function type for configuring Validator instances
type Option func(*validatorOptions)

// validatorOptions holds all configuration options for creating a Validator
type validatorOptions struct {
	fs              common.FileSystem
	groupMembership *groupmembership.GroupMembership
}

// WithFileSystem sets a custom file system for the validator.
// This is primarily used for testing with mock file systems.
func WithFileSystem(fs common.FileSystem) Option {
	return func(opts *validatorOptions) {
		opts.fs = fs
	}
}

// WithGroupMembership sets a group membership checker for permission validation.
// This is used for output capture functionality that needs UID/GID permission checks.
func WithGroupMembership(gm *groupmembership.GroupMembership) Option {
	return func(opts *validatorOptions) {
		opts.groupMembership = gm
	}
}

// NewValidator creates a new security validator with the given configuration and options.
// If config is nil, DefaultConfig() will be used.
// Returns an error if any regex patterns in the config are invalid.
func NewValidator(config *Config, opts ...Option) (*Validator, error) {
	// Apply default options
	options := &validatorOptions{
		fs:              common.NewDefaultFileSystem(),
		groupMembership: nil,
	}

	// Apply provided options
	for _, opt := range opts {
		opt(options)
	}

	return newValidatorCore(config, options.fs, options.groupMembership)
}

// newValidatorCore creates a new security validator with all options.
// If config is nil, DefaultConfig() will be used.
// Returns an error if any regex patterns in the config are invalid.
func newValidatorCore(config *Config, fs common.FileSystem, groupMembership *groupmembership.GroupMembership) (*Validator, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Initialize common redaction functionality
	sensitivePatterns := redaction.DefaultSensitivePatterns()
	redactionConfig := redaction.DefaultConfig()

	v := &Validator{
		config:            config,
		fs:                fs,
		groupMembership:   groupMembership,
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

	// Validate DangerousRootPatterns to ensure exact matching
	if err := validateDangerousRootPatterns(config.DangerousRootPatterns); err != nil {
		return nil, err
	}

	return v, nil
}

// validateDangerousRootPatterns validates that DangerousRootPatterns entries are suitable for exact matching.
// It checks that patterns don't contain path separators or wildcards that would indicate they're meant
// for substring or regex matching rather than exact command name matching.
func validateDangerousRootPatterns(patterns []string) error {
	for _, pattern := range patterns {
		// Check for empty patterns
		if pattern == "" {
			return fmt.Errorf("%w: DangerousRootPatterns contains empty pattern", ErrInvalidRegexPattern)
		}

		// Check for path separators (patterns should be command names only, not paths)
		if strings.Contains(pattern, "/") || strings.Contains(pattern, "\\") {
			return fmt.Errorf("%w: DangerousRootPatterns pattern %q contains path separator (use command name only)", ErrInvalidRegexPattern, pattern)
		}

		// Check for wildcard characters that suggest regex/glob usage
		// Note: dot (.) is allowed as it's valid in command names (e.g., update-rc.d)
		if strings.ContainsAny(pattern, "*?[]{}()^$|+") {
			return fmt.Errorf("%w: DangerousRootPatterns pattern %q contains wildcard/regex characters (exact matching only)", ErrInvalidRegexPattern, pattern)
		}

		// Warn about uppercase patterns (commands are normalized to lowercase)
		if pattern != strings.ToLower(pattern) {
			return fmt.Errorf("%w: DangerousRootPatterns pattern %q contains uppercase (patterns are matched case-insensitively, use lowercase)", ErrInvalidRegexPattern, pattern)
		}

		// Check that pattern is a valid filename (no control characters, etc.)
		if filepath.Base(pattern) != pattern {
			return fmt.Errorf("%w: DangerousRootPatterns pattern %q is not a valid command name", ErrInvalidRegexPattern, pattern)
		}
	}
	return nil
}

// ValidateCommandAllowed checks whether a command path is permitted for execution.
// Validation logic:
//  1. Resolve symlinks to get canonical path (security: prevents symlink bypass attacks)
//  2. If the resolved command matches any AllowedCommands regex pattern -> allowed
//  3. Else if groupCmdAllowed map is provided and contains the resolved command path -> allowed
//  4. Otherwise returns *CommandNotAllowedError (wrapping ErrCommandNotAllowed)
//
// Security note: Both global pattern matching and group-level allowlist checks are
// performed against the symlink-resolved canonical path. This prevents attacks where
// a symlink like /usr/bin/safe-looking-name points to a disallowed command.
//
// Parameters:
//   - cmdPath: absolute, symlink-resolved command path (resolved by PathResolver.ResolvePath)
//   - groupCmdAllowed: expanded, normalized, symlink-resolved group-level allowed command map (may be nil or empty)
//
// IMPORTANT: cmdPath is expected to be already symlink-resolved by the caller
// (via verification.PathResolver.ResolvePath()). This ensures TOCTOU safety
// by resolving symlinks once at the start of the execution flow.
//
// Returns:
//   - nil if allowed
//   - error (*CommandNotAllowedError or other structural errors)
func (v *Validator) ValidateCommandAllowed(cmdPath string, groupCmdAllowed map[string]struct{}) error {
	// Basic input validation
	if cmdPath == "" {
		return ErrEmptyCommandPath
	}

	// cmdPath is already symlink-resolved by PathResolver.ResolvePath(),
	// so no need for filepath.EvalSymlinks() here.

	// 1. Global AllowedCommands pattern match (using precompiled regexps)
	// Patterns are matched against the resolved canonical path
	for _, re := range v.allowedCommandRegexps {
		if re.MatchString(cmdPath) {
			return nil
		}
	}

	// 2. Group-level cmd_allowed map check (O(1) lookup)
	// The map already contains symlink-resolved paths, so we compare resolved paths
	if len(groupCmdAllowed) > 0 {
		if _, exists := groupCmdAllowed[cmdPath]; exists {
			return nil
		}
	}

	// 3. Neither global patterns nor group-level map matched -> not allowed
	// Convert map keys to slice and sort for stable error messages
	groupCmdAllowedSlice := make([]string, 0, len(groupCmdAllowed))
	for path := range groupCmdAllowed {
		groupCmdAllowedSlice = append(groupCmdAllowedSlice, path)
	}
	sort.Strings(groupCmdAllowedSlice)
	return &CommandNotAllowedError{
		CommandPath:     cmdPath,
		ResolvedPath:    cmdPath, // Already resolved
		AllowedPatterns: v.config.AllowedCommands,
		GroupCmdAllowed: groupCmdAllowedSlice,
	}
}
