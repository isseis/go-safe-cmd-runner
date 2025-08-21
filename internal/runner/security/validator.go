package security

import (
	"fmt"
	"regexp"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
