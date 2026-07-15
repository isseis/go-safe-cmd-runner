// Package redaction provides shared redaction functionality.
package redaction

import (
	"regexp"
	"strings"
)

// valueDetectorPatterns holds compiled regex patterns for value-based secret detection.
// Patterns are compiled once at package initialization and cached to avoid repeated
// allocation during redaction of long command outputs.
var valueDetectorPatterns = struct {
	awsKeyID    *regexp.Regexp // AWS access key IDs: AKIA, ASIA, etc.
	githubToken *regexp.Regexp // GitHub tokens: ghp_, gho_, ghs_, etc.
	slackToken  *regexp.Regexp // Slack tokens: xoxb-, xoxp-, xoxa-, xoxr-, etc.
	gcpSAKey    *regexp.Regexp // GCP service account key ID field; group 1 is the "private_key_id":" prefix, group 2 is the closing quote (see doc comment on gcpSAKey below)
	pemPrivate  *regexp.Regexp // PEM private key blocks: -----BEGIN ... PRIVATE KEY-----
	bearerToken *regexp.Regexp // Bearer tokens: standard OAuth pattern; group 1 is the "Bearer " prefix
	urlCred     *regexp.Regexp // URL-embedded credentials: scheme://user:pass@host; group 1 is "scheme://"
}{
	awsKeyID:    regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b|\bASIA[0-9A-Z]{16}\b`),
	githubToken: regexp.MustCompile(`\bgh[pors]_\s*[A-Za-z0-9_]{36,}\b`),
	slackToken:  regexp.MustCompile(`\bxox[bpar]-[0-9]{10,}-[0-9]{10,}-[a-zA-Z0-9]+\b`),
	// NOTE: unlike the other patterns in this set, this one is NOT independent of key
	// context - it anchors on the literal JSON field name "private_key_id" because a
	// GCP service-account key ID has no self-identifying value format (it is an opaque
	// hex fingerprint, indistinguishable from any other hex string by value alone). It
	// is also not itself secret material: the actual credential in a GCP service-account
	// JSON key file is the "private_key" PEM block, which is already caught independent
	// of key name by pemPrivate below. This pattern is kept as defense-in-depth (it masks
	// the fingerprint too) but does not by itself satisfy "detection independent of key
	// name" for the GCP category; see docs/user/security-risk-assessment.md Limitations.
	gcpSAKey:    regexp.MustCompile(`("private_key_id"\s*:\s*")[a-fA-F0-9]{32,}(")`),
	pemPrivate:  regexp.MustCompile(`(?s)-----BEGIN\s[A-Z\s]*PRIVATE\sKEY-----.*?-----END\s[A-Z\s]*PRIVATE\sKEY-----`),
	bearerToken: regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9\-._~+/]+=*`),
	urlCred:     regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+\-.]*://)[^/?:]+:[^/@?]+@`),
}

// ValueDetector detects and masks sensitive values in text based on value format,
// independent of key names. It complements the existing key-name-based detection
// in SensitivePatterns by catching secrets that appear without a recognizable key.
type ValueDetector struct {
	placeholder string
}

// NewValueDetector creates a ValueDetector that masks detected values with
// the given placeholder string (e.g., "[REDACTED]").
func NewValueDetector(placeholder string) *ValueDetector {
	return &ValueDetector{placeholder: placeholder}
}

// Mask scans text for known sensitive value formats and replaces matched
// portions with the detector's placeholder. It returns the original text
// unchanged if no patterns match.
func (d *ValueDetector) Mask(text string) string {
	if text == "" {
		return text
	}

	// ReplaceAllString treats "$0"/"$1"/etc. in the replacement string as
	// expansions of the match/capture groups. Escape literal "$" characters
	// in the placeholder so a placeholder configured with "$1"-like text is
	// always treated literally instead of re-injecting matched (secret) text.
	escapedPlaceholder := strings.ReplaceAll(d.placeholder, "$", "$$")

	result := text
	result = valueDetectorPatterns.awsKeyID.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.githubToken.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.slackToken.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.pemPrivate.ReplaceAllString(result, escapedPlaceholder)
	// Preserve the "Bearer " prefix, the URL scheme, and the surrounding
	// "private_key_id":"..." JSON structure so masked output stays readable
	// (e.g. "Bearer [REDACTED]" instead of a bare placeholder).
	result = valueDetectorPatterns.gcpSAKey.ReplaceAllString(result, "${1}"+escapedPlaceholder+"${2}")
	result = valueDetectorPatterns.bearerToken.ReplaceAllString(result, "${1}"+escapedPlaceholder)
	result = valueDetectorPatterns.urlCred.ReplaceAllString(result, "${1}"+escapedPlaceholder+"@")

	return result
}
