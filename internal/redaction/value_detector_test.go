package redaction

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		require.NotEqual(t, input, result,
			"expected input to be masked, but it was left unchanged: %q", input)
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

// TestValueDetector_GitHubFineGrainedPAT verifies that a fine-grained PAT
// (github_pat_ prefix) is masked when it appears without a key name. The
// min-length boundary is fixed in a pair: 29 body characters stay untouched, 30
// are replaced.
func TestValueDetector_GitHubFineGrainedPAT(t *testing.T) {
	const placeholder = "[MASKED]"
	d := NewValueDetector(placeholder)

	tests := []struct {
		name     string
		input    string
		wantMask bool
	}{
		{
			name:     "30 characters is replaced",
			input:    "github_pat_" + strings.Repeat("a", 30),
			wantMask: true,
		},
		{
			name:     "29 characters is left alone",
			input:    "github_pat_" + strings.Repeat("a", 29),
			wantMask: false,
		},
		{
			name:     "underscores count toward the length",
			input:    "github_pat_" + strings.Repeat("a", 15) + strings.Repeat("_", 15),
			wantMask: true,
		},
		{
			name:     "embedded between prose",
			input:    "cloning as github_pat_" + strings.Repeat("a", 30) + " over https",
			wantMask: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Mask(tt.input)
			if tt.wantMask {
				assert.Contains(t, result, placeholder,
					"fine-grained PAT must be replaced with the placeholder")
				assert.NotContains(t, result, "github_pat_",
					"the PAT body must not remain in the clear")
			} else {
				assert.Equal(t, tt.input, result,
					"a PAT below the min length must be left unchanged")
			}
		})
	}
}

// TestValueDetector_SlackPrefixTokens verifies that tokens with the xapp- /
// xoxe- / xoxs- prefixes are masked, while the existing xoxb- / xoxp- / xoxa- /
// xoxr- prefixes keep their prior detection (the 3-segment slackToken pattern
// still owns them). The min-length boundary is fixed in a pair.
func TestValueDetector_SlackPrefixTokens(t *testing.T) {
	const placeholder = "[MASKED]"
	d := NewValueDetector(placeholder)

	tests := []struct {
		name     string
		input    string
		wantMask bool
	}{
		{
			name:     "xapp- 10 characters is replaced",
			input:    "xapp-" + strings.Repeat("a", 10),
			wantMask: true,
		},
		{
			name:     "xapp- 9 characters is left alone",
			input:    "xapp-" + strings.Repeat("a", 9),
			wantMask: false,
		},
		{
			name:     "xoxe- is replaced",
			input:    "xoxe-" + strings.Repeat("a", 10),
			wantMask: true,
		},
		{
			name:     "xoxs- is replaced",
			input:    "xoxs-" + strings.Repeat("a", 10),
			wantMask: true,
		},
		{
			name:     "xoxb- keeps the existing 3-segment detection",
			input:    "xoxb-" + strings.Repeat("1", 12) + "-" + strings.Repeat("2", 12) + "-abc",
			wantMask: true,
		},
		{
			name:     "xoxp- keeps the existing 3-segment detection",
			input:    "xoxp-" + strings.Repeat("1", 12) + "-" + strings.Repeat("2", 12) + "-abc",
			wantMask: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Mask(tt.input)
			if tt.wantMask {
				assert.Contains(t, result, placeholder,
					"Slack prefix token must be replaced with the placeholder")
				assert.NotContains(t, result, tt.input,
					"the token must not remain in the clear")
			} else {
				assert.Equal(t, tt.input, result,
					"a token below the min length must be left unchanged")
			}
		})
	}
}

// TestValueDetector_JWT verifies that a three-segment JWT is replaced, that an
// alg=none JWT (empty signature) is replaced too, and that the false-positive
// guards in 3.3.2 hold: the header/payload min-length 10 boundary is fixed in
// pairs.
func TestValueDetector_JWT(t *testing.T) {
	const placeholder = "[MASKED]"
	d := NewValueDetector(placeholder)

	tests := []struct {
		name     string
		input    string
		wantMask bool
	}{
		{
			name:     "three-segment JWT is replaced",
			input:    "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc",
			wantMask: true,
		},
		{
			name:     "empty signature (alg=none) is replaced",
			input:    "eyJhbGciOiJub25lIn0.eyJzdWIiOiJhZG1pbiJ9.",
			wantMask: true,
		},
		{
			name:     "header of 9 characters is left alone",
			input:    "eyJaaaaaa.bbbbbbbbbb.ccc",
			wantMask: false,
		},
		{
			name:     "header of 10 characters is replaced",
			input:    "eyJaaaaaaa.bbbbbbbbbb.ccc",
			wantMask: true,
		},
		{
			name:     "payload of 9 characters is left alone",
			input:    "eyJaaaaaaa.bbbbbbbbb.ccc",
			wantMask: false,
		},
		{
			name:     "payload of 10 characters is replaced",
			input:    "eyJaaaaaaa.bbbbbbbbbb.ccc",
			wantMask: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Mask(tt.input)
			if tt.wantMask {
				assert.Contains(t, result, placeholder,
					"JWT must be replaced with the placeholder")
				assert.NotContains(t, result, "eyJ",
					"the JWT must not remain in the clear")
			} else {
				assert.Equal(t, tt.input, result,
					"a JWT below the length bounds must be left unchanged")
			}
		})
	}
}

