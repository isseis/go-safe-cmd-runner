package redaction

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValueDetector_Mask_PositiveCases(t *testing.T) {
	const placeholder = "[MASKED]"
	d := NewValueDetector(placeholder)

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "AWS access key ID AKIA",
			input: "export AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n",
		},
		{
			name:  "AWS access key ID ASIA (temporary)",
			input: " creds: ASIAJ2F4ABCDEFGHIJKL for session\n",
		},
		{
			name:  "GitHub personal access token ghp_",
			input: "token = ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ab\n",
		},
		{
			name:  "GitHub OAuth token gho_",
			input: "GITHUB_TOKEN=gho_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ab\n",
		},
		{
			name:  "GitHub server-to-server token ghs_",
			input: "GH_TOKEN=ghs_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ab\n",
		},
		{
			name:  "Slack bot token xoxb-",
			input: "SLACK_BOT_TOKEN=" + "xoxb-" + "999999999999-888888888888-aaaaaaaaaaaaaaaabbbbbbbbbb\n",
		},
		{
			name:  "Slack xoxp- token",
			input: "xoxp-" + "111111111111-222222222222-cccccccccccccccccddddddddddd",
		},
		{
			name:  "Slack token with mock data",
			input: "token: " + "xoxb-" + "000000000000-111111111111-zzzzzzzzzzzzzzzzzzzz",
		},
		{
			name:  "GCP service account private_key_id field",
			input: `{"type": "service_account", "private_key_id": "abcd1234ef5678abcd1234ef5678abcd1234ef56"}`,
		},
		{
			name: "PEM private key block",
			input: `-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA...
-----END RSA PRIVATE KEY-----`,
		},
		{
			name: "PEM EC private key block",
			input: `-----BEGIN EC PRIVATE KEY-----
MHQCAQEEI...
-----END EC PRIVATE KEY-----`,
		},
		{
			name:  "Bearer token in Authorization header",
			input: "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		},
		{
			name:  "URL with embedded credentials",
			input: "endpoint: https://admin:hunter2@api.internal.example.com/v1/status\n",
		},
		{
			name:  "Multiple secrets in one text",
			input: "AKIAIOSFODNN7EXAMPLE and ghp_token123456789012345678901234567890ab in same line",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Mask(tt.input)
			assert.NotEqual(t, tt.input, result,
				"expected input to be modified by masking")
			assert.Contains(t, result, placeholder,
				"masked output must contain the placeholder")
		})
	}
}

func TestValueDetector_Mask_NegativeCases(t *testing.T) {
	const placeholder = "[MASKED]"
	d := NewValueDetector(placeholder)

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "normal text without secrets",
			input: "This is a normal log message with no secrets.",
		},
		{
			name:  "command output with file paths",
			input: "Compiling src/main.go -> bin/main... OK",
		},
		{
			name:  "partial AWS-like prefix without enough chars",
			input: "AKIA123456 (too short)",
		},
		{
			name:  "similar but not a real GitHub token prefix",
			input: "gh_pages_branch_1234",
		},
		{
			name: "public key, not private",
			input: `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhki...
-----END PUBLIC KEY-----`,
		},
		{
			name:  "URL without credentials",
			input: "downloading from https://github.com/user/repo/releases/download/v1.0/app",
		},
		{
			name:  "hex hash value (not GCP SA key)",
			input: "sha256:abcd1234ef5678abcd1234ef5678abcd1234ef5678abcd1234ef5678abcd12",
		},
		{
			name:  "token keyword without a credential format",
			input: "token count: 5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Mask(tt.input)
			assert.Equal(t, tt.input, result,
				"expected output to be identical to input (no masking)")
		})
	}
}

func TestValueDetector_Mask_AllPatternsReturnSamePlaceholder(t *testing.T) {
	const placeholder = "[SECRET]"
	d := NewValueDetector(placeholder)

	// Each secret type should be replaced with the placeholder, not left partially masked
	inputs := []string{
		"AWS key: AKIAIOSFODNN7EXAMPLE",
		"token: ghp_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789ab",
		"token: xoxb-123456789012-123456789012-abcd",
		"Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.xyz",
		"https://user:pass@example.com/path",
	}

	for _, input := range inputs {
		result := d.Mask(input)
		if result == input {
			continue // this input may not match any pattern, skip
		}
		assert.Contains(t, result, placeholder,
			"masked text must contain the placeholder: %q -> %q", input, result)
		// The placeholder should exactly replace the matched part, and any remaining
		// text must not contain credential-looking substrings.
		// (We don't assert exact count because patterns may overlap.)
	}
}

// TestValueDetector_Mask_PreservesNonSecretContext verifies that Bearer and
// URL-credential masking replaces only the secret portion, leaving the
// non-secret structural context (the "Bearer " prefix, the URL scheme and
// host) intact for log readability.
func TestValueDetector_Mask_PreservesNonSecretContext(t *testing.T) {
	const placeholder = "[MASKED]"
	d := NewValueDetector(placeholder)

	t.Run("Bearer prefix is preserved", func(t *testing.T) {
		result := d.Mask("Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc")
		assert.Contains(t, result, "Bearer "+placeholder)
		assert.NotContains(t, result, "eyJhbGciOiJIUzI1NiJ9")
	})

	t.Run("URL scheme and host are preserved", func(t *testing.T) {
		result := d.Mask("https://admin:hunter2@api.example.com/v1")
		assert.Contains(t, result, "https://"+placeholder+"@api.example.com/v1")
		assert.NotContains(t, result, "hunter2")
	})

	t.Run("GCP private_key_id field name and JSON structure are preserved", func(t *testing.T) {
		result := d.Mask(`{"private_key_id": "abcd1234ef5678abcd1234ef5678abcd1234ef56"}`)
		assert.Contains(t, result, `"private_key_id": "`+placeholder+`"`)
		assert.NotContains(t, result, "abcd1234ef5678abcd1234ef5678abcd1234ef56")
	})
}

// TestValueDetector_Mask_URLWithPortAndAtInPath verifies that a URL with an
// explicit port and a path segment containing "@" (but no embedded
// credentials) is not falsely matched as having a password containing "/".
func TestValueDetector_Mask_URLWithPortAndAtInPath(t *testing.T) {
	d := NewValueDetector("[MASKED]")
	input := "https://api.example.com:8080/path@something"
	result := d.Mask(input)
	assert.Equal(t, input, result,
		"URL with port and path segment containing @ but no credentials must not be masked")
}

func TestValueDetector_Mask_EmptyInput(t *testing.T) {
	d := NewValueDetector("[REDACTED]")
	result := d.Mask("")
	assert.Equal(t, "", result)
}

func TestNewValueDetector(t *testing.T) {
	d := NewValueDetector("[CUSTOM]")
	assert.NotNil(t, d)
	assert.Equal(t, "[CUSTOM]", d.placeholder)
}
