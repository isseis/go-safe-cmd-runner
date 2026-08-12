// Package redaction provides shared redaction functionality.
package redaction

import (
	"errors"
	"fmt"
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

// ErrInvalidWebhookHost is returned when WithWebhookHost is given something that
// is not a bare hostname or address literal.
var ErrInvalidWebhookHost = errors.New("webhook host must be a bare hostname or address literal without scheme, port, path or whitespace")

// webhookHostCharsRE is the character set a webhook host may use: the characters
// of a DNS hostname, an IPv4 literal, or an IPv6 literal (which brings the colon).
// It is a rejection filter, not a hostname grammar - the caller's host has
// already been validated as a hostname where it was read from configuration.
// What this stops is a value carrying regex-relevant or URL-structural
// characters (a scheme, a path, a space) into the pattern below.
var webhookHostCharsRE = regexp.MustCompile(`^[A-Za-z0-9.:-]+$`)

// compileWebhookHostPattern builds the URL pattern for a deployment's configured
// webhook host. Every path under that host is treated as secret, because a
// webhook URL's path is the credential and a Slack-compatible endpoint
// (Mattermost and others) puts it under a path this package cannot know in
// advance - unlike hooks.slack.com, whose /services/ prefix is fixed.
//
// Group 1 is the scheme, host and the slash that ends the authority, kept out of
// the replacement so masked output still names the host that was contacted. The
// path class matches valueDetectorPatterns.slackWebhookURL so a trailing period
// or comma from the surrounding prose is not swallowed.
func compileWebhookHostPattern(host string) (*regexp.Regexp, error) {
	if !webhookHostCharsRE.MatchString(host) {
		return nil, fmt.Errorf("%w (got %q)", ErrInvalidWebhookHost, host)
	}

	authority := regexp.QuoteMeta(host)
	if strings.Contains(host, ":") {
		// A colon can only be an IPv6 literal here: the character filter above has
		// already rejected a scheme or a port. Configuration carries such a host in
		// its bare form ("::1"), while a URL always brackets it, so put the
		// brackets back.
		authority = `\[` + authority + `\]`
	}

	// The port is optional because validateWebhookURL compares only the hostname,
	// so a configured webhook URL may carry one.
	return regexp.Compile(`(?i)(\bhttps://` + authority + `(?::\d+)?/)[A-Za-z0-9/_-]+`)
}

// ValueDetector detects and masks sensitive values in text based on value format,
// independent of key names. It complements the existing key-name-based detection
// in SensitivePatterns by catching secrets that appear without a recognizable key.
type ValueDetector struct {
	placeholder string
	// webhookHostURL masks URLs on the deployment's configured webhook host. It
	// is nil when no host is configured, which is the only interpretation that
	// assumes nothing about the caller: a Config built without WithWebhookHost
	// falls back to the fixed hooks.slack.com pattern alone.
	webhookHostURL *regexp.Regexp
}

// newValueDetector creates a ValueDetector that masks detected values with the
// given placeholder string (e.g., "[REDACTED]"). webhookHostURL comes from
// compileWebhookHostPattern, or is nil when the deployment configured no webhook
// host; taking the compiled pattern rather than the host name keeps validation
// in NewConfig, which is the only place that can report the error.
func newValueDetector(placeholder string, webhookHostURL *regexp.Regexp) *ValueDetector {
	return &ValueDetector{placeholder: placeholder, webhookHostURL: webhookHostURL}
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

	// The configured host runs before the fixed hooks.slack.com pattern, and not
	// after, for the case where the two name the same host - the default
	// deployment. Masking hooks.slack.com first would leave "/services/" as the
	// only path characters in front of the placeholder, which this pattern would
	// then match and mask a second time.
	if d.webhookHostURL != nil {
		result = d.webhookHostURL.ReplaceAllString(result, "${1}"+escapedPlaceholder)
	}
	result = valueDetectorPatterns.slackWebhookURL.ReplaceAllString(result, "${1}"+escapedPlaceholder)

	return result
}
