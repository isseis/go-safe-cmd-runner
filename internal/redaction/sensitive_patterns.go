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

// PatternKind declares how a KeyValuePattern's Value is to be interpreted. The
// kind is stated by whoever writes the pattern rather than derived from the
// shape of Value, so adding a pattern that happens to contain a space or a colon
// cannot silently change which redaction rule applies to it.
type PatternKind int

const (
	// PatternKindKey marks Value as a key name whose value follows after a
	// separator, e.g. "password" against password=x, "password": "x" and
	// password: x. The separator, the quoting and the extent of the value are
	// interpreted by the redactor, so Value should be the bare key name; a
	// trailing "=" or ":" is stripped rather than matched twice.
	//
	// This is the zero value, so a KeyValuePattern written without an explicit
	// Kind gets the most conservative interpretation.
	PatternKindKey PatternKind = iota
	// PatternKindPrefix marks Value as a literal prefix that is immediately
	// followed by the secret, e.g. "Bearer ", "Basic " or "password=". Value
	// carries its own separator, so everything up to the first whitespace after
	// it is redacted.
	PatternKindPrefix
	// PatternKindHeader marks Value as a header name, e.g. "Authorization". The
	// colon and the surrounding whitespace are supplied by the redactor, and
	// everything from there to the end of the line is redacted (an auth scheme
	// name such as "Bearer" is kept). Value should be the bare header name; a
	// trailing ":" is stripped rather than matched twice.
	PatternKindHeader
)

// KeyValuePattern is one entry of Config.KeyValuePatterns: the literal to look
// for, plus the declaration of how to interpret it.
type KeyValuePattern struct {
	Value string
	Kind  PatternKind
}

// DefaultKeyValuePatterns returns the default patterns for key-name-based redaction
func DefaultKeyValuePatterns() []KeyValuePattern {
	return []KeyValuePattern{
		// API keys, tokens, passwords (common patterns)
		{Value: "password", Kind: PatternKindKey},
		{Value: "token", Kind: PatternKindKey},
		{Value: "key", Kind: PatternKindKey},
		{Value: "secret", Kind: PatternKindKey},
		{Value: "api_key", Kind: PatternKindKey},

		// Environment variable assignments that might contain secrets
		{Value: "_PASSWORD", Kind: PatternKindKey},
		{Value: "_TOKEN", Kind: PatternKindKey},
		{Value: "_KEY", Kind: PatternKindKey},
		{Value: "_SECRET", Kind: PatternKindKey},

		// Auth scheme prefixes, where the secret follows the scheme name directly
		{Value: "Bearer ", Kind: PatternKindPrefix},
		{Value: "Basic ", Kind: PatternKindPrefix},

		// Header name; the colon and its surrounding whitespace are supplied by
		// the header rule, so both "Authorization: x" and "Authorization:x" match
		{Value: "Authorization", Kind: PatternKindHeader},
	}
}
