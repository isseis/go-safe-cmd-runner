// Package redaction provides shared functionality across the go-safe-cmd-runner project.
package redaction

import (
	"fmt"
	"regexp"
	"strings"
)

// SensitivePatterns contains compiled patterns for detecting sensitive information
type SensitivePatterns struct {
	// AllowedEnvVars contains environment variable names that are safe to log
	AllowedEnvVars map[string]struct{}
	// Combined patterns for efficient matching - always guaranteed to be non-nil
	combinedCredentialPattern *regexp.Regexp
	combinedEnvVarPattern     *regexp.Regexp
}

// NewSensitivePatterns creates a new SensitivePatterns with given pattern strings
func NewSensitivePatterns(credentialPatterns, envVarPatterns []string, allowedEnvVars map[string]struct{}) (*SensitivePatterns, error) {
	if allowedEnvVars == nil {
		allowedEnvVars = make(map[string]struct{})
	}

	patterns := &SensitivePatterns{
		AllowedEnvVars: allowedEnvVars,
	}

	if err := patterns.buildCombinedPatterns(credentialPatterns, envVarPatterns); err != nil {
		return nil, fmt.Errorf("failed to build combined patterns: %w", err)
	}

	return patterns, nil
}

// DefaultSensitivePatterns returns a default set of sensitive patterns
func DefaultSensitivePatterns() *SensitivePatterns {
	// Common credential patterns for log keys and values
	credentialPatterns := []string{
		`(?i)(password|token|secret|key|api_key)`,
		`(?i)aws_access_key_id`,
		`(?i)aws_secret_access_key`,
		`(?i)aws_session_token`,
		`(?i)google_application_credentials`,
		`(?i)gcp_service_account_key`,
		`(?i)github_token`,
		`(?i)gitlab_token`,
		`(?i)bearer`,
		`(?i)basic`,
		`(?i)authorization`,
	}

	// Environment variable patterns (for config validation)
	envVarPatterns := []string{
		`(?i).*PASSWORD.*`,
		`(?i).*SECRET.*`,
		`(?i).*TOKEN.*`,
		`(?i).*KEY.*`,
		`(?i).*API.*`,
		`(?i).*CREDENTIAL.*`,
		`(?i).*AUTH.*`,
	}

	// Common safe environment variables
	allowedEnvVars := map[string]struct{}{
		"PATH":     {},
		"HOME":     {},
		"USER":     {},
		"LANG":     {},
		"SHELL":    {},
		"TERM":     {},
		"PWD":      {},
		"OLDPWD":   {},
		"HOSTNAME": {},
		"LOGNAME":  {},
		"TZ":       {},
		"DISPLAY":  {},
		"TMPDIR":   {},
		"EDITOR":   {},
		"PAGER":    {},
	}

	// Use constructor to ensure patterns are always properly initialized
	patterns, err := NewSensitivePatterns(credentialPatterns, envVarPatterns, allowedEnvVars)
	if err != nil {
		// This should not happen with our default patterns
		panic(fmt.Sprintf("failed to create default sensitive patterns: %v", err))
	}
	return patterns
}

// buildCombinedPatterns creates optimized combined regular expressions from pattern strings
// This is an internal method and should only be called during construction
func (sp *SensitivePatterns) buildCombinedPatterns(credentialPatterns, envVarPatterns []string) error {
	compiledNeverMatch := regexp.MustCompile("$^")
	sp.combinedCredentialPattern = compiledNeverMatch
	// Combine credential patterns with OR operator
	if len(credentialPatterns) > 0 {
		combinedCredentialPattern := "(" + strings.Join(credentialPatterns, "|") + ")"

		// Compile the combined pattern
		compiled, err := regexp.Compile(combinedCredentialPattern)
		if err != nil {
			return fmt.Errorf("failed to compile combined credential pattern: %w", err)
		}
		sp.combinedCredentialPattern = compiled
	}

	sp.combinedEnvVarPattern = compiledNeverMatch
	// Combine environment variable patterns with OR operator
	if len(envVarPatterns) > 0 {
		combinedEnvVarPattern := "(" + strings.Join(envVarPatterns, "|") + ")"

		// Compile the combined pattern
		compiled, err := regexp.Compile(combinedEnvVarPattern)
		if err != nil {
			return fmt.Errorf("failed to compile combined env var pattern: %w", err)
		}
		sp.combinedEnvVarPattern = compiled
	}

	return nil
}

// IsSensitiveKey checks if a key (e.g., log attribute key) contains sensitive information
func (sp *SensitivePatterns) IsSensitiveKey(key string) bool {
	return sp.combinedCredentialPattern.MatchString(key)
}

// IsSensitiveValue checks if a value contains sensitive information
func (sp *SensitivePatterns) IsSensitiveValue(value string) bool {
	return sp.combinedCredentialPattern.MatchString(value)
}

// IsSensitiveEnvVar checks if an environment variable name is sensitive
func (sp *SensitivePatterns) IsSensitiveEnvVar(name string) bool {
	upperName := strings.ToUpper(name)

	// Check if it's explicitly allowed
	if _, ok := sp.AllowedEnvVars[upperName]; ok {
		return false
	}
	return sp.combinedEnvVarPattern.MatchString(upperName)
}

// DefaultKeyValuePatterns returns default keys for key=value redaction
func DefaultKeyValuePatterns() []string {
	return []string{
		// API keys, tokens, passwords (common patterns)
		"password",
		"token",
		"key",
		"secret",
		"api_key",

		// Environment variable assignments that might contain secrets
		"_PASSWORD",
		"_TOKEN",
		"_KEY",
		"_SECRET",

		// Common credential patterns (will be handled specially)
		"Bearer ",
		"Basic ",
		// Header-style pattern (colon redaction handles both with/without space)
		"Authorization: ",
	}
}
