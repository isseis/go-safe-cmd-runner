// Package redaction provides shared functionality across the go-safe-cmd-runner project.
package redaction

import (
	"fmt"
	"regexp"
	"strings"
)

// SensitivePatterns contains compiled patterns for detecting sensitive information
type SensitivePatterns struct {
	// CredentialPatterns contains regex patterns to match credentials in log keys and values
	CredentialPatterns []*regexp.Regexp
	// EnvVarPatterns contains regex patterns to match sensitive environment variable names
	EnvVarPatterns []*regexp.Regexp
	// AllowedEnvVars contains environment variable names that are safe to log
	AllowedEnvVars map[string]bool
	// Combined patterns for efficient matching
	combinedCredentialPattern *regexp.Regexp
	combinedEnvVarPattern     *regexp.Regexp
}

// DefaultSensitivePatterns returns a default set of sensitive patterns
func DefaultSensitivePatterns() *SensitivePatterns {
	// Common credential patterns for log keys and values
	credentialPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(password|token|secret|key|api_key)`),
		regexp.MustCompile(`(?i)aws_access_key_id`),
		regexp.MustCompile(`(?i)aws_secret_access_key`),
		regexp.MustCompile(`(?i)aws_session_token`),
		regexp.MustCompile(`(?i)google_application_credentials`),
		regexp.MustCompile(`(?i)gcp_service_account_key`),
		regexp.MustCompile(`(?i)github_token`),
		regexp.MustCompile(`(?i)gitlab_token`),
		regexp.MustCompile(`(?i)bearer`),
		regexp.MustCompile(`(?i)basic`),
		regexp.MustCompile(`(?i)authorization`),
	}

	// Environment variable patterns (for config validation)
	envVarPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i).*PASSWORD.*`),
		regexp.MustCompile(`(?i).*SECRET.*`),
		regexp.MustCompile(`(?i).*TOKEN.*`),
		regexp.MustCompile(`(?i).*KEY.*`),
		regexp.MustCompile(`(?i).*API.*`),
		regexp.MustCompile(`(?i).*CREDENTIAL.*`),
		regexp.MustCompile(`(?i).*AUTH.*`),
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
		CredentialPatterns: credentialPatterns,
		EnvVarPatterns:     envVarPatterns,
		AllowedEnvVars:     allowedEnvVars,
	}

	// Build combined patterns for efficiency
	if err := patterns.buildCombinedPatterns(); err != nil {
		// This should not happen with our default patterns, but handle gracefully
		panic(fmt.Sprintf("failed to build combined patterns: %v", err))
	}
	return patterns
}

// buildCombinedPatterns creates optimized combined regular expressions
func (sp *SensitivePatterns) buildCombinedPatterns() error {
	// Combine credential patterns with OR operator
	if len(sp.CredentialPatterns) > 0 {
		var credentialPatternStrings []string
		for _, pattern := range sp.CredentialPatterns {
			// Extract the pattern string (removing any flags)
			patternStr := pattern.String()
			credentialPatternStrings = append(credentialPatternStrings, patternStr)
		}
		combinedCredentialPattern := "(" + strings.Join(credentialPatternStrings, "|") + ")"

		// Compile the combined pattern
		compiled, err := regexp.Compile(combinedCredentialPattern)
		if err != nil {
			return fmt.Errorf("failed to compile combined credential pattern: %w", err)
		}
		sp.combinedCredentialPattern = compiled
	}

	// Combine environment variable patterns with OR operator
	if len(sp.EnvVarPatterns) > 0 {
		var envVarPatternStrings []string
		for _, pattern := range sp.EnvVarPatterns {
			patternStr := pattern.String()
			envVarPatternStrings = append(envVarPatternStrings, patternStr)
		}
		combinedEnvVarPattern := "(" + strings.Join(envVarPatternStrings, "|") + ")"

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
