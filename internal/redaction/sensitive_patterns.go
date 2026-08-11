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

// PatternKind names what a rule redacts, and so declares which rule a
// KeyValuePattern's Literal selects. The kind is stated by whoever writes the
// pattern rather than derived from the shape of Literal, so adding a pattern
// that happens to contain a space or a colon cannot silently change which rule
// applies to it.
//
// Choosing between them: if Literal names a key, use PatternKindKeyedValue; if
// it names a header, PatternKindHeaderValue. PatternKindNextToken is for the
// remaining case, where Literal is not a name at all but a token the secret
// follows directly - an auth scheme. The kinds are instructions rather than a
// classification of literals, so more than one can be made to fire on the same
// text; the guidance above picks the one that reads the text correctly.
type PatternKind int

const (
	// PatternKindKeyedValue redacts the value that Literal is the key of, e.g.
	// "password" against password=x, password: x and "password": "x". The
	// separator, the quoting and the extent of the value are interpreted by the
	// redactor, so Literal should be the bare key name; a trailing "=" or ":" is
	// stripped rather than matched twice.
	//
	// This is the zero value, so a KeyValuePattern written without an explicit
	// Kind gets the most conservative interpretation.
	PatternKindKeyedValue PatternKind = iota
	// PatternKindNextToken redacts the one token that follows Literal, e.g. the
	// auth schemes "Bearer " and "Basic ". Literal carries its own separator, so
	// it is taken verbatim - nothing is stripped from it - and everything from
	// its end to the first whitespace is redacted.
	//
	// Do not use this for a key name that carries its own separator
	// ("password="). PatternKindKeyedValue reads the same text and more: it
	// recognizes ":" and spaced separators, and it redacts a quoted value to its
	// closing quote, where this rule stops at the first space and leaves the rest
	// of "password=\"a b\"" in the clear.
	PatternKindNextToken
	// PatternKindHeaderValue redacts the value of the header named by Literal,
	// e.g. "Authorization" - everything from the colon to the end of the line, an
	// auth scheme name such as "Bearer" excepted. The colon and the whitespace
	// around it are supplied by the redactor, so Literal should be the bare header
	// name; a trailing ":" is stripped rather than matched twice.
	PatternKindHeaderValue
)

// KeyValuePattern is one entry of Config.KeyValuePatterns: the text to look for,
// plus the declaration of what to redact once it is found.
type KeyValuePattern struct {
	// Literal is the text matched in the input - a key name, an auth scheme or a
	// header name, according to Kind. It is deliberately not called "Value":
	// everywhere else in this package a value is the secret being redacted, which
	// is the opposite of what this field holds.
	Literal string
	Kind    PatternKind
}

// DefaultKeyValuePatterns returns the default patterns for key-name-based redaction
func DefaultKeyValuePatterns() []KeyValuePattern {
	return []KeyValuePattern{
		// API keys, tokens, passwords (common patterns)
		{Literal: "password", Kind: PatternKindKeyedValue},
		{Literal: "token", Kind: PatternKindKeyedValue},
		{Literal: "key", Kind: PatternKindKeyedValue},
		{Literal: "secret", Kind: PatternKindKeyedValue},
		{Literal: "api_key", Kind: PatternKindKeyedValue},

		// Environment variable assignments that might contain secrets
		{Literal: "_PASSWORD", Kind: PatternKindKeyedValue},
		{Literal: "_TOKEN", Kind: PatternKindKeyedValue},
		{Literal: "_KEY", Kind: PatternKindKeyedValue},
		{Literal: "_SECRET", Kind: PatternKindKeyedValue},

		// Auth scheme prefixes, where the secret follows the scheme name directly
		{Literal: "Bearer ", Kind: PatternKindNextToken},
		{Literal: "Basic ", Kind: PatternKindNextToken},

		// Header name; the colon and its surrounding whitespace are supplied by
		// the header-value rule, so both "Authorization: x" and "Authorization:x" match
		{Literal: "Authorization", Kind: PatternKindHeaderValue},
	}
}
