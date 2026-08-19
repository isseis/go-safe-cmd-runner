// Package redaction provides shared redaction functionality.
package redaction

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

// valueDetectorPatterns holds the value-based secret patterns, compiled once at
// package initialization.
var valueDetectorPatterns = struct {
	awsKeyID         *regexp.Regexp // AWS access key IDs: AKIA, ASIA, etc.
	githubToken      *regexp.Regexp // GitHub tokens: ghp_, gho_, ghs_, etc.
	slackToken       *regexp.Regexp // Slack tokens: xoxb-, xoxp-, xoxa-, xoxr-, etc.
	gcpSAKey         *regexp.Regexp // GCP service account key ID field; groups 1 and 2 are the surrounding "private_key_id":" and closing quote
	pemPrivate       *regexp.Regexp // PEM private key blocks: -----BEGIN ... PRIVATE KEY-----
	bearerToken      *regexp.Regexp // Bearer tokens: standard OAuth pattern; group 1 is the "Bearer " prefix
	urlCred          *regexp.Regexp // URL-embedded credentials: scheme://user:pass@host; group 1 is "scheme://"
	githubPAT        *regexp.Regexp // GitHub fine-grained PATs: github_pat_ + 30+ base62 chars
	slackPrefixToken *regexp.Regexp // Slack tokens with the xapp- / xoxe- / xoxs- prefixes
	jwt              *regexp.Regexp // JSON Web Tokens: eyJ + three dot-separated base64url segments; group 1 is the character that ends the token
}{
	awsKeyID:    regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b|\bASIA[0-9A-Z]{16}\b`),
	githubToken: regexp.MustCompile(`\bgh[pors]_\s*[A-Za-z0-9_]{36,}\b`),
	slackToken:  regexp.MustCompile(`\bxox[bpar]-[0-9]{10,}-[0-9]{10,}-[a-zA-Z0-9]+\b`),
	// Unlike the rest of this set this pattern is NOT key-independent: a GCP key
	// ID is an opaque hex fingerprint, so it anchors on the field name. The real
	// credential is the "private_key" PEM block, which pemPrivate catches
	// key-independently; see docs/user/security-risk-assessment.md Limitations.
	gcpSAKey:    regexp.MustCompile(`("private_key_id"\s*:\s*")[a-fA-F0-9]{32,}(")`),
	pemPrivate:  regexp.MustCompile(`(?s)-----BEGIN\s[A-Z\s]*PRIVATE\sKEY-----.*?-----END\s[A-Z\s]*PRIVATE\sKEY-----`),
	bearerToken: regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9\-._~+/]+=*`),
	urlCred:     regexp.MustCompile(`(?i)(\b[a-z][a-z0-9+\-.]*://)[^/?:]+:[^/@?]+@`),
	// Fine-grained PATs, whose github_pat_ prefix githubToken does not cover. Real
	// tokens run to 80+ characters; the 30 lower bound keeps prose like
	// "github_pattern" from matching.
	githubPAT: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{30,}\b`),
	// The xapp-, xoxe- and xoxs- prefixes slackToken's [bpar] class omits. Kept
	// separate rather than widening that class because these tokens do not take
	// the three-segment shape slackToken requires.
	slackPrefixToken: regexp.MustCompile(`\bx(?:app|oxe|oxs)-[A-Za-z0-9-]{9,}[A-Za-z0-9]`),
	// A compact JWT; the signature may be empty (alg=none) and the length bounds
	// exclude short base64url fragments. RE2 has no lookahead, so the trailing
	// group consumes the character after the token and rejects a fourth segment
	// instead of truncating the match; Mask re-emits it via "${1}". Consequence:
	// a JWT followed by a sentence-final period is not matched, pinned as a known
	// limitation by TestValueDetector_JWT.
	jwt: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{7,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]*([^A-Za-z0-9_.-]|$)`),
}

// ErrInvalidWebhookHost is returned when WithWebhookHost is given something that
// is not a bare hostname or address literal.
var ErrInvalidWebhookHost = errors.New("webhook host must be a bare hostname or address literal without scheme, port, path or whitespace")

