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
}{
	awsKeyID:    regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b|\bASIA[0-9A-Z]{16}\b`),
	githubToken: regexp.MustCompile(`\bgh[pors]_\s*[A-Za-z0-9_]{36,}\b`),
	slackToken:  regexp.MustCompile(`\bxox[bpar]-[0-9]{10,}-[0-9]{10,}-[a-zA-Z0-9]+\b`),
	// NOTE: unlike the rest of this set, this pattern is NOT key-independent - it
	// anchors on the literal field name "private_key_id" because a GCP key ID is an
	// opaque hex fingerprint with no self-identifying format. It's also not itself
	// secret: the real credential is the "private_key" PEM block, already caught
	// key-independently by pemPrivate below. Kept as defense-in-depth (masks the
	// fingerprint too), but doesn't alone satisfy "key-independent detection" for
	// the GCP category; see docs/user/security-risk-assessment.md Limitations.
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
	// dots, with header/payload length bounds (7+/10+) to exclude short base64url
	// fragments; the signature may be empty (alg=none). RE2 has no lookahead, so
	// the trailing group consumes the character after the token and requires it
	// to be neither base64url nor a dot - a fourth segment stops the match rather
	// than silently truncating it. Mask re-emits that character via "${1}". One
	// consequence: a JWT followed by a sentence-final period ("...abc.") is not
	// matched, since that's a three-dot string per 3.3.2 condition 1 -
	// TestValueDetector_JWT pins this as a known limitation.
	jwt: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{7,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]*([^A-Za-z0-9_.-]|$)`),
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
// advance.
//
// Group 1 is the scheme, host and the slash that ends the authority, kept out of
// the replacement so masked output still names the host that was contacted. The
// path class excludes trailing punctuation so surrounding prose isn't swallowed.
//
// Deliberately unanchored (CodeQL go/regex/missing-regexp-anchor): that check is
// about trust decisions, where an unanchored host lets an attacker forge a
// prefix. Here the pattern searches free text for a secret to erase, so it must
// match mid-string - over-matching just costs an extra placeholder, not a leak.
// Anchoring would stop the common case (a URL quoted in an error message) from
// being masked at all.
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
	// is nil when no host is configured (Slack notifications switched off, or -
	// before the TOML is read - the bootstrap phase that predates it), in which
	// case Mask performs no webhook masking at all.
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
	// From here on, patterns with a capture group re-emit it (via "${1}" etc.) so
	// masked output keeps its surrounding structure - "Bearer [REDACTED]" rather
	// than a bare placeholder.
	result = valueDetectorPatterns.gcpSAKey.ReplaceAllString(result, "${1}"+escapedPlaceholder+"${2}")
	result = valueDetectorPatterns.bearerToken.ReplaceAllString(result, "${1}"+escapedPlaceholder)
	result = valueDetectorPatterns.urlCred.ReplaceAllString(result, "${1}"+escapedPlaceholder+"@")

	// Added in 3.3.1; run after the original seven so their masked output is
	// unchanged.
	result = valueDetectorPatterns.githubPAT.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.slackPrefixToken.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.jwt.ReplaceAllString(result, escapedPlaceholder+"${1}")

	// webhookHostURL is nil until the deployment's webhook host is known (see
	// the field comment), in which case there is nothing to mask here.
	if d.webhookHostURL != nil {
		result = d.webhookHostURL.ReplaceAllString(result, "${1}"+escapedPlaceholder)
	}

	return result
}