// TestValueDetector_SlackWebhookURL verifies the hooks.slack.com/services/
// pattern: the URL is replaced while the /services/ prefix is kept, and both
// false-positive pairs (no /services/ path, non-hooks host) are left alone.
func TestValueDetector_SlackWebhookURL(t *testing.T) {
	const placeholder = "[MASKED]"
	d := NewValueDetector(placeholder)

	tests := []struct {
		name     string
		input    string
		wantMask bool
	}{
		{
			name:     "webhook URL keeps the /services/ prefix",
			input:    "https://hooks.slack.com/services/T000/B000/XXXX",
			wantMask: true,
		},
		{
			name:     "hooks host without /services/ path is left alone",
			input:    "https://hooks.slack.com/",
			wantMask: false,
		},
		{
			name:     "different host is left alone",
			input:    "https://hooks.slack.com.evil/services/T000/B000/XXXX",
			wantMask: false,
		},
		{
			name:     "unrelated host is left alone",
			input:    "https://example.com/services/T000/B000/XXXX",
			wantMask: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Mask(tt.input)
			if tt.wantMask {
				assert.Contains(t, result, "https://hooks.slack.com/services/"+placeholder,
					"the /services/ prefix must be preserved around the placeholder")
				assert.NotContains(t, result, "/T000/",
					"the webhook path must not remain in the clear")
			} else {
				assert.Equal(t, tt.input, result,
					"a URL that is not a hooks.slack.com webhook must be left unchanged")
			}
		})
	}
}

// TestValueDetector_FreeTextEmbedding verifies that the three value formats
// covered above (fine-grained PAT, Slack prefix token, JWT) are masked even
// when embedded in free text (surrounding prose) rather than appearing
// standalone.
func TestValueDetector_FreeTextEmbedding(t *testing.T) {
	const placeholder = "[MASKED]"
	d := NewValueDetector(placeholder)

	tests := []struct {
		name     string
		input    string
		fragment string
	}{
		{
			name:     "fine-grained PAT in command output",
			input:    "cloning repo with github_pat_" + strings.Repeat("a", 30) + " over https",
			fragment: "github_pat_",
		},
		{
			name:     "Slack prefix token in command output",
			input:    "posting to channel with xapp-" + strings.Repeat("a", 10) + " as the app token",
			fragment: "xapp-",
		},
		{
			name:     "JWT in command output",
			input:    "auth token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc expired",
			fragment: "eyJ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Mask(tt.input)
			assert.Contains(t, result, placeholder,
				"the embedded secret must be replaced with the placeholder")
			assert.NotContains(t, result, tt.fragment,
				"the embedded secret must not remain in the clear")
		})
	}
}

// TestValueDetector_FalsePositives verifies that none of the new patterns
// matches a string that merely resembles one of the secret formats.
func TestValueDetector_FalsePositives(t *testing.T) {
	d := NewValueDetector("[MASKED]")

	tests := []string{
		"github_pattern",
		"xapple",
		"eyJhbGciOiJIUzI1NiJ9",
		"eyJaaaaaaa.bbbbbbbbbb",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc.def",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			result := d.Mask(input)
			assert.Equal(t, input, result,
				"a non-secret string resembling a secret format must be left unchanged")
		})
	}
}

// TestValueDetector_PlaceholderWithDollarNoReinjection verifies that when the
// placeholder contains "$1", none of the four added patterns re-injects the
// original secret via capture-group expansion.
func TestValueDetector_PlaceholderWithDollarNoReinjection(t *testing.T) {
	const placeholder = "[$1]"
	d := NewValueDetector(placeholder)

	tests := []struct {
		name     string
		input    string
		fragment string
	}{
		{
			name:     "fine-grained PAT",
			input:    "github_pat_" + strings.Repeat("a", 30),
			fragment: "github_pat_",
		},
		{
			name:     "Slack prefix token",
			input:    "xapp-" + strings.Repeat("a", 10),
			fragment: "xapp-",
		},
		{
			name:     "JWT",
			input:    "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc",
			fragment: "eyJ",
		},
		{
			name:     "webhook URL",
			input:    "https://hooks.slack.com/services/T000/B000/XXXX",
			fragment: "/T000/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.Mask(tt.input)
			assert.Contains(t, result, placeholder,
				"the masked output must contain the literal [$1] placeholder")
			assert.NotContains(t, result, tt.fragment,
				"the secret must not be re-injected by capture-group expansion")
		})
	}
}
