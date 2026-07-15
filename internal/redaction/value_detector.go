// Package redaction provides shared redaction functionality.
package redaction

import (
	"regexp"
)

// valueDetectorPatterns holds compiled regex patterns for value-based secret detection.
// Patterns are compiled once at package initialization and cached to avoid repeated
// allocation during redaction of long command outputs.
var valueDetectorPatterns = struct {
	awsKeyID    *regexp.Regexp // AWS access key IDs: AKIA, ASIA, etc.
	githubToken *regexp.Regexp // GitHub tokens: ghp_, gho_, ghs_, etc.
	slackToken  *regexp.Regexp // Slack tokens: xoxb-, xoxp-, xoxa-, xoxr-, etc.
	gcpSAKey    *regexp.Regexp // GCP service account key: json-key pattern
	pemPrivate  *regexp.Regexp // PEM private key blocks: -----BEGIN ... PRIVATE KEY-----
	bearerToken *regexp.Regexp // Bearer tokens: standard OAuth pattern
	urlCred     *regexp.Regexp // URL-embedded credentials: scheme://user:pass@host
}{
	awsKeyID:    regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b|\bASIA[0-9A-Z]{16}\b`),
	githubToken: regexp.MustCompile(`\bgh[pors]_\s*[A-Za-z0-9_]{36,}\b`),
	slackToken:  regexp.MustCompile(`\bxox[bpar]-[0-9]{10,}-[0-9]{10,}-[a-zA-Z0-9]+\b`),
	gcpSAKey:    regexp.MustCompile(`"private_key_id"\s*:\s*"[a-f0-9]{32,}"`),
	pemPrivate:  regexp.MustCompile(`(?s)-----BEGIN\s[A-Z\s]*PRIVATE\sKEY-----.*?-----END\s[A-Z\s]*PRIVATE\sKEY-----`),
	bearerToken: regexp.MustCompile(`(?i)Bearer\s+([A-Za-z0-9\-._~+/]+=*)`),
	urlCred:     regexp.MustCompile(`(?i)\b[a-z][a-z0-9+\-.]*://[^/?:]+:([^@?]+)@`),
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

	result := text
	patterns := []*regexp.Regexp{
		valueDetectorPatterns.awsKeyID,
		valueDetectorPatterns.githubToken,
		valueDetectorPatterns.slackToken,
		valueDetectorPatterns.gcpSAKey,
		valueDetectorPatterns.pemPrivate,
		valueDetectorPatterns.bearerToken,
		valueDetectorPatterns.urlCred,
	}

	for _, re := range patterns {
		result = re.ReplaceAllString(result, d.placeholder)
	}

	return result
}
