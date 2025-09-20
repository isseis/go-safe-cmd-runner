package security

import (
	"fmt"
	"regexp"

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

// NewValidator creates a new security validator with the given configuration.
// If config is nil, DefaultConfig() will be used.
// Returns an error if any regex patterns in the config are invalid.
func NewValidator(config *Config) (*Validator, error) {
	return NewValidatorWithFS(config, common.NewDefaultFileSystem())
}

// NewValidatorWithGroupMembership creates a new security validator with group membership support.
// This constructor is specifically for output capture functionality that needs UID/GID permission checks.
func NewValidatorWithGroupMembership(config *Config, groupMembership *groupmembership.GroupMembership) (*Validator, error) {
	return NewValidatorWithFSAndGroupMembership(config, common.NewDefaultFileSystem(), groupMembership)
}

// NewValidatorWithFS creates a new security validator with the given configuration and FileSystem.
// If config is nil, DefaultConfig() will be used.
// Returns an error if any regex patterns in the config are invalid.
func NewValidatorWithFS(config *Config, fs common.FileSystem) (*Validator, error) {
	return NewValidatorWithFSAndGroupMembership(config, fs, nil)
}

// NewValidatorWithFSAndGroupMembership creates a new security validator with all options.
// If config is nil, DefaultConfig() will be used.
// Returns an error if any regex patterns in the config are invalid.
func NewValidatorWithFSAndGroupMembership(config *Config, fs common.FileSystem, groupMembership *groupmembership.GroupMembership) (*Validator, error) {
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

	return v, nil
}
