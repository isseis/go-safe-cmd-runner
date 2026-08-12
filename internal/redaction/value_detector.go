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
	awsKeyID         *regexp.Regexp // AWS access key IDs: AKIA, ASIA, etc.
	githubToken      *regexp.Regexp // GitHub tokens: ghp_, gho_, ghs_, etc.
	slackToken       *regexp.Regexp // Slack tokens: xoxb-, xoxp-, xoxa-, xoxr-, etc.
	gcpSAKey         *regexp.Regexp // GCP service account key ID field; group 1 is the "private_key_id":" prefix, group 2 is the closing quote (see doc comment on gcpSAKey below)
	pemPrivate       *regexp.Regexp // PEM private key blocks: -----BEGIN ... PRIVATE KEY-----
	bearerToken      *regexp.Regexp // Bearer tokens: standard OAuth pattern; group 1 is the "Bearer " prefix
	urlCred          *regexp.Regexp // URL-embedded credentials: scheme://user:pass@host; group 1 is "scheme://"
	githubPAT        *regexp.Regexp // GitHub fine-grained PATs: github_pat_ + 30+ base62 chars
	slackPrefixToken *regexp.Regexp // Slack tokens with the xapp- / xoxe- / xoxs- prefixes
	jwt              *regexp.Regexp // JSON Web Tokens: eyJ + three dot-separated base64url segments; group 1 is the character that ends the token (see doc comment on jwt below)
	slackWebhookURL  *regexp.Regexp // Slack webhook URLs: hooks.slack.com/services/<path>; group 1 is the "https://hooks.slack.com/services/" prefix
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
	// githubPAT matches fine-grained PATs, which start with github_pat_ instead of
	// the classic ghp_/gho_/ghs_ prefixes that githubToken covers. The body is
	// base62 plus underscores; 30 is a lower bound for real tokens (they run to
	// 80+ characters), set high enough that prose like "github_pattern" or
	// "github_pat_" alone does not match.
	githubPAT: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{30,}\b`),
	// slackPrefixToken covers the xapp- (App-level), xoxe- (refresh) and xoxs-
	// prefixes that slackToken's [bpar] character class does not include. It is a
	// separate pattern rather than a widened character class because those tokens
	// do not take the three-segment shape that slackToken requires.
	slackPrefixToken: regexp.MustCompile(`\bx(?:app|oxe|oxs)-[A-Za-z0-9-]{9,}[A-Za-z0-9]`),
	// jwt matches a compact JWT: three base64url segments joined by exactly two
	// dots. The header (eyJ + 7+) and payload (10+) length bounds separate a real
	// JWT from short base64url fragments; the signature is allowed to be empty
	// (an alg=none JWT). The trailing group is required because RE2 has no
	// lookahead: it consumes the single character after the token and requires it
	// to be neither base64url nor a dot, so a third dot (a fourth segment) stops
	// the match instead of letting it silently truncate the token. Mask re-emits
	// that character through "${1}". A deliberate consequence is that a JWT
	// directly followed by a sentence-final period (e.g. "...abc.") is not
	// matched: that is a three-dot string, which 3.3.2 condition 1 excludes.
	// TestValueDetector_JWT pins this as a known limitation.
	jwt: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{7,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]*([^A-Za-z0-9_.-]|$)`),
	// slackWebhookURL treats the /services/ path as the secret. The host is fixed
	// to hooks.slack.com rather than taken from AllowedHost because redaction
	// cannot depend on logging (see 3.3.3). The path class is restricted to URL
	// path characters so a trailing period or comma is not swallowed.
	slackWebhookURL: regexp.MustCompile(`(?i)(\bhttps://hooks\.slack\.com/services/)[A-Za-z0-9/_-]+`),
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

	// The four patterns added in 3.3.1 run after the original seven so existing
	// detection results and their masked appearance are unchanged. The JWT's
	// trailing character is re-emitted, and the webhook URL keeps its
	// /services/ prefix, for the same readability reason as above.
	result = valueDetectorPatterns.githubPAT.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.slackPrefixToken.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.jwt.ReplaceAllString(result, escapedPlaceholder+"${1}")
	result = valueDetectorPatterns.slackWebhookURL.ReplaceAllString(result, "${1}"+escapedPlaceholder)

	return result
}
