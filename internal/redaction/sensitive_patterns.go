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
	AllowedEnvVars map[string]bool
	// Combined patterns for efficient matching
	combinedCredentialPattern *regexp.Regexp
	combinedEnvVarPattern     *regexp.Regexp
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
	allowedEnvVars := map[string]bool{
		"PATH":     true,
		"HOME":     true,
		"USER":     true,
		"LANG":     true,
		"SHELL":    true,
		"TERM":     true,
		"PWD":      true,
		"OLDPWD":   true,
		"HOSTNAME": true,
		"LOGNAME":  true,
		"TZ":       true,
		"DISPLAY":  true,
		"TMPDIR":   true,
		"EDITOR":   true,
		"PAGER":    true,
	}

	patterns := &SensitivePatterns{
		AllowedEnvVars: allowedEnvVars,
	}

	// Build combined patterns for efficiency
	if err := patterns.buildCombinedPatterns(credentialPatterns, envVarPatterns); err != nil {
		// This should not happen with our default patterns, but handle gracefully
		panic(fmt.Sprintf("failed to build combined patterns: %v", err))
	}
	return patterns
}

// buildCombinedPatterns creates optimized combined regular expressions from pattern strings
func (sp *SensitivePatterns) buildCombinedPatterns(credentialPatterns, envVarPatterns []string) error {
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
	if sp.combinedCredentialPattern == nil {
		// This should not happen if buildCombinedPatterns succeeded
		return false
	}
	return sp.combinedCredentialPattern.MatchString(key)
}

// IsSensitiveValue checks if a value contains sensitive information
func (sp *SensitivePatterns) IsSensitiveValue(value string) bool {
	if sp.combinedCredentialPattern == nil {
		// This should not happen if buildCombinedPatterns succeeded
		return false
	}
	return sp.combinedCredentialPattern.MatchString(value)
}

// IsSensitiveEnvVar checks if an environment variable name is sensitive
func (sp *SensitivePatterns) IsSensitiveEnvVar(name string) bool {
	// Check if it's explicitly allowed
	if sp.AllowedEnvVars[strings.ToUpper(name)] {
		return false
	}

	upperName := strings.ToUpper(name)

	if sp.combinedEnvVarPattern == nil {
		// This should not happen if buildCombinedPatterns succeeded
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