// webhookHostLabelRE matches the RFC 1123 §2.1 character pattern for a single
// DNS label or IPv4 octet: starts and ends with a letter or digit; interior
// may contain hyphens.
var webhookHostLabelRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?$`)

// ValidateWebhookHost reports whether host is a bare hostname, IPv4 literal, or
// bare IPv6 literal - no scheme, port, path, brackets, or whitespace. It is the
// single definition of "valid webhook host" shared by compileWebhookHostPattern
// and normalizeSlackAllowedHost (internal/runner/bootstrap), so the two callers
// cannot drift into disagreeing about what a valid host looks like.
//
// An IPv6 zone identifier is rejected rather than stripped: a URL spells it
// percent-encoded ("[fe80::1%25eth0]") and configuration spells it bare, so a
// caller comparing decoded hostnames would accept a URL that the pattern built
// from the bare form never matches - leaving the webhook path unmasked.
func ValidateWebhookHost(host string) error {
	if strings.Contains(host, ":") {
		// A bare host carries a colon only as an IPv6 literal; host:port or a
		// scheme fails ParseAddr.
		addr, err := netip.ParseAddr(host)
		if err != nil || !addr.Is6() || addr.Zone() != "" {
			return fmt.Errorf("%w (got %q)", ErrInvalidWebhookHost, host)
		}
		return nil
	}
	for label := range strings.SplitSeq(host, ".") {
		if !webhookHostLabelRE.MatchString(label) {
			return fmt.Errorf("%w (got %q)", ErrInvalidWebhookHost, host)
		}
	}
	return nil
}

// compileWebhookHostPattern builds the URL pattern for a deployment's configured
// webhook host. Every path under that host is treated as secret: the path is the
// credential, and a Slack-compatible endpoint (Mattermost and others) puts it
// somewhere this package cannot know in advance. Group 1 is the scheme, host and
// authority-ending slash, kept out of the replacement so masked output still
// names the host that was contacted.
//
// Deliberately unanchored (CodeQL go/regex/missing-regexp-anchor): that check is
// about trust decisions, where an unanchored host lets an attacker forge a
// prefix. Here the pattern hunts free text for a secret to erase, so it must
// match mid-string; over-matching costs an extra placeholder, not a leak, while
// anchoring would miss the common case of a URL quoted in an error message.
func compileWebhookHostPattern(host string) (*regexp.Regexp, error) {
	if err := ValidateWebhookHost(host); err != nil {
		return nil, err
	}

	authority := regexp.QuoteMeta(host)
	if strings.Contains(host, ":") {
		// ValidateWebhookHost has left only IPv6 literals carrying a colon.
		// Configuration holds them bare ("::1") and a URL brackets them.
		authority = `\[` + authority + `\]`
	}

	// The port is optional because validateWebhookURL compares only the hostname,
	// so a configured URL may carry one. The path class allows embedded dots but
	// must end on a non-dot, leaving a sentence-final period outside the match.
	return regexp.Compile(`(?i)(\bhttps://` + authority + `(?::\d+)?/)(?:[A-Za-z0-9/_.-]*[A-Za-z0-9/_-])`)
}

// ValueDetector detects and masks sensitive values in text based on value format,
// independent of key names. It complements the existing key-name-based detection
// in SensitivePatterns by catching secrets that appear without a recognizable key.
type ValueDetector struct {
	placeholder string
	// webhookHostURL masks URLs on the deployment's configured webhook host. It is
	// nil when there is no such host yet - Slack switched off, or the bootstrap
	// phase that runs before the TOML is read - and Mask then skips that step.
	webhookHostURL *regexp.Regexp
}

// newValueDetector creates a ValueDetector masking with the given placeholder
// (e.g. "[REDACTED]"). It takes webhookHostURL already compiled rather than the
// host name so validation stays in NewConfig, the only place that can report
// the error.
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

	// ReplaceAllString expands "$1" and friends in the replacement, so a
	// placeholder containing "$" would re-inject the matched (secret) text.
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

	// Run after the original seven so their masked output is unchanged.
	result = valueDetectorPatterns.githubPAT.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.slackPrefixToken.ReplaceAllString(result, escapedPlaceholder)
	result = valueDetectorPatterns.jwt.ReplaceAllString(result, escapedPlaceholder+"${1}")

	if d.webhookHostURL != nil {
		result = d.webhookHostURL.ReplaceAllString(result, "${1}"+escapedPlaceholder)
	}

	return result
}
