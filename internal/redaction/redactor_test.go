package redaction

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
)

// panickingLogValuer is a helper struct that panics when LogValue is called.
type panickingLogValuer struct{}

// LogValue implements the slog.LogValuer interface and always panics.
func (p panickingLogValuer) LogValue() slog.Value {
	panic("test panic")
}

// sensitiveLogValuer is a helper struct for testing LogValuer redaction with sensitive data.
type sensitiveLogValuer struct {
	data string
}

// LogValue implements the slog.LogValuer interface.
func (v sensitiveLogValuer) LogValue() slog.Value {
	return slog.StringValue(v.data)
}

// TestRedactText_EmptyString tests that empty strings are handled correctly
func TestRedactText_EmptyString(t *testing.T) {
	config := DefaultConfig()
	result := config.RedactText("")
	assert.Equal(t, "", result, "Empty string should return empty string")
}

// TestRedactText_NoSensitiveInfo tests that text without sensitive info is unchanged
func TestRedactText_NoSensitiveInfo(t *testing.T) {
	config := DefaultConfig()
	input := "This is a normal log message with no sensitive data"
	result := config.RedactText(input)
	assert.Equal(t, input, result, "Non-sensitive text should remain unchanged")
}

// TestRedactText_KeyValuePatterns tests key=value pattern redaction
func TestRedactText_KeyValuePatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase password",
			input:    "password=secret123",
			expected: "password=[REDACTED]",
		},
		{
			name:     "uppercase TOKEN",
			input:    "TOKEN=abc",
			expected: "TOKEN=[REDACTED]",
		},
		{
			name:     "mixed case preserving",
			input:    "Password=test",
			expected: "Password=[REDACTED]",
		},
		{
			name:     "multiple key=value pairs",
			input:    "user=john password=secret token=abc123",
			expected: "user=john password=[REDACTED] token=[REDACTED]",
		},
		{
			name:     "key with equals in pattern",
			input:    "Set-Cookie: sessionid=xyz123",
			expected: "Set-Cookie: sessionid=xyz123", // Not matched by default patterns
		},
		{
			name:     "api_key pattern",
			input:    "api_key=1234567890abcdef",
			expected: "api_key=[REDACTED]",
		},
		{
			name:     "secret pattern",
			input:    "secret=my-secret-value",
			expected: "secret=[REDACTED]",
		},
		{
			name:     "key pattern",
			input:    "key=some-key-value",
			expected: "key=[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_SpacePatterns tests Bearer/Basic authentication pattern redaction
func TestRedactText_SpacePatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Bearer token",
			input:    "Bearer token123",
			expected: "Bearer [REDACTED]",
		},
		{
			name:     "Basic auth",
			input:    "Basic dGVzdA==",
			expected: "Basic [REDACTED]",
		},
		{
			name:     "lowercase bearer",
			input:    "bearer mytoken456",
			expected: "bearer [REDACTED]",
		},
		{
			name:     "mixed case Basic",
			input:    "BaSiC encoded123",
			expected: "BaSiC [REDACTED]",
		},
		{
			name:     "multiple Bearer tokens",
			input:    "Bearer abc123 and Bearer xyz789",
			expected: "Bearer [REDACTED] and Bearer [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_ColonPatterns tests Authorization header pattern redaction
func TestRedactText_ColonPatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Authorization header with Bearer",
			input:    "Authorization: Bearer token123",
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "Authorization header no space (not in default patterns)",
			input:    "Authorization:token456",
			expected: "Authorization:token456", // Not matched by default patterns which only include "Authorization: " with space
		},
		{
			name:     "lowercase authorization with Basic",
			input:    "authorization: Basic dGVzdA==",
			expected: "authorization: Basic [REDACTED]",
		},
		{
			name:     "mixed case Authorization",
			input:    "AuThOrIzAtIoN: bearer secret",
			expected: "AuThOrIzAtIoN: bearer [REDACTED]",
		},
		{
			name:     "multiline headers",
			input:    "Authorization: Bearer abc\nContent-Type: application/json",
			expected: "Authorization: Bearer [REDACTED]\nContent-Type: application/json",
		},
		{
			name:     "Authorization with tabs",
			input:    "Authorization:\t\tBearer secret123",
			expected: "Authorization:\t\tBearer [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_MixedPatterns tests multiple different patterns in same text
func TestRedactText_MixedPatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "mixed key=value and Bearer",
			input:    "password=secret Bearer token123",
			expected: "password=[REDACTED] Bearer [REDACTED]",
		},
		{
			name:     "all pattern types",
			input:    "token=abc Bearer xyz Authorization: Basic dGVzdA==",
			expected: "token=[REDACTED] Bearer [REDACTED] Authorization: Basic [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_SpecialCharacters tests handling of special characters in keys
func TestRedactText_SpecialCharacters(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "underscore in value",
			input:    "password=my_secret_123",
			expected: "password=[REDACTED]",
		},
		{
			name:     "hyphen in value",
			input:    "token=abc-def-123",
			expected: "token=[REDACTED]",
		},
		{
			name:     "equals in value (stops at space)",
			input:    "key=value=with=equals next=token",
			expected: "key=[REDACTED] next=token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactText_QuotedValue tests that a quoted value is redacted up to its
// closing quote, including values that contain whitespace.
func TestRedactText_QuotedValue(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "double quoted value with space",
			input:    `password="abc def"`,
			expected: `password="[REDACTED]"`,
		},
		{
			name:     "single quoted value with space",
			input:    `password='abc def'`,
			expected: `password='[REDACTED]'`,
		},
		{
			name:     "unterminated double quote redacts to end of line",
			input:    `password="abc def`,
			expected: `password="[REDACTED]`,
		},
		{
			name:     "unterminated single quote redacts to end of line",
			input:    `password='abc def`,
			expected: `password='[REDACTED]`,
		},
		{
			name:     "quoted value keeps following text",
			input:    `password="abc def" user=john`,
			expected: `password="[REDACTED]" user=john`,
		},
		{
			name:     "common word key at start of text is redacted in full",
			input:    `TOKEN="abc def"`,
			expected: `TOKEN="[REDACTED]"`,
		},
		{
			name:     "two quoted values in one text",
			input:    `password="abc def" api_key="abc def"`,
			expected: `password="[REDACTED]" api_key="[REDACTED]"`,
		},
		{
			name:     "quoted value on a later line",
			input:    "line one\npassword=\"abc def\"\nline three",
			expected: "line one\npassword=\"[REDACTED]\"\nline three",
		},
		{
			name:     "unterminated quote stops at the newline",
			input:    "password=\"abc def\nline two",
			expected: "password=\"[REDACTED]\nline two",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.NotContains(t, result, "abc", "no fragment of the value may survive")
			assert.NotContains(t, result, "def", "no fragment of the value may survive")
		})
	}
}

// TestRedactText_QuotedValueKnownLimits fixes the two quoted-value shapes whose
// handling is deliberately incomplete, so that a later reader can tell the
// behavior is known rather than accidental.
func TestRedactText_QuotedValueKnownLimits(t *testing.T) {
	config := DefaultConfig()

	t.Run("escaped quote ends the value early", func(t *testing.T) {
		// Escaped quotes are out of scope: the pattern complexity and the
		// false-positive surface are not worth the rarity of the shape in command
		// output. The tail after the escaped quote therefore survives.
		assert.Equal(t, `{"password":"[REDACTED]"b","next":"c"}`,
			config.RedactText(`{"password":"a\"b","next":"c"}`))
	})

	t.Run("empty quoted value is still marked as redacted", func(t *testing.T) {
		// An empty value carries no secret, but distinguishing it would require a
		// separate alternative and would report the absence of a value to anyone
		// reading the log, so the placeholder is emitted unconditionally.
		assert.Equal(t, `password="[REDACTED]"`, config.RedactText(`password=""`))
	})
}

// TestRedactText_JSONForm tests the JSON shape, where the key name is quoted and
// the separator is a colon.
func TestRedactText_JSONForm(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "json object field",
			input:    `{"password": "secret"}`,
			expected: `{"password": "[REDACTED]"}`,
		},
		{
			name:     "json field without space after colon",
			input:    `{"password":"secret"}`,
			expected: `{"password":"[REDACTED]"}`,
		},
		{
			name:     "json field among other fields",
			input:    `{"user": "john", "password": "secret", "port": 8080}`,
			expected: `{"user": "john", "password": "[REDACTED]", "port": 8080}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.NotContains(t, result, "secret")
		})
	}
}

// TestRedactText_SeparatorVariants tests separators other than a bare "=".
func TestRedactText_SeparatorVariants(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "colon and space",
			input:    "password: secret",
			expected: "password: [REDACTED]",
		},
		{
			name:     "equals surrounded by spaces",
			input:    "password = secret",
			expected: "password = [REDACTED]",
		},
		{
			name:     "space before colon only",
			input:    "password :secret",
			expected: "password :[REDACTED]",
		},
		{
			name:     "equals followed by tab",
			input:    "password=\tsecret",
			expected: "password=\t[REDACTED]",
		},
		{
			name:     "match on a later line",
			input:    "line one\npassword: secret\nline three",
			expected: "line one\npassword: [REDACTED]\nline three",
		},
		{
			name:     "two separated values in one text",
			input:    "password: secret api_key: secret",
			expected: "password: [REDACTED] api_key: [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.Equal(t, tt.expected, result)
			assert.NotContains(t, result, "secret", "no fragment of the value may survive")
		})
	}
}

// TestRedactText_AlternativePriority tests that the quoted alternative wins over
// the adjacent "=" alternative when both match at the same position, and that a
// key that fails its boundary still falls through to the adjacent alternative.
func TestRedactText_AlternativePriority(t *testing.T) {
	config := DefaultConfig()

	t.Run("quoted alternative wins for a key with a loose boundary", func(t *testing.T) {
		result := config.RedactText(`password="abc def"`)
		assert.Equal(t, `password="[REDACTED]"`, result)
		assert.NotContains(t, result, ` def"`, "the adjacent alternative must not truncate at the space")
	})

	t.Run("common-word key without a boundary falls through to the adjacent form", func(t *testing.T) {
		// "key" is preceded by "n", which is not an identifier-internal character
		// or a quote, so the quoted alternative does not apply and the result is
		// the same as before this coverage was added.
		result := config.RedactText(`monkey="a b"`)
		assert.Equal(t, `monkey=[REDACTED] b"`, result)
	})
}

// TestRedactText_KeyGroupBehavior fixes the redaction outcome of every key of
// DefaultKeyValuePatterns that is routed to the key/value path, for each of the
// five value shapes, according to the key's boundary group.
func TestRedactText_KeyGroupBehavior(t *testing.T) {
	config := DefaultConfig()

	// forms builds the five shapes under test for a given key.
	type form struct {
		name  string
		input string
	}
	forms := func(key string) []form {
		return []form{
			{name: "adjacent_equals", input: key + "=xyz"},
			{name: "colon_separator", input: key + ": xyz"},
			{name: "spaced_equals", input: key + " = xyz"},
			{name: "quoted_value", input: key + `="a b"`},
			{name: "json_field", input: `"` + key + `": "xyz"`},
		}
	}

	// redactedForms returns the expectation for keys whose boundary permits every
	// shape (the specific and prefixed groups).
	redactedForms := func(key string) []string {
		return []string{
			key + "=[REDACTED]",
			key + ": [REDACTED]",
			key + " = [REDACTED]",
			key + `="[REDACTED]"`,
			`"` + key + `": "[REDACTED]"`,
		}
	}

	// commonWordForms returns the expectation for common-word keys. The bare
	// colon and spaced-equals shapes stay untouched so that ordinary prose such
	// as "Primary key: id" is not redacted. The adjacent form carries no boundary
	// requirement at all, and the double-quoted value is a structural signal
	// strong enough to take the loose boundary, so both are redacted in full.
	commonWordForms := func(key string) []string {
		return []string{
			key + "=[REDACTED]",
			key + ": xyz",
			key + " = xyz",
			key + `="[REDACTED]"`,
			`"` + key + `": "[REDACTED]"`,
		}
	}

	tests := []struct {
		key      string
		expected []string
	}{
		// Specific keys: every shape is redacted.
		{key: "password", expected: redactedForms("password")},
		{key: "api_key", expected: redactedForms("api_key")},
		// Common-word keys: limited to identifier-internal and quoted boundaries.
		{key: "token", expected: commonWordForms("token")},
		{key: "key", expected: commonWordForms("key")},
		{key: "secret", expected: commonWordForms("secret")},
		// Prefixed keys: the leading "_" is the boundary, so every shape is
		// redacted without any additional boundary requirement.
		{key: "_PASSWORD", expected: redactedForms("_PASSWORD")},
		{key: "_TOKEN", expected: redactedForms("_TOKEN")},
		{key: "_KEY", expected: redactedForms("_KEY")},
		{key: "_SECRET", expected: redactedForms("_SECRET")},
	}

	for _, tt := range tests {
		for i, f := range forms(tt.key) {
			t.Run(tt.key+"/"+f.name, func(t *testing.T) {
				assert.Equal(t, tt.expected[i], config.RedactText(f.input))
			})
		}
	}
}

// TestKeyBoundaryGroup_Classification tests that every default key lands in the
// intended boundary group, and that a user-added key gets the loose boundary.
func TestKeyBoundaryGroup_Classification(t *testing.T) {
	tests := []struct {
		key      string
		expected boundaryGroup
	}{
		{key: "password", expected: boundaryGroupSpecific},
		{key: "api_key", expected: boundaryGroupSpecific},
		{key: "token", expected: boundaryGroupCommonWord},
		{key: "key", expected: boundaryGroupCommonWord},
		{key: "secret", expected: boundaryGroupCommonWord},
		{key: "_PASSWORD", expected: boundaryGroupPrefixed},
		{key: "_TOKEN", expected: boundaryGroupPrefixed},
		{key: "_KEY", expected: boundaryGroupPrefixed},
		{key: "_SECRET", expected: boundaryGroupPrefixed},
		// A key the user adds is treated as a declaration that the key marks a
		// secret, so it gets the loose boundary rather than the strict one.
		{key: "passphrase", expected: boundaryGroupSpecific},
		// Case does not affect the common-word lookup.
		{key: "Token", expected: boundaryGroupCommonWord},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			assert.Equal(t, tt.expected, keyBoundaryGroup(tt.key))
		})
	}

	t.Run("remaining default keys never reach keyBoundaryGroup", func(t *testing.T) {
		// The colon and space paths claim these keys before the key/value path,
		// so their boundary group is never consulted.
		classified := map[string]struct{}{}
		for _, tt := range tests {
			classified[tt.key] = struct{}{}
		}
		for _, key := range DefaultKeyValuePatterns() {
			if _, ok := classified[key]; ok {
				continue
			}
			assert.True(t, strings.Contains(key, ":") || strings.Contains(key, " "),
				"unclassified default key %q must be routed to the colon or space path", key)
		}
	})

	t.Run("user-added key is redacted with the loose boundary", func(t *testing.T) {
		config := DefaultConfig()
		config.KeyValuePatterns = append(config.KeyValuePatterns, "passphrase")
		assert.Equal(t, "passphrase: [REDACTED]", config.RedactText("passphrase: xyz"))
	})
}

// TestRedactText_ExistingBehaviorPreserved tests that the shapes handled by the
// colon path, the space path and the "key contains =" rule are unchanged.
//
// The first three rows overlap with TestRedactText_ColonPatterns and
// TestRedactText_SpacePatterns on purpose: those tests describe what the colon
// and space paths do, while this one is the regression guard that the key/value
// path's new alternatives did not reach into them.
func TestRedactText_ExistingBehaviorPreserved(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "authorization header keeps scheme",
			input:    "Authorization: Bearer xxx",
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "bearer prefix is preserved",
			input:    "Bearer xxx",
			expected: "Bearer [REDACTED]",
		},
		{
			name:     "basic prefix is preserved",
			input:    "Basic xxx",
			expected: "Basic [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, config.RedactText(tt.input))
		})
	}

	t.Run("key containing equals redacts only the adjacent token", func(t *testing.T) {
		result := config.performKeyValuePatternRedaction("Authorization=Bearer token", "Authorization=", "[REDACTED]")
		assert.Equal(t, "Authorization=[REDACTED] token", result)
	})
}

// TestRedactText_NoNewOverRedaction fixes the texts that must stay untouched, so
// that widening the separator and quote coverage does not start redacting
// ordinary diagnostic output.
func TestRedactText_NoNewOverRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "common word key preceded by a space",
			input: "Primary key: id",
		},
		{
			name:  "common word key in prose preceded by a space",
			input: "unexpected token: '}'",
		},
		{
			name:  "common word key preceded by an opening bracket",
			input: "map[key:value]",
		},
		{
			name:  "common word key preceded by an opening brace",
			input: "configMapKeyRef: {key: LOG_LEVEL}",
		},
		{
			name:  "key name continues into another word",
			input: "keyboard: qwerty",
		},
		{
			name:  "key name is a path segment",
			input: "/usr/local/key/path",
		},
		{
			name:  "flag that matches no key",
			input: "--timeout=30",
		},
		{
			name:  "separator is not allowed to span a newline",
			input: "password:\nsecret",
		},
		{
			// An object value has no whitespace to stop at, so redacting it would
			// delete every sibling field and leave the line unparseable.
			name:  "compact json object value",
			input: `{"password":{"a":1},"port":80}`,
		},
		{
			name:  "compact json array value",
			input: `{"api_key":["a","b"],"port":80}`,
		},
		{
			name:  "brace value after a colon separator",
			input: "password: {json: here} trailing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.input, config.RedactText(tt.input))
		})
	}
}

// TestRedactText_IntentionalOverRedaction fixes the shapes that are newly
// redacted even though they hold no secret. Neither can be told apart from a
// real secret by the key name and the shape alone, and excluding either would
// also stop a required shape from being redacted.
func TestRedactText_IntentionalOverRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			// A common-word key quoted as a structured-data field name. Excluding
			// this would also stop the JSON shape from being redacted at all.
			name:     "common word key as a json field name",
			input:    `"key": "us-east-1"`,
			expected: `"key": "[REDACTED]"`,
		},
		{
			// A specific key followed by a colon in prose. This is the same shape
			// as "password: secret", which must be redacted, so the two cannot be
			// told apart; the first word of the message is lost.
			name:     "specific key followed by prose",
			input:    "failed to read password: permission denied",
			expected: "failed to read password: [REDACTED] denied",
		},
		{
			name:     "specific key followed by prose mid-sentence",
			input:    "could not parse api_key: unexpected EOF",
			expected: "could not parse api_key: [REDACTED] EOF",
		},
		{
			// The loose boundary of the double-quoted alternative applies to the
			// key name, so a common-word key takes it mid-sentence too, not only
			// at the start of a line.
			name:     "common word key quoted mid-sentence",
			input:    `Please set token="my note" before continuing`,
			expected: `Please set token="[REDACTED]" before continuing`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, config.RedactText(tt.input))
		})
	}
}

// TestRedactText_LongTextUnchanged tests that a long text holding nothing to
// redact is returned unchanged.
func TestRedactText_LongTextUnchanged(t *testing.T) {
	config := DefaultConfig()

	const line = "lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod\n"
	var sb strings.Builder
	for sb.Len() < 10*1024 {
		sb.WriteString(line)
	}
	input := sb.String()

	assert.Equal(t, input, config.RedactText(input))
}

// TestRedactLogAttribute_SensitiveKeys tests redaction of sensitive key names
func TestRedactLogAttribute_SensitiveKeys(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "password key",
			key:      "password",
			value:    "secret123",
			expected: "[REDACTED]",
		},
		{
			name:     "token key",
			key:      "token",
			value:    "abc123",
			expected: "[REDACTED]",
		},
		{
			name:     "api_key",
			key:      "api_key",
			value:    "xyz",
			expected: "[REDACTED]",
		},
		{
			name:     "secret key",
			key:      "secret",
			value:    "mysecret",
			expected: "[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: slog.StringValue(tt.value)}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.expected, result.Value.String())
		})
	}
}

// TestRedactLogAttribute_NormalKeys tests that normal keys are preserved
func TestRedactLogAttribute_NormalKeys(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "username",
			key:   "username",
			value: "john",
		},
		{
			name:  "message",
			key:   "message",
			value: "hello world",
		},
		{
			name:  "count",
			key:   "count",
			value: "42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: slog.StringValue(tt.value)}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.value, result.Value.String())
		})
	}
}

// TestRedactLogAttribute_SensitiveValues tests redaction based on value content
func TestRedactLogAttribute_SensitiveValues(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "value contains 'password'",
			key:      "field",
			value:    "my_password_123",
			expected: "[REDACTED]",
		},
		{
			name:     "value contains 'token'",
			key:      "data",
			value:    "bearer_token_xyz",
			expected: "[REDACTED]",
		},
		{
			name:     "normal value",
			key:      "field",
			value:    "normal_value",
			expected: "normal_value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: slog.StringValue(tt.value)}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.expected, result.Value.String())
		})
	}
}

// TestRedactLogAttribute_GroupValues tests nested group handling
func TestRedactLogAttribute_GroupValues(t *testing.T) {
	config := DefaultConfig()

	t.Run("simple group with sensitive data", func(t *testing.T) {
		innerAttrs := []slog.Attr{
			{Key: "password", Value: slog.StringValue("secret")},
			{Key: "username", Value: slog.StringValue("john")},
		}
		attr := slog.Attr{Key: "credentials", Value: slog.GroupValue(innerAttrs...)}

		result := config.RedactLogAttribute(attr)
		assert.Equal(t, "credentials", result.Key)
		assert.Equal(t, slog.KindGroup, result.Value.Kind())

		groupAttrs := result.Value.Group()
		require.Len(t, groupAttrs, 2)
		assert.Equal(t, "password", groupAttrs[0].Key)
		assert.Equal(t, "[REDACTED]", groupAttrs[0].Value.String())
		assert.Equal(t, "username", groupAttrs[1].Key)
		assert.Equal(t, "john", groupAttrs[1].Value.String())
	})

	t.Run("nested groups", func(t *testing.T) {
		deepInnerAttrs := []slog.Attr{
			{Key: "token", Value: slog.StringValue("abc123")},
		}
		innerAttrs := []slog.Attr{
			{Key: "auth", Value: slog.GroupValue(deepInnerAttrs...)},
			{Key: "id", Value: slog.StringValue("123")},
		}
		attr := slog.Attr{Key: "request", Value: slog.GroupValue(innerAttrs...)}

		result := config.RedactLogAttribute(attr)
		assert.Equal(t, "request", result.Key)

		groupAttrs := result.Value.Group()
		require.Len(t, groupAttrs, 2)

		// Check nested group
		authGroup := groupAttrs[0]
		assert.Equal(t, "auth", authGroup.Key)
		assert.Equal(t, slog.KindGroup, authGroup.Value.Kind())

		authAttrs := authGroup.Value.Group()
		require.Len(t, authAttrs, 1)
		assert.Equal(t, "token", authAttrs[0].Key)
		assert.Equal(t, "[REDACTED]", authAttrs[0].Value.String())
	})
}

// TestRedactLogAttribute_NonStringValues tests that non-string values are preserved
func TestRedactLogAttribute_NonStringValues(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name  string
		key   string
		value slog.Value
	}{
		{
			name:  "integer value",
			key:   "count",
			value: slog.IntValue(42),
		},
		{
			name:  "boolean value",
			key:   "enabled",
			value: slog.BoolValue(true),
		},
		{
			name:  "float value",
			key:   "ratio",
			value: slog.Float64Value(3.14),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: tt.value}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.value.Kind(), result.Value.Kind())
			assert.Equal(t, tt.value, result.Value)
		})
	}
}

// mockHandler is a simple mock implementation of slog.Handler for testing
type mockHandler struct {
	enabled      bool
	records      []slog.Record
	attrs        []slog.Attr
	groups       []string
	enabledLevel slog.Level
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		enabled:      true,
		records:      make([]slog.Record, 0),
		attrs:        make([]slog.Attr, 0),
		groups:       make([]string, 0),
		enabledLevel: slog.LevelInfo,
	}
}

func (m *mockHandler) Enabled(_ context.Context, level slog.Level) bool {
	return m.enabled && level >= m.enabledLevel
}

func (m *mockHandler) Handle(_ context.Context, record slog.Record) error {
	m.records = append(m.records, record)
	return nil
}

func (m *mockHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := &mockHandler{
		enabled:      m.enabled,
		records:      m.records,
		attrs:        append(m.attrs, attrs...),
		groups:       m.groups,
		enabledLevel: m.enabledLevel,
	}
	return newHandler
}

func (m *mockHandler) WithGroup(name string) slog.Handler {
	newHandler := &mockHandler{
		enabled:      m.enabled,
		records:      m.records,
		attrs:        m.attrs,
		groups:       append(m.groups, name),
		enabledLevel: m.enabledLevel,
	}
	return newHandler
}

// TestNewRedactingHandler tests handler creation
func TestNewRedactingHandler(t *testing.T) {
	t.Run("with custom config", func(t *testing.T) {
		mock := newMockHandler()
		config := DefaultConfig()
		handler := NewRedactingHandler(mock, config, nil)

		assert.NotNil(t, handler)
		assert.Equal(t, mock, handler.handler)
		assert.Equal(t, config, handler.config)
	})

	t.Run("with nil config uses default", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, nil, nil)

		assert.NotNil(t, handler)
		assert.NotNil(t, handler.config)
		assert.Equal(t, "[REDACTED]", handler.config.Placeholder)
	})
}

// TestRedactingHandler_Enabled tests Enabled method
func TestRedactingHandler_Enabled(t *testing.T) {
	mock := newMockHandler()
	mock.enabledLevel = slog.LevelWarn
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	ctx := context.Background()

	assert.False(t, handler.Enabled(ctx, slog.LevelDebug))
	assert.False(t, handler.Enabled(ctx, slog.LevelInfo))
	assert.True(t, handler.Enabled(ctx, slog.LevelWarn))
	assert.True(t, handler.Enabled(ctx, slog.LevelError))
}

// TestRedactingHandler_Handler tests Handler getter
func TestRedactingHandler_Handler(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	assert.Equal(t, mock, handler.Handler())
}

// TestRedactingHandler_Handle tests log record handling with redaction
func TestRedactingHandler_Handle(t *testing.T) {
	t.Run("redacts sensitive attributes", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		ctx := context.Background()
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
		record.AddAttrs(
			slog.String("username", "john"),
			slog.String("password", "secret123"),
		)

		err := handler.Handle(ctx, record)
		require.NoError(t, err)
		require.Len(t, mock.records, 1)

		handledRecord := mock.records[0]
		attrs := make([]slog.Attr, 0)
		handledRecord.Attrs(func(attr slog.Attr) bool {
			attrs = append(attrs, attr)
			return true
		})

		require.Len(t, attrs, 2)
		assert.Equal(t, "username", attrs[0].Key)
		assert.Equal(t, "john", attrs[0].Value.String())
		assert.Equal(t, "password", attrs[1].Key)
		assert.Equal(t, "[REDACTED]", attrs[1].Value.String())
	})

	t.Run("preserves record metadata", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		ctx := context.Background()
		originalTime := time.Now()
		record := slog.NewRecord(originalTime, slog.LevelWarn, "warning message", 123)

		err := handler.Handle(ctx, record)
		require.NoError(t, err)
		require.Len(t, mock.records, 1)

		handledRecord := mock.records[0]
		assert.Equal(t, originalTime, handledRecord.Time)
		assert.Equal(t, slog.LevelWarn, handledRecord.Level)
		assert.Equal(t, "warning message", handledRecord.Message)
		assert.Equal(t, uintptr(123), handledRecord.PC)
	})
}

// TestRedactingHandler_Handle_MessageRedaction tests that record.Message is redacted
func TestRedactingHandler_Handle_MessageRedaction(t *testing.T) {
	t.Run("redacts key=value sensitive content in message", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		ctx := context.Background()
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "Connecting with password=secret123 to server", 0)

		err := handler.Handle(ctx, record)
		require.NoError(t, err)
		require.Len(t, mock.records, 1)

		handledRecord := mock.records[0]
		assert.NotContains(t, handledRecord.Message, "secret123")
		assert.Contains(t, handledRecord.Message, "password=")
		assert.Contains(t, handledRecord.Message, "[REDACTED]")
	})

	t.Run("redacts value-detected sensitive content in message", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		ctx := context.Background()
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "Session used AKIAIOSFODNN7EXAMPLE without a recognizable key name", 0)

		err := handler.Handle(ctx, record)
		require.NoError(t, err)
		require.Len(t, mock.records, 1)

		handledRecord := mock.records[0]
		assert.NotContains(t, handledRecord.Message, "AKIAIOSFODNN7EXAMPLE")
		assert.Contains(t, handledRecord.Message, "[REDACTED]")
		assert.Contains(t, handledRecord.Message, "key name")
	})

	t.Run("preserves non-sensitive message unchanged", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		ctx := context.Background()
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "User logged in successfully", 0)

		err := handler.Handle(ctx, record)
		require.NoError(t, err)
		require.Len(t, mock.records, 1)

		handledRecord := mock.records[0]
		assert.Equal(t, "User logged in successfully", handledRecord.Message)
	})
}

// TestRedactingHandler_WithAttrs tests attribute addition with redaction
func TestRedactingHandler_WithAttrs(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	newAttrs := []slog.Attr{
		slog.String("token", "abc123"),
		slog.String("user", "alice"),
	}

	newHandler := handler.WithAttrs(newAttrs)

	// Should return a new RedactingHandler
	redactingHandler, ok := newHandler.(*RedactingHandler)
	require.True(t, ok)

	// Check that underlying handler received redacted attributes
	underlyingMock, ok := redactingHandler.handler.(*mockHandler)
	require.True(t, ok)
	require.Len(t, underlyingMock.attrs, 2)

	assert.Equal(t, "token", underlyingMock.attrs[0].Key)
	assert.Equal(t, "[REDACTED]", underlyingMock.attrs[0].Value.String())
	assert.Equal(t, "user", underlyingMock.attrs[1].Key)
	assert.Equal(t, "alice", underlyingMock.attrs[1].Value.String())

	// Original handler should be unchanged
	originalMock, ok := handler.handler.(*mockHandler)
	require.True(t, ok)
	assert.Len(t, originalMock.attrs, 0)
}

// TestRedactingHandler_WithAttrs_LogValuer tests WithAttrs with LogValuer attributes
func TestRedactingHandler_WithAttrs_LogValuer(t *testing.T) {
	tests := []struct {
		name           string
		attr           slog.Attr
		expectedKey    string
		expectedValue  string
		expectRedacted bool
	}{
		{
			name:           "LogValuer with sensitive key",
			attr:           slog.Any("token", sensitiveLogValuer{data: "secret123"}),
			expectedKey:    "token",
			expectedValue:  "[REDACTED]",
			expectRedacted: true,
		},
		{
			name:           "LogValuer with non-sensitive key",
			attr:           slog.Any("user", sensitiveLogValuer{data: "alice"}),
			expectedKey:    "user",
			expectedValue:  "alice",
			expectRedacted: false,
		},
		{
			name:           "LogValuer returning sensitive value",
			attr:           slog.Any("data", sensitiveLogValuer{data: "password=secret"}),
			expectedKey:    "data",
			expectedValue:  "password=[REDACTED]",
			expectRedacted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockHandler()
			handler := NewRedactingHandler(mock, DefaultConfig(), nil)

			newHandler := handler.WithAttrs([]slog.Attr{tt.attr})

			// Should return a new RedactingHandler
			redactingHandler, ok := newHandler.(*RedactingHandler)
			require.True(t, ok)

			// Check that underlying handler received redacted attributes
			underlyingMock, ok := redactingHandler.handler.(*mockHandler)
			require.True(t, ok)
			require.Len(t, underlyingMock.attrs, 1)

			assert.Equal(t, tt.expectedKey, underlyingMock.attrs[0].Key)
			assert.Equal(t, tt.expectedValue, underlyingMock.attrs[0].Value.String())
		})
	}
}

// TestRedactingHandler_WithAttrs_Slice tests WithAttrs with slice attributes
func TestRedactingHandler_WithAttrs_Slice(t *testing.T) {
	t.Run("slice with LogValuer elements", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		// Create a slice with LogValuer elements
		newHandler := handler.WithAttrs([]slog.Attr{
			slog.Any("users", []slog.LogValuer{
				sensitiveLogValuer{data: "alice"},
				sensitiveLogValuer{data: "bob"},
			}),
		})

		// Should return a new RedactingHandler
		redactingHandler, ok := newHandler.(*RedactingHandler)
		require.True(t, ok)

		// Check that underlying handler received the attribute
		underlyingMock, ok := redactingHandler.handler.(*mockHandler)
		require.True(t, ok)
		require.Len(t, underlyingMock.attrs, 1)

		assert.Equal(t, "users", underlyingMock.attrs[0].Key)

		// Check that the slice was processed - it should be []any now
		sliceValue := underlyingMock.attrs[0].Value.Any()
		require.NotNil(t, sliceValue)

		sliceAny, ok := sliceValue.([]any)
		require.True(t, ok, "expected []any after processing LogValuer slice, got %T", sliceValue)
		assert.Len(t, sliceAny, 2)
	})

	t.Run("slice with sensitive LogValuer - key based redaction", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)

		// Use "token" as the key which is sensitive
		newHandler := handler.WithAttrs([]slog.Attr{
			slog.Any("token", []slog.LogValuer{
				sensitiveLogValuer{data: "token123"},
			}),
		})

		// Should return a new RedactingHandler
		redactingHandler, ok := newHandler.(*RedactingHandler)
		require.True(t, ok)

		// Check that underlying handler received the attribute
		underlyingMock, ok := redactingHandler.handler.(*mockHandler)
		require.True(t, ok)
		require.Len(t, underlyingMock.attrs, 1)

		assert.Equal(t, "token", underlyingMock.attrs[0].Key)
		// The entire attribute should be redacted because "token" is a sensitive key
		assert.Equal(t, "[REDACTED]", underlyingMock.attrs[0].Value.String())
	})
}

// TestRedactingHandler_WithAttrs_PanicRecovery tests panic recovery in WithAttrs
func TestRedactingHandler_WithAttrs_PanicRecovery(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	// Should not panic even if LogValue() panics
	newHandler := handler.WithAttrs([]slog.Attr{
		slog.Any("panic_attr", panickingLogValuer{}),
	})

	// Should return a new RedactingHandler
	redactingHandler, ok := newHandler.(*RedactingHandler)
	require.True(t, ok)

	// Check that underlying handler received safe placeholder
	underlyingMock, ok := redactingHandler.handler.(*mockHandler)
	require.True(t, ok)
	require.Len(t, underlyingMock.attrs, 1)

	assert.Equal(t, "panic_attr", underlyingMock.attrs[0].Key)
	assert.Equal(t, RedactionFailurePlaceholder, underlyingMock.attrs[0].Value.String())
}

// TestRedactingHandler_WithGroup tests group creation
func TestRedactingHandler_WithGroup(t *testing.T) {
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, DefaultConfig(), nil)

	newHandler := handler.WithGroup("request")

	// Should return a new RedactingHandler
	redactingHandler, ok := newHandler.(*RedactingHandler)
	require.True(t, ok)

	// Check that underlying handler has the group
	underlyingMock, ok := redactingHandler.handler.(*mockHandler)
	require.True(t, ok)
	require.Len(t, underlyingMock.groups, 1)
	assert.Equal(t, "request", underlyingMock.groups[0])

	// Original handler should be unchanged
	originalMock, ok := handler.handler.(*mockHandler)
	require.True(t, ok)
	assert.Len(t, originalMock.groups, 0)
}

// TestPerformKeyValueRedaction tests the routing logic
func TestPerformKeyValueRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		text        string
		key         string
		placeholder string
		expected    string
	}{
		{
			name:        "colon pattern",
			text:        "Authorization: Bearer token",
			key:         "Authorization: ",
			placeholder: "[REDACTED]",
			expected:    "Authorization: Bearer [REDACTED]",
		},
		{
			name:        "space pattern",
			text:        "Bearer token123",
			key:         "Bearer ",
			placeholder: "[REDACTED]",
			expected:    "Bearer [REDACTED]",
		},
		{
			name:        "key=value pattern",
			text:        "password=secret",
			key:         "password",
			placeholder: "[REDACTED]",
			expected:    "password=[REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.performKeyValueRedaction(tt.text, tt.key, tt.placeholder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPerformSpacePatternRedaction tests space pattern handling details
func TestPerformSpacePatternRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		text        string
		pattern     string
		placeholder string
		expected    string
	}{
		{
			name:        "simple Bearer",
			text:        "Bearer abc123",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "Bearer ***",
		},
		{
			name:        "case insensitive",
			text:        "bearer token",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "bearer ***",
		},
		{
			name:        "preserves original case",
			text:        "BeArEr secret",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "BeArEr ***",
		},
		{
			name:        "multiple occurrences",
			text:        "Bearer abc Bearer xyz",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "Bearer *** Bearer ***",
		},
		{
			name:        "no match returns original",
			text:        "no match here",
			pattern:     "Bearer ",
			placeholder: "***",
			expected:    "no match here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.performSpacePatternRedaction(tt.text, tt.pattern, tt.placeholder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPerformColonPatternRedaction tests colon pattern handling details
func TestPerformColonPatternRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		text        string
		pattern     string
		placeholder string
		expected    string
	}{
		{
			name:        "with Bearer scheme",
			text:        "Authorization: Bearer token123",
			pattern:     "Authorization: ",
			placeholder: "***",
			expected:    "Authorization: Bearer ***",
		},
		{
			name:        "with Basic scheme",
			text:        "Authorization: Basic dGVzdA==",
			pattern:     "Authorization: ",
			placeholder: "***",
			expected:    "Authorization: Basic ***",
		},
		{
			name:        "no scheme",
			text:        "Authorization: token123",
			pattern:     "Authorization: ",
			placeholder: "***",
			expected:    "Authorization: ***",
		},
		{
			name:        "no space after colon",
			text:        "Authorization:token",
			pattern:     "Authorization:",
			placeholder: "***",
			expected:    "Authorization:***",
		},
		{
			name:        "with space after colon",
			text:        "Authorization: token",
			pattern:     "Authorization:",
			placeholder: "***",
			expected:    "Authorization: ***",
		},
		{
			name:        "case insensitive pattern",
			text:        "authorization: bearer secret",
			pattern:     "Authorization: ",
			placeholder: "***",
			expected:    "authorization: bearer ***",
		},
		{
			name:        "preserves whitespace",
			text:        "Authorization:\t\tBearer token",
			pattern:     "Authorization:",
			placeholder: "***",
			expected:    "Authorization:\t\tBearer ***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.performColonPatternRedaction(tt.text, tt.pattern, tt.placeholder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestPerformKeyValuePatternRedaction tests key=value pattern handling details
func TestPerformKeyValuePatternRedaction(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name        string
		text        string
		key         string
		placeholder string
		expected    string
	}{
		{
			name:        "simple key=value",
			text:        "password=secret",
			key:         "password",
			placeholder: "***",
			expected:    "password=***",
		},
		{
			name:        "key with equals",
			text:        "Authorization=Bearer token",
			key:         "Authorization=",
			placeholder: "***",
			expected:    "Authorization=*** token", // Only "Bearer" is redacted, " token" remains
		},
		{
			name:        "case insensitive",
			text:        "PASSWORD=secret",
			key:         "password",
			placeholder: "***",
			expected:    "PASSWORD=***",
		},
		{
			name:        "preserves case",
			text:        "PaSsWoRd=test",
			key:         "password",
			placeholder: "***",
			expected:    "PaSsWoRd=***",
		},
		{
			name:        "multiple matches",
			text:        "password=abc token=xyz password=def",
			key:         "password",
			placeholder: "***",
			expected:    "password=*** token=xyz password=***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.performKeyValuePatternRedaction(tt.text, tt.key, tt.placeholder)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestRedactLogAttribute_StringWithKeyValuePatterns tests that log attributes containing
// key=value patterns in their string values are properly redacted
func TestRedactLogAttribute_StringWithKeyValuePatterns(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name     string
		key      string
		value    string
		expected string
	}{
		{
			name:     "stdout with password",
			key:      "stdout",
			value:    "Connecting with password=secret123 to server",
			expected: "Connecting with password=[REDACTED] to server",
		},
		{
			name:     "stderr with token",
			key:      "stderr",
			value:    "Error: token=abc123 is invalid",
			expected: "Error: token=[REDACTED] is invalid",
		},
		{
			name:     "output with Bearer token",
			key:      "output",
			value:    "Authorization: Bearer token123",
			expected: "Authorization: Bearer [REDACTED]",
		},
		{
			name:     "output with multiple secrets",
			key:      "message",
			value:    "password=pass123 api_key=key456 normal=value",
			expected: "password=[REDACTED] api_key=[REDACTED] normal=value",
		},
		{
			name:     "normal output without secrets",
			key:      "stdout",
			value:    "Build completed successfully",
			expected: "Build completed successfully",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attr := slog.Attr{Key: tt.key, Value: slog.StringValue(tt.value)}
			result := config.RedactLogAttribute(attr)
			assert.Equal(t, tt.key, result.Key)
			assert.Equal(t, tt.expected, result.Value.String())
		})
	}
}

// TestRedactingHandler_LogValuerSingle tests redaction of a single LogValuer
func TestRedactingHandler_LogValuerSingle(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test data with a LogValuer that contains sensitive information
	testValuer := sensitiveLogValuer{data: "password=secret123"}

	// Execute
	logger.Info("Command executed", "result", testValuer)

	// Verify the sensitive data is redacted
	output := buf.String()
	assert.Contains(t, output, "password=[REDACTED]")
	assert.NotContains(t, output, "secret123")
}

// commandResultMock is a helper struct for testing LogValuer with CommandResult-like data.
type commandResultMock struct {
	Name   string
	Output string
}

// LogValue implements the slog.LogValuer interface.
func (c commandResultMock) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", c.Name),
		slog.String("output", c.Output),
	)
}

// Test LogValuer with actual CommandResult-like struct
func TestRedactingHandler_LogValuerWithCommandResult(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Create a LogValuer that returns sensitive data
	result := commandResultMock{
		Name:   "test_cmd",
		Output: "password=secret123 and token=abc456",
	}

	// Log the CommandResult using LogValuer interface
	logger.Info("Command executed", "result", result)

	// Verify
	output := buf.String()
	assert.Contains(t, output, "password=[REDACTED]")
	assert.Contains(t, output, "token=[REDACTED]")
	assert.NotContains(t, output, "secret123")
	assert.NotContains(t, output, "abc456")
}

func TestRedactingHandler_CommandResults_Integration(t *testing.T) {
	tests := []struct {
		name     string
		results  common.CommandResults
		validate func(t *testing.T, output string)
	}{
		{
			name: "redact password in output",
			results: common.CommandResults{
				{CommandResultFields: common.CommandResultFields{
					Name:     "setup",
					ExitCode: 0,
					Output:   "Database password=secret123 configured",
					Stderr:   "",
				}},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "[REDACTED]")
				assert.NotContains(t, output, "secret123")
				assert.Contains(t, output, "Database")
				assert.Contains(t, output, "configured")
			},
		},
		{
			name: "redact multiple sensitive fields",
			results: common.CommandResults{
				{CommandResultFields: common.CommandResultFields{
					Name:     "deploy",
					ExitCode: 0,
					Output:   "API key=sk-1234567890abcdef set",
					Stderr:   "",
				}},
				{CommandResultFields: common.CommandResultFields{
					Name:     "configure",
					ExitCode: 0,
					Output:   "",
					Stderr:   "Warning: token=ghp_xxxxxxxxxxxx expired",
				}},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "[REDACTED]")
				assert.NotContains(t, output, "sk-1234567890abcdef")
				assert.NotContains(t, output, "ghp_xxxxxxxxxxxx")
				assert.Contains(t, output, "API")
				assert.Contains(t, output, "Warning")
			},
		},
		{
			name: "preserve non-sensitive output",
			results: common.CommandResults{
				{CommandResultFields: common.CommandResultFields{
					Name:     "test",
					ExitCode: 0,
					Output:   "All tests passed",
					Stderr:   "",
				}},
			},
			validate: func(t *testing.T, output string) {
				assert.Contains(t, output, "All tests passed")
				assert.NotContains(t, output, "[REDACTED]")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			handler := slog.NewJSONHandler(&buf, nil)
			config := DefaultConfig()
			redactingHandler := NewRedactingHandler(handler, config, nil)
			logger := slog.New(redactingHandler)

			logger.Info(
				"test",
				slog.String(common.GroupSummaryAttrs.Status, "success"),
				slog.String(common.GroupSummaryAttrs.Group, "test_group"),
				slog.Any(common.GroupSummaryAttrs.Commands, tt.results),
			)

			output := buf.String()
			tt.validate(t, output)
		})
	}
}

// TestRedactingHandler_DeepRecursion tests recursion depth limiting
func TestRedactingHandler_DeepRecursion(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Create deeply nested group structure (depth > 10)
	var createNestedGroup func(int) slog.Value
	createNestedGroup = func(depth int) slog.Value {
		if depth == 0 {
			return slog.StringValue("leaf_value")
		}
		return slog.GroupValue(
			slog.Attr{Key: "level", Value: slog.Int64Value(int64(depth))},
			slog.Attr{Key: "nested", Value: createNestedGroup(depth - 1)},
		)
	}

	// Create structure with depth 15
	deepGroup := createNestedGroup(15)

	// Execute
	logger.Info("Deep structure", "data", deepGroup)

	// Verify: Should handle without panic
	// The depth limit prevents infinite recursion
	output := buf.String()
	assert.Contains(t, output, "level")
}

// TestRedactingHandler_PanicHandling tests our panic recovery when logging a panicking LogValuer
func TestRedactingHandler_PanicHandling(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()

	// Create failure logger to capture panic logs
	var failureBuf bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureBuf, nil)
	failureLogger := slog.New(failureHandler)

	redactingHandler := NewRedactingHandler(handler, config, failureLogger)
	logger := slog.New(redactingHandler)

	// Execute with a LogValuer that is designed to panic.
	// Note: The `panickingLogValuer` type with its `LogValue()` method
	// is defined at the top level of this test file.
	//
	// Our RedactingHandler now handles KindLogValuer and catches panics,
	// replacing them with RedactionFailurePlaceholder and logging to failureLogger.
	logger.Info("Test message", "data", panickingLogValuer{})

	// Verify main log contains placeholder and not the panic message
	output := buf.String()
	assert.Contains(t, output, RedactionFailurePlaceholder)
	assert.NotContains(t, output, "test panic")
	assert.NotContains(t, output, "LogValue panicked")

	// Verify failure log contains detailed panic info
	failureOutput := failureBuf.String()
	assert.Contains(t, failureOutput, "Redaction failed - detailed log")
	assert.Contains(t, failureOutput, "test panic")
	assert.Contains(t, failureOutput, "panic_value")
	assert.Contains(t, failureOutput, "panic_type")
	assert.Contains(t, failureOutput, "stack_trace")
}

// TestRedactingHandler_PanicInProcessKindAny tests our custom panic recovery
// when we manually process KindAny LogValuers (e.g., in groups)
func TestRedactingHandler_PanicInProcessKindAny(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()

	// Create failure logger to capture panic logs
	var failureBuf bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureBuf, nil)
	failureLogger := slog.New(failureHandler)

	redactingHandler := NewRedactingHandler(handler, config, failureLogger)

	// Create a KindAny attribute with an unresolved panicking LogValuer
	// We use slog.Attr directly with AnyValue to avoid premature resolution
	attr := slog.Attr{
		Key:   "test_data",
		Value: slog.AnyValue(panickingLogValuer{}),
	}

	// Process the attribute through WithAttrs public API
	// This will trigger our panic recovery code in processLogValuer()
	handlerWithAttrs := redactingHandler.WithAttrs([]slog.Attr{attr})
	logger := slog.New(handlerWithAttrs)

	// Log a message to trigger the attribute rendering
	logger.Info("Test message")

	// Verify main log contains placeholder
	output := buf.String()
	assert.Contains(t, output, "Test message")
	assert.Contains(t, output, RedactionFailurePlaceholder)
	assert.NotContains(t, output, "test panic")

	// Verify failure log contains detailed panic info
	failureOutput := failureBuf.String()
	assert.Contains(t, failureOutput, "Redaction failed - detailed log")
	assert.Contains(t, failureOutput, "test panic")
	assert.Contains(t, failureOutput, "panic_value")
	assert.Contains(t, failureOutput, "panic_type")
	assert.Contains(t, failureOutput, "stack_trace")
}

// TestRedactingHandler_NilValue tests nil value handling
func TestRedactingHandler_NilValue(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test with nil value
	logger.Info("Test message", "data", slog.AnyValue(nil))

	// Verify: Should handle nil gracefully
	output := buf.String()
	assert.Contains(t, output, "Test message")
}

// TestRedactingHandler_EmptySlice tests empty slice handling
func TestRedactingHandler_EmptySlice(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test with empty slice
	emptySlice := []string{}
	logger.Info("Test message", "data", slog.AnyValue(emptySlice))

	// Verify: Should handle empty slice gracefully
	output := buf.String()
	assert.Contains(t, output, "Test message")
}

// TestRedactingHandler_MixedSlice tests slice with mixed types
func TestRedactingHandler_MixedSlice(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test with mixed slice (interfaces)
	mixedSlice := []any{
		"string_value",
		123,
		true,
	}
	logger.Info("Test message", "data", slog.AnyValue(mixedSlice))

	// Verify: Should handle mixed slice gracefully
	output := buf.String()
	assert.Contains(t, output, "Test message")
	assert.Contains(t, output, "string_value")
}

// TestRedactingHandler_NonLogValuer tests non-LogValuer types pass through
func TestRedactingHandler_NonLogValuer(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	config := DefaultConfig()
	redactingHandler := NewRedactingHandler(handler, config, nil)
	logger := slog.New(redactingHandler)

	// Test with various non-LogValuer types
	logger.Info(
		"Test message",
		"int", slog.IntValue(123),
		"bool", slog.BoolValue(true),
		"float", slog.Float64Value(3.14),
	)

	// Verify: Should pass through without modification
	output := buf.String()
	assert.Contains(t, output, "123")
	assert.Contains(t, output, "true")
	assert.Contains(t, output, "3.14")
}

// TestRedactionContext_DepthTracking tests depth tracking
func TestRedactionContext_DepthTracking(t *testing.T) {
	ctx1 := redactionContext{depth: 0}
	assert.Equal(t, 0, ctx1.depth)

	ctx2 := redactionContext{depth: 5}
	assert.Equal(t, 5, ctx2.depth)

	// Test depth limit
	assert.True(t, ctx2.depth < maxRedactionDepth)

	ctxLimit := redactionContext{depth: maxRedactionDepth}
	assert.Equal(t, maxRedactionDepth, ctxLimit.depth)
}

// TestRedactionFailurePlaceholder tests the failure placeholder constant
func TestRedactionFailurePlaceholder(t *testing.T) {
	assert.Equal(t, "[REDACTION FAILED - OUTPUT SUPPRESSED]", RedactionFailurePlaceholder)
	assert.NotEqual(t, "[REDACTED]", RedactionFailurePlaceholder)
}

// TestMaxRedactionDepth tests the depth limit constant
func TestMaxRedactionDepth(t *testing.T) {
	assert.Equal(t, 10, maxRedactionDepth)
	assert.True(t, maxRedactionDepth > 0)
}

// TestRedactingHandler_SliceTypeConversion tests and documents the type conversion behavior
// for slices processed by the redacting handler
func TestRedactingHandler_SliceTypeConversion(t *testing.T) {
	t.Run("typed slice without LogValuer converts to []any", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// Test with a typed slice that has no LogValuer elements
		stringSlice := []string{"alice", "bob", "charlie"}
		logger.Info("Test message", "users", slog.AnyValue(stringSlice))

		// Verify: Even without LogValuer, processSlice converts to []any
		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var usersAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "users" {
				usersAttr = attr
				return false
			}
			return true
		})

		// ALL slices are processed and converted to []any
		sliceValue := usersAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any after processSlice, got %T", sliceValue)
		assert.Len(t, anySlice, 3)

		// Verify semantic content is preserved
		assert.Equal(t, "alice", anySlice[0])
		assert.Equal(t, "bob", anySlice[1])
		assert.Equal(t, "charlie", anySlice[2])
	})

	t.Run("slice with LogValuer converts to []any", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// Test with a slice containing LogValuer elements
		logValuerSlice := []slog.LogValuer{
			sensitiveLogValuer{data: "alice"},
			sensitiveLogValuer{data: "bob"},
		}
		logger.Info("Test message", "users", slog.AnyValue(logValuerSlice))

		// Verify: Should convert to []any after processing
		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var usersAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "users" {
				usersAttr = attr
				return false
			}
			return true
		})

		// After processSlice, the type should be []any
		sliceValue := usersAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any after processing LogValuer slice, got %T", sliceValue)
		assert.Len(t, anySlice, 2)

		// Verify that the semantic content is preserved
		// (even though the type changed from []slog.LogValuer to []any)
		assert.Equal(t, "alice", anySlice[0])
		assert.Equal(t, "bob", anySlice[1])
	})

	t.Run("mixed slice type conversion", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// Test with interface slice containing some LogValuers
		mixedSlice := []any{
			sensitiveLogValuer{data: "alice"},
			"plain_string",
			123,
		}
		logger.Info("Test message", "data", slog.AnyValue(mixedSlice))

		// Verify: []any is similar to []any, should handle gracefully
		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var dataAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "data" {
				dataAttr = attr
				return false
			}
			return true
		})

		sliceValue := dataAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any for processed mixed slice, got %T", sliceValue)
		assert.Len(t, anySlice, 3)

		// First element was LogValuer -> resolved to its string value
		assert.Equal(t, "alice", anySlice[0])
		// Other elements preserved as-is
		assert.Equal(t, "plain_string", anySlice[1])
		assert.Equal(t, 123, anySlice[2])
	})

	t.Run("array with string elements - element-wise redaction", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// Test with array (fixed-size) containing sensitive strings
		// Arrays should be processed element-wise just like slices
		stringArray := [3]string{"password=secret123", "token=abc456", "normal_value"}
		logger.Info("Test message", "data", slog.AnyValue(stringArray))

		// Verify: array should be processed and converted to []any
		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var dataAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "data" {
				dataAttr = attr
				return false
			}
			return true
		})

		// Array should be converted to []any after processing
		sliceValue := dataAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any after processing array, got %T", sliceValue)
		assert.Len(t, anySlice, 3)

		// Verify each element was redacted appropriately
		assert.Equal(t, "password=[REDACTED]", anySlice[0], "Array element with password pattern should be redacted")
		assert.Equal(t, "token=[REDACTED]", anySlice[1], "Array element with token pattern should be redacted")
		assert.Equal(t, "normal_value", anySlice[2], "Normal array element should be preserved")
	})

	t.Run("array with mixed types and sensitive content", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// Test with array of any containing different types
		type mixedArray [4]any
		arr := mixedArray{
			"password=secret",
			123,
			"normal_text",
			45.6,
		}
		logger.Info("Test message", "data", slog.AnyValue(arr))

		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var dataAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "data" {
				dataAttr = attr
				return false
			}
			return true
		})

		sliceValue := dataAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any after processing array with mixed types, got %T", sliceValue)
		assert.Len(t, anySlice, 4)

		// First element with sensitive key=value should be redacted
		assert.Equal(t, "password=[REDACTED]", anySlice[0])
		// Non-string primitives should be preserved
		assert.Equal(t, 123, anySlice[1])
		assert.Equal(t, "normal_text", anySlice[2])
		assert.Equal(t, 45.6, anySlice[3])
	})

	t.Run("slice of arrays - nested array elements are redacted", func(t *testing.T) {
		mock := newMockHandler()
		handler := NewRedactingHandler(mock, DefaultConfig(), nil)
		logger := slog.New(handler)

		// A slice whose elements are themselves fixed-size arrays. Each element
		// must be recursed into individually, not passed through as opaque data.
		nested := [][2]string{
			{"password=secret1", "normal1"},
			{"token=secret2", "normal2"},
		}
		logger.Info("Test message", "data", slog.AnyValue(nested))

		require.Len(t, mock.records, 1)
		record := mock.records[0]

		var dataAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "data" {
				dataAttr = attr
				return false
			}
			return true
		})

		sliceValue := dataAttr.Value.Any()
		anySlice, ok := sliceValue.([]any)
		assert.True(t, ok, "Expected []any after processing slice of arrays, got %T", sliceValue)
		assert.Len(t, anySlice, 2)

		firstElem, ok := anySlice[0].([]any)
		assert.True(t, ok, "Expected nested array to be converted to []any, got %T", anySlice[0])
		assert.Equal(t, []any{"password=[REDACTED]", "normal1"}, firstElem)

		secondElem, ok := anySlice[1].([]any)
		assert.True(t, ok, "Expected nested array to be converted to []any, got %T", anySlice[1])
		assert.Equal(t, []any{"token=[REDACTED]", "normal2"}, secondElem)
	})
}

// TestRedactingHandler_MapRedaction tests map redaction functionality
func TestRedactingHandler_MapRedaction(t *testing.T) {
	config := DefaultConfig()

	t.Run("SensitiveKeyMasking", func(t *testing.T) {
		// Test that sensitive keys in maps are masked
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		// Log a map with a sensitive key
		sensitiveMap := map[string]any{
			"api_key": "secret-value-123",
		}
		logger.Info("Test message", "details", slog.AnyValue(sensitiveMap))

		// Parse JSON output
		output := buf.String()
		var logEntry map[string]any
		err := json.Unmarshal([]byte(output), &logEntry)
		require.NoError(t, err)

		// Check that secret value is not present
		assert.NotContains(t, output, "secret-value-123", "Sensitive value should be redacted")
		assert.Contains(t, output, "[REDACTED]", "Should contain redaction placeholder")

		// Positive control: verify secret appears via non-redacting JSON handler
		var controlBuf bytes.Buffer
		controlHandler := slog.NewJSONHandler(&controlBuf, nil)
		controlLogger := slog.New(controlHandler)
		controlLogger.Info("Test message", "details", slog.AnyValue(sensitiveMap))
		controlOutput := controlBuf.String()
		assert.Contains(t, controlOutput, "secret-value-123", "Secret should appear in non-redacting handler output, confirming redaction is actually preventing leakage")
	})

	t.Run("ValueContentDetection", func(t *testing.T) {
		// Test that sensitive patterns in values are detected and masked
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		// Log a map with sensitive content in value
		dataMap := map[string]any{
			"note": "password=hunter2",
		}
		logger.Info("Test message", "details", slog.AnyValue(dataMap))

		output := buf.String()
		assert.NotContains(t, output, "hunter2", "Sensitive value content should be redacted")
		assert.Contains(t, output, "[REDACTED]", "Should contain redaction placeholder")
	})

	t.Run("NestedMap", func(t *testing.T) {
		// Test that nested maps are recursively redacted
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		nestedMap := map[string]any{
			"outer": map[string]any{
				"token": "secret-token-xyz",
			},
		}
		logger.Info("Test message", "details", slog.AnyValue(nestedMap))

		output := buf.String()
		assert.NotContains(t, output, "secret-token-xyz", "Nested sensitive value should be redacted")
	})

	t.Run("DepthLimit", func(t *testing.T) {
		// Test that depth limit is respected
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		// Create deeply nested map to exceed depth limit
		deepMap := map[string]any{
			"level1": map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"level4": map[string]any{
							"level5": map[string]any{
								"level6": map[string]any{
									"level7": map[string]any{
										"level8": map[string]any{
											"level9": map[string]any{
												"level10": map[string]any{
													"level11": "this should be redacted due to depth limit",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		logger.Info("Test message", "deep", slog.AnyValue(deepMap))

		output := buf.String()
		// At depth limit, should return placeholder
		assert.Contains(t, output, "[REDACTION FAILED - OUTPUT SUPPRESSED]")
		// The depth-limited leaf value must not leak anywhere in the output, even
		// though the placeholder is present (regression guard: placeholder emitted
		// but the suppressed value still leaks elsewhere).
		assert.NotContains(t, output, "this should be redacted due to depth limit")
	})

	t.Run("NoSensitiveContent", func(t *testing.T) {
		// Test that non-sensitive maps maintain their content
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		nonSensitiveMap := map[string]any{
			"name": "John",
			"age":  30,
			"city": "San Francisco",
		}
		logger.Info("Test message", "user", slog.AnyValue(nonSensitiveMap))

		output := buf.String()
		assert.Contains(t, output, "John", "Non-sensitive string should be preserved")
		assert.Contains(t, output, "San Francisco", "Non-sensitive values should be preserved")
	})

	t.Run("NonStringKey", func(t *testing.T) {
		// Test that non-string keys are handled
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		// Create map with int keys
		intKeyMap := map[int]string{
			1: "first",
			2: "second",
		}
		logger.Info("Test message", "items", slog.AnyValue(intKeyMap))

		// Should not panic and should output something
		output := buf.String()
		assert.Contains(t, output, "Test message")
	})
}

// TestRedactingHandler_StructRedaction tests struct redaction functionality
func TestRedactingHandler_StructRedaction(t *testing.T) {
	config := DefaultConfig()

	type SensitiveStruct struct {
		APIKey string `json:"api_key"`
		Name   string
		Secret string `json:"secret"`
	}

	t.Run("SensitiveFieldRedaction", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		data := SensitiveStruct{
			APIKey: "secret-api-key-123",
			Name:   "John",
			Secret: "secret-data-456",
		}
		logger.Info("Test message", "data", slog.AnyValue(data))

		output := buf.String()
		assert.NotContains(t, output, "secret-api-key-123", "Sensitive field should be redacted")
		assert.NotContains(t, output, "secret-data-456", "Secret field should be redacted")
		assert.Contains(t, output, "John", "Non-sensitive field should be preserved")
	})

	t.Run("JsonTagFieldNaming", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		data := SensitiveStruct{
			APIKey: "key123",
			Name:   "Jane",
			Secret: "secret123",
		}
		logger.Info("Test message", "data", slog.AnyValue(data))

		output := buf.String()
		// Should use json tag names, not Go field names
		assert.Contains(t, output, "api_key", "JSON tag should be used as field name")
	})

	t.Run("NoSensitiveContent", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		// Use struct with non-sensitive field names to avoid pattern matching
		type NormalStruct struct {
			Username string `json:"username"`
			Email    string `json:"email"`
			Status   string `json:"status"`
		}

		data := NormalStruct{
			Username: "alice",
			Email:    "alice@example.com",
			Status:   "active",
		}
		logger.Info("Test message", "data", slog.AnyValue(data))

		output := buf.String()
		assert.Contains(t, output, "alice")
		assert.Contains(t, output, "alice@example.com")
		assert.Contains(t, output, "active")
	})

	t.Run("DepthLimit", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		// Create a struct with deeply nested field
		type DeepStruct struct {
			Level1 map[string]any `json:"level1"`
		}

		deepData := DeepStruct{
			Level1: map[string]any{
				"level2": map[string]any{
					"level3": map[string]any{
						"level4": map[string]any{
							"level5": map[string]any{
								"level6": map[string]any{
									"level7": map[string]any{
										"level8": map[string]any{
											"level9": map[string]any{
												"level10": map[string]any{
													"level11": "too deep",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		logger.Info("Test message", "data", slog.AnyValue(deepData))
		// Should handle depth limit gracefully
		output := buf.String()
		assert.Contains(t, output, "Test message")
		// Verify depth-limit behavior: the too-deep value should be replaced with placeholder
		assert.Contains(t, output, "[REDACTION FAILED - OUTPUT SUPPRESSED]", "Depth-limited value should be redacted with placeholder")
		assert.NotContains(t, output, "too deep", "Actual too-deep value should not appear in redacted output")
	})
}

// TestRedactingHandler_TwoTierLogging tests that panic handling produces
// two log entries: detailed (to failureLogger) and summary (to slog.Default)
func TestRedactingHandler_TwoTierLogging(t *testing.T) {
	var mainBuf bytes.Buffer
	mainHandler := slog.NewJSONHandler(&mainBuf, nil)
	config := DefaultConfig()

	// Create failure logger (simulates file/stderr, excludes Slack)
	var failureBuf bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureBuf, nil)
	failureLogger := slog.New(failureHandler)

	// Create redacting handler
	redactingHandler := NewRedactingHandler(mainHandler, config, failureLogger)
	logger := slog.New(redactingHandler)

	// Set this logger as default so slog.Warn() in panic handler works
	oldDefault := slog.Default()
	slog.SetDefault(logger)
	defer slog.SetDefault(oldDefault)

	// Trigger panic in LogValuer
	logger.Info("Test message", "data", panickingLogValuer{})

	// Parse main output
	mainLines := strings.Split(strings.TrimSpace(mainBuf.String()), "\n")
	require.GreaterOrEqual(t, len(mainLines), 2, "Expected at least 2 log entries (placeholder + summary)")

	// Parse failure output
	failureLines := strings.Split(strings.TrimSpace(failureBuf.String()), "\n")
	require.GreaterOrEqual(t, len(failureLines), 1, "Expected at least 1 detailed log entry")

	// Verify detailed log (in failureLogger)
	var detailedLog map[string]any
	err := json.Unmarshal([]byte(failureLines[0]), &detailedLog)
	require.NoError(t, err)

	assert.Equal(t, "Redaction failed - detailed log", detailedLog["msg"])
	assert.Contains(t, detailedLog, "panic_value")
	assert.Contains(t, detailedLog, "panic_type")
	assert.Contains(t, detailedLog, "stack_trace")
	assert.Equal(t, "redaction_failure_detail", detailedLog["log_category"])

	// Verify summary log (in main logger via slog.Default)
	// Find the summary log in main output
	var summaryLog map[string]any
	for _, line := range mainLines {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err == nil {
			if msg, ok := entry["msg"].(string); ok && strings.Contains(msg, "see logs for details") {
				summaryLog = entry
				break
			}
		}
	}

	require.NotNil(t, summaryLog, "Expected to find summary log in main output")
	assert.Equal(t, "Redaction failed - see logs for details", summaryLog["msg"])
	assert.Contains(t, summaryLog, "panic_type")
	assert.Equal(t, "redaction_failure_summary", summaryLog["log_category"])
	assert.Equal(t, true, summaryLog["details_in_log"])

	// Verify sensitive information is NOT in summary
	assert.NotContains(t, summaryLog, "panic_value")
	assert.NotContains(t, summaryLog, "stack_trace")
}

// TestRedactingHandler_SliceStringElementRedaction tests recursive redaction of slice elements
func TestRedactingHandler_SliceStringElementRedaction(t *testing.T) {
	config := DefaultConfig()

	t.Run("SensitiveStringElement", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		args := []string{"--password=hunter2", "--verbose"}
		logger.Info("Test message", "args", slog.AnyValue(args))

		output := buf.String()
		assert.NotContains(t, output, "hunter2", "Sensitive string in slice should be redacted")
		assert.Contains(t, output, "[REDACTED]", "Should contain redaction placeholder")
		assert.Contains(t, output, "--verbose", "Non-sensitive sibling element should be preserved, confirming redaction is targeted not blanket")

		// Positive control: verify secret appears via non-redacting JSON handler
		var controlBuf bytes.Buffer
		controlHandler := slog.NewJSONHandler(&controlBuf, nil)
		controlLogger := slog.New(controlHandler)
		controlLogger.Info("Test message", "args", slog.AnyValue(args))
		controlOutput := controlBuf.String()
		assert.Contains(t, controlOutput, "hunter2", "Secret should appear in non-redacting handler output, confirming redaction is actually preventing leakage")
	})

	t.Run("SliceOfMaps", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		items := []map[string]string{
			{"path": "/usr/bin/ls", "token": "secret-token-abc"},
			{"path": "/usr/bin/cat"},
		}
		logger.Info("Test message", "items", slog.AnyValue(items))

		output := buf.String()
		assert.NotContains(t, output, "secret-token-abc", "Sensitive value in slice of maps should be redacted")
		assert.Contains(t, output, "[REDACTED]", "Should contain redaction placeholder")
		assert.Contains(t, output, "/usr/bin/ls", "Non-sensitive map values should be preserved")
		assert.Contains(t, output, "/usr/bin/cat", "Non-sensitive map values should be preserved")

		// Positive control: verify secret appears via non-redacting JSON handler
		var controlBuf bytes.Buffer
		controlHandler := slog.NewJSONHandler(&controlBuf, nil)
		controlLogger := slog.New(controlHandler)
		controlLogger.Info("Test message", "items", slog.AnyValue(items))
		controlOutput := controlBuf.String()
		assert.Contains(t, controlOutput, "secret-token-abc", "Secret should appear in non-redacting handler output, confirming redaction is actually preventing leakage")
	})

	t.Run("NoSensitiveContent", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		args := []string{"normal", "args", "verbose"}
		logger.Info("Test message", "args", slog.AnyValue(args))

		output := buf.String()
		assert.Contains(t, output, "normal", "Non-sensitive string should be preserved")
		assert.Contains(t, output, "args", "Non-sensitive string should be preserved")
		assert.Contains(t, output, "verbose", "Non-sensitive string should be preserved")
	})

	t.Run("MixedTypes", func(t *testing.T) {
		var buf bytes.Buffer
		handler := slog.NewJSONHandler(&buf, nil)
		redactingHandler := NewRedactingHandler(handler, config, nil)
		logger := slog.New(redactingHandler)

		mixedSlice := []any{"string", 123, true, []string{"nested"}}
		logger.Info("Test message", "items", slog.AnyValue(mixedSlice))

		output := buf.String()
		assert.Contains(t, output, "string", "String element should be preserved")
		assert.Contains(t, output, "nested", "Nested array element should be preserved")

		// Verify []any type conversion via mock handler
		mock := newMockHandler()
		redactingMock := NewRedactingHandler(mock, config, nil)
		mockLogger := slog.New(redactingMock)
		mockLogger.Info("Test message", "items", slog.AnyValue(mixedSlice))

		require.Len(t, mock.records, 1)
		record := mock.records[0]
		var itemsAttr slog.Attr
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "items" {
				itemsAttr = attr
				return false
			}
			return true
		})
		anySlice, ok := itemsAttr.Value.Any().([]any)
		assert.True(t, ok, "Mixed slice should be converted to []any, got %T", itemsAttr.Value.Any())
		// All elements should have survived redaction
		assert.Len(t, anySlice, len(mixedSlice))
	})
}

// TestContainsRedactingHandler tests the containsRedactingHandler helper function
func TestContainsRedactingHandler(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() slog.Handler
		expected bool
	}{
		{
			name: "nil handler",
			setup: func() slog.Handler {
				return nil
			},
			expected: false,
		},
		{
			name: "simple text handler without RedactingHandler",
			setup: func() slog.Handler {
				return slog.NewTextHandler(os.Stderr, nil)
			},
			expected: false,
		},
		{
			name: "simple JSON handler without RedactingHandler",
			setup: func() slog.Handler {
				return slog.NewJSONHandler(os.Stderr, nil)
			},
			expected: false,
		},
		{
			name: "direct RedactingHandler",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				return NewRedactingHandler(baseHandler, nil, nil)
			},
			expected: true,
		},
		{
			name: "RedactingHandler wrapped in another RedactingHandler",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redacting1 := NewRedactingHandler(baseHandler, nil, nil)
				return NewRedactingHandler(redacting1, nil, nil)
			},
			expected: true,
		},
		{
			name: "RedactingHandler accessed via Handler() method",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redacting := NewRedactingHandler(baseHandler, nil, nil)
				// The Handler() method should expose the underlying handler
				return redacting
			},
			expected: true,
		},
		{
			name: "MultiHandler without RedactingHandler",
			setup: func() slog.Handler {
				textHandler := slog.NewTextHandler(os.Stderr, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(textHandler, jsonHandler)
				require.NoError(t, err)
				return multiHandler
			},
			expected: false,
		},
		{
			name: "MultiHandler with RedactingHandler in first position",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(redactingHandler, jsonHandler)
				require.NoError(t, err)
				return multiHandler
			},
			expected: true,
		},
		{
			name: "MultiHandler with RedactingHandler in middle position",
			setup: func() slog.Handler {
				textHandler := slog.NewTextHandler(os.Stderr, nil)
				baseHandler := slog.NewJSONHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				anotherTextHandler := slog.NewTextHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(textHandler, redactingHandler, anotherTextHandler)
				require.NoError(t, err)
				return multiHandler
			},
			expected: true,
		},
		{
			name: "MultiHandler with RedactingHandler in last position",
			setup: func() slog.Handler {
				textHandler := slog.NewTextHandler(os.Stderr, nil)
				baseHandler := slog.NewJSONHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				multiHandler, err := logging.NewMultiHandler(textHandler, redactingHandler)
				require.NoError(t, err)
				return multiHandler
			},
			expected: true,
		},
		{
			name: "MultiHandler with nested RedactingHandler",
			setup: func() slog.Handler {
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redacting1 := NewRedactingHandler(baseHandler, nil, nil)
				redacting2 := NewRedactingHandler(redacting1, nil, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(jsonHandler, redacting2)
				require.NoError(t, err)
				return multiHandler
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.setup()
			result := containsRedactingHandler(handler)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestNewRedactingHandler_FailureLoggerValidation tests that NewRedactingHandler
// panics when failureLogger contains a RedactingHandler in its chain
func TestNewRedactingHandler_FailureLoggerValidation(t *testing.T) {
	tests := []struct {
		name        string
		setupLogger func() *slog.Logger
		expectPanic bool
	}{
		{
			name: "failureLogger without RedactingHandler - no panic",
			setupLogger: func() *slog.Logger {
				// Create a simple logger without RedactingHandler
				handler := slog.NewTextHandler(os.Stderr, nil)
				return slog.New(handler)
			},
			expectPanic: false,
		},
		{
			name: "failureLogger with RedactingHandler - panic expected",
			setupLogger: func() *slog.Logger {
				// Create a logger with RedactingHandler in the chain
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				return slog.New(redactingHandler)
			},
			expectPanic: true,
		},
		{
			name: "failureLogger with nested RedactingHandler - panic expected",
			setupLogger: func() *slog.Logger {
				// Create a logger with nested RedactingHandler
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redacting1 := NewRedactingHandler(baseHandler, nil, nil)
				redacting2 := NewRedactingHandler(redacting1, nil, nil)
				return slog.New(redacting2)
			},
			expectPanic: true,
		},
		{
			name: "nil failureLogger (uses default) - no panic in this specific case",
			setupLogger: func() *slog.Logger {
				return nil
			},
			expectPanic: false,
		},
		{
			name: "failureLogger with MultiHandler containing RedactingHandler - panic expected",
			setupLogger: func() *slog.Logger {
				// Create a MultiHandler that contains a RedactingHandler
				baseHandler := slog.NewTextHandler(os.Stderr, nil)
				redactingHandler := NewRedactingHandler(baseHandler, nil, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(jsonHandler, redactingHandler)
				require.NoError(t, err)
				return slog.New(multiHandler)
			},
			expectPanic: true,
		},
		{
			name: "failureLogger with MultiHandler without RedactingHandler - no panic",
			setupLogger: func() *slog.Logger {
				// Create a MultiHandler without RedactingHandler
				textHandler := slog.NewTextHandler(os.Stderr, nil)
				jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
				multiHandler, err := logging.NewMultiHandler(textHandler, jsonHandler)
				require.NoError(t, err)
				return slog.New(multiHandler)
			},
			expectPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseHandler := slog.NewTextHandler(os.Stderr, nil)
			failureLogger := tt.setupLogger()

			if tt.expectPanic {
				// Expect a panic
				assert.Panics(t, func() {
					NewRedactingHandler(baseHandler, nil, failureLogger)
				}, "Expected NewRedactingHandler to panic with RedactingHandler in failureLogger chain")
			} else {
				// Should not panic
				assert.NotPanics(t, func() {
					NewRedactingHandler(baseHandler, nil, failureLogger)
				}, "NewRedactingHandler should not panic with valid failureLogger")
			}
		})
	}
}

// TestProductionLoggerSetup verifies that the production logger setup
// (as used in internal/runner/bootstrap/logger.go) does not violate
// the constraint that failureLogger must not contain RedactingHandler
func TestProductionLoggerSetup(t *testing.T) {
	// Simulate the production setup from internal/runner/bootstrap/logger.go

	// 1. Create base handlers (text and JSON)
	textHandler := slog.NewTextHandler(os.Stderr, nil)
	jsonHandler := slog.NewJSONHandler(os.Stderr, nil)

	// 2. Create failureLogger from base handlers (NO RedactingHandler)
	failureHandlers := []slog.Handler{textHandler, jsonHandler}
	failureMultiHandler, err := logging.NewMultiHandler(failureHandlers...)
	require.NoError(t, err)
	failureLogger := slog.New(failureMultiHandler)

	// 3. Verify failureLogger does not contain RedactingHandler
	assert.False(t, containsRedactingHandler(failureLogger.Handler()),
		"Production failureLogger should not contain RedactingHandler")

	// 4. Create main handler with RedactingHandler
	// Should not panic with valid failureLogger
	mainHandler, err := logging.NewMultiHandler(textHandler, jsonHandler)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		_ = NewRedactingHandler(mainHandler, nil, failureLogger)
	}, "Production setup should not panic - failureLogger is correctly configured without RedactingHandler")
}

// TestRedactText_ValueBasedDetection verifies that RedactText masks known value
// formats (AWS keys, GitHub tokens, etc.) through the ValueDetector integration.
// This tests the Layer 1 path (SanitizeOutputForLogging).
func TestRedactText_ValueBasedDetection(t *testing.T) {
	config := DefaultConfig()

	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "AWS access key ID is masked",
			input: "export KEY=AKIAIOSFODNN7EXAMPLE",
		},
		{
			name:  "GitHub token ghp_ is masked",
			input: "GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789ab",
		},
		{
			name:  "Slack bot token is masked",
			input: "SLACK_TOKEN=" + "xoxb-" + "999999999999-888888888888-zzzzzzzzzzzzzzzzzzzz",
		},
		{
			name:  "PEM private key block is masked",
			input: "key data:\n-----BEGIN RSA PRIVATE KEY-----\nMIIEpAIBAAKCAQEA...\n-----END RSA PRIVATE KEY-----",
		},
		{
			name:  "Bearer token is masked",
			input: "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abc",
		},
		{
			name:  "URL credentials are masked",
			input: "Fetching from https://admin:hunter2@api.example.com/data",
		},
		{
			name:  "GCP service account key is masked",
			input: `{"private_key_id": "abcd1234ef5678abcd1234ef5678abcd1234ef56"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.RedactText(tt.input)
			assert.NotEqual(t, tt.input, result,
				"RedactText should modify input by masking sensitive values")
			assert.Contains(t, result, "[REDACTED]",
				"Masked output must contain the redaction placeholder")
		})
	}
}

// TestRedactText_ValueBasedDetection_BypassWhenNil verifies that when ValueDetector
// is nil, RedactText still works with key=value patterns but skips value-based detection.
// This ensures backward compatibility for callers that construct Config directly.
func TestRedactText_ValueBasedDetection_BypassWhenNil(t *testing.T) {
	config := Config{
		Placeholder:      "[HIDDEN]",
		Patterns:         DefaultSensitivePatterns(),
		KeyValuePatterns: []string{"password"},
		ValueDetector:    nil, // explicitly nil
	}

	// Key=value pattern should still work
	result := config.RedactText("password=secret value AKIAIOSFODNN7EXAMPLE")
	assert.Equal(t, "password=[HIDDEN] value AKIAIOSFODNN7EXAMPLE", result,
		"key=value redaction should work, but value-based detection should be skipped")
}

// TestRedactText_ValueBasedDetection_DefaultConfigMasksByDefault verifies AC-11:
// value-based masking is active by default (DefaultConfig wires a non-nil
// ValueDetector) without requiring any explicit opt-in. Plaintext values are only
// produced when a caller explicitly bypasses redaction (e.g. the CLI's
// --show-sensitive flag, which operates upstream of this package and is covered by
// its own tests; see internal/runner/group_executor_test.go and
// internal/runner/resource/types_test.go for that flag's own default-off behavior).
func TestRedactText_ValueBasedDetection_DefaultConfigMasksByDefault(t *testing.T) {
	config := DefaultConfig()

	result := config.RedactText("session used AKIAIOSFODNN7EXAMPLE without a recognizable key name")

	assert.NotContains(t, result, "AKIAIOSFODNN7EXAMPLE",
		"DefaultConfig must mask known secret value formats by default (AC-11)")
	assert.Contains(t, result, config.Placeholder)
}

// TestRedactingHandler_ValueBasedDetection_Layer2 verifies that the RedactingHandler
// (Layer 2 / Slack path) also masks value-format secrets through the ValueDetector
// integrated into RedactText. This confirms the Slack notification path is covered.
func TestRedactingHandler_ValueBasedDetection_Layer2(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	// Log a string attribute containing an AWS key -- should be masked by ValueDetector
	logger.Info(
		"command output",
		"stdout", "Found credentials: AKIAIOSFODNN7EXAMPLE in log",
	)

	// Parse JSON output
	output := buf.String()
	assert.NotEmpty(t, output)

	var entry map[string]any
	err := json.Unmarshal([]byte(output), &entry)
	require.NoError(t, err, "RedactingHandler should produce valid JSON")

	// The stdout value should be redacted
	stdout, ok := entry["stdout"].(string)
	require.True(t, ok, "stdout attribute should be a string")
	assert.Contains(t, stdout, "[REDACTED]",
		"stdout value containing AWS key should be masked by ValueDetector")
	assert.NotContains(t, stdout, "AKIA",
		"stdout value must not contain unmasked AWS key prefix")
}

// TestRedactingHandler_ValueBasedDetection_NestedGroup verifies that value-based
// detection works on nested group attributes (e.g., cmd_N subgroups via Slack).
func TestRedactingHandler_ValueBasedDetection_NestedGroup(t *testing.T) {
	var buf bytes.Buffer
	baseHandler := slog.NewJSONHandler(&buf, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	// Simulate a nested group structure (like Slack's cmd_N grouping)
	logger.Info(
		"command results",
		"cmd_1", slog.GroupValue(
			slog.String("command", "echo"),
			slog.String("stdout", "token=ghp_abcdefghijklmnopqrstuvwxyz0123456789ab"),
			slog.String("stderr", ""),
		),
	)

	output := buf.String()
	assert.NotEmpty(t, output)

	var entry map[string]any
	err := json.Unmarshal([]byte(output), &entry)
	require.NoError(t, err)

	cmd1, ok := entry["cmd_1"].(map[string]any)
	require.True(t, ok, "cmd_1 should be a nested object")
	stdout, ok := cmd1["stdout"].(string)
	require.True(t, ok)
	assert.NotContains(t, stdout, "ghp_",
		"Nested group stdout must not contain unmasked GitHub token prefix")
}

func BenchmarkRedactingHandler_String(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	timestamp := time.Now().String()

	b.ResetTimer()
	for range b.N {
		logger.Info(
			"test message",
			"user", "testuser",
			"action", "login",
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactingHandler_String_WithSensitiveData benchmarks with sensitive data redaction
func BenchmarkRedactingHandler_String_WithSensitiveData(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	timestamp := time.Now().String()

	b.ResetTimer()
	for range b.N {
		logger.Info(
			"test message",
			"user", "testuser",
			"credentials", "password=secret123 token=abc456",
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactingHandler_LogValuer benchmarks RedactingHandler with LogValuer attributes
func BenchmarkRedactingHandler_LogValuer(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	// Create LogValuer with sensitive data
	valuer := sensitiveLogValuer{data: "password=secret123"}
	timestamp := time.Now().String()

	b.ResetTimer()
	for range b.N {
		logger.Info(
			"test message",
			"user", "testuser",
			"data", valuer,
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactingHandler_Slice benchmarks RedactingHandler with slice attributes
func BenchmarkRedactingHandler_Slice(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	// Create slice of LogValuers with sensitive data
	slice := []slog.LogValuer{
		sensitiveLogValuer{data: "password=secret1"},
		sensitiveLogValuer{data: "token=secret2"},
		sensitiveLogValuer{data: "api_key=secret3"},
	}
	timestamp := time.Now().String()

	b.ResetTimer()
	for range b.N {
		logger.Info(
			"test message",
			"user", "testuser",
			"items", slice,
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactingHandler_Mixed benchmarks RedactingHandler with mixed attribute types
func BenchmarkRedactingHandler_Mixed(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	valuer := sensitiveLogValuer{data: "password=secret123"}
	slice := []slog.LogValuer{
		sensitiveLogValuer{data: "token=abc"},
		sensitiveLogValuer{data: "api_key=xyz"},
	}
	timestamp := time.Now().String()

	b.ResetTimer()
	for range b.N {
		logger.Info(
			"test message",
			"user", "testuser",
			"simple_string", "normal data",
			"sensitive_string", "password=mypass",
			"logvaluer", valuer,
			"slice", slice,
			"timestamp", timestamp,
		)
	}
}

// BenchmarkRedactText benchmarks the RedactText function
func BenchmarkRedactText(b *testing.B) {
	config := DefaultConfig()
	text := "User logged in with password=secret123 and token=abc456xyz"

	b.ResetTimer()
	for range b.N {
		_ = config.RedactText(text)
	}
}

// BenchmarkRedactText_NoSensitiveData benchmarks RedactText with non-sensitive data
func BenchmarkRedactText_NoSensitiveData(b *testing.B) {
	config := DefaultConfig()
	text := "User logged in successfully at 2024-01-01 12:00:00"

	b.ResetTimer()
	for range b.N {
		_ = config.RedactText(text)
	}
}

// BenchmarkRedactLogAttribute_String benchmarks RedactLogAttribute with string values
func BenchmarkRedactLogAttribute_String(b *testing.B) {
	config := DefaultConfig()
	attr := slog.String("message", "password=secret123")

	b.ResetTimer()
	for range b.N {
		_ = config.RedactLogAttribute(attr)
	}
}

// BenchmarkRedactLogAttribute_Group benchmarks RedactLogAttribute with group values
func BenchmarkRedactLogAttribute_Group(b *testing.B) {
	config := DefaultConfig()
	attr := slog.Group(
		"user",
		slog.String("name", "testuser"),
		slog.String("password", "secret123"),
		slog.String("token", "abc456"),
	)

	b.ResetTimer()
	for range b.N {
		_ = config.RedactLogAttribute(attr)
	}
}

// BenchmarkRedactLogAttribute_Any_LogValuer benchmarks RedactLogAttribute with LogValuer
func BenchmarkRedactLogAttribute_Any_LogValuer(b *testing.B) {
	config := DefaultConfig()
	valuer := sensitiveLogValuer{data: "password=secret123"}
	attr := slog.Any("data", valuer)

	b.ResetTimer()
	for range b.N {
		_ = config.RedactLogAttribute(attr)
	}
}

// BenchmarkRedactingHandler_WithLargeMap benchmarks RedactingHandler with a large 1,000 entry map
func BenchmarkRedactingHandler_WithLargeMap(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	largeMap := make(map[string]string, 1000)
	for i := range 1000 {
		largeMap[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d", i)
	}
	// Add a few sensitive entries to exercise the redaction path
	largeMap["api_key"] = "secret-12345"
	largeMap["token"] = "token-abcdef"

	b.ResetTimer()
	for range b.N {
		logger.Info("test message", "large_map", slog.AnyValue(largeMap))
	}
}

// BenchmarkRedactingHandler_WithWideStruct benchmarks RedactingHandler with a 50-field struct
func BenchmarkRedactingHandler_WithWideStruct(b *testing.B) {
	baseHandler := slog.NewJSONHandler(io.Discard, nil)

	failureLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewRedactingHandler(baseHandler, nil, failureLogger)
	logger := slog.New(handler)

	type WideStruct struct {
		Field01 string `json:"field_01"`
		Field02 string `json:"field_02"`
		Field03 string `json:"field_03"`
		Field04 string `json:"field_04"`
		Field05 string `json:"field_05"`
		Field06 string `json:"field_06"`
		Field07 string `json:"field_07"`
		Field08 string `json:"field_08"`
		Field09 string `json:"field_09"`
		Field10 string `json:"field_10"`
		Field11 string `json:"field_11"`
		Field12 string `json:"field_12"`
		Field13 string `json:"field_13"`
		Field14 string `json:"field_14"`
		Field15 string `json:"field_15"`
		Field16 string `json:"field_16"`
		Field17 string `json:"field_17"`
		Field18 string `json:"field_18"`
		Field19 string `json:"field_19"`
		Field20 string `json:"field_20"`
		Field21 string `json:"field_21"`
		Field22 string `json:"field_22"`
		Field23 string `json:"field_23"`
		Field24 string `json:"field_24"`
		Field25 string `json:"field_25"`
		Field26 string `json:"field_26"`
		Field27 string `json:"field_27"`
		Field28 string `json:"field_28"`
		Field29 string `json:"field_29"`
		Field30 string `json:"field_30"`
		Field31 string `json:"field_31"`
		Field32 string `json:"field_32"`
		Field33 string `json:"field_33"`
		Field34 string `json:"field_34"`
		Field35 string `json:"field_35"`
		Field36 string `json:"field_36"`
		Field37 string `json:"field_37"`
		Field38 string `json:"field_38"`
		Field39 string `json:"field_39"`
		Field40 string `json:"field_40"`
		Field41 string `json:"field_41"`
		Field42 string `json:"field_42"`
		Field43 string `json:"field_43"`
		Field44 string `json:"field_44"`
		Field45 string `json:"field_45"`
		Field46 string `json:"field_46"`
		Field47 string `json:"field_47"`
		Field48 string `json:"field_48"`
		Field49 string `json:"field_49"`
		Field50 string `json:"field_50"`
	}

	ws := WideStruct{
		Field01: "value_01", Field02: "value_02", Field03: "value_03",
		Field04: "value_04", Field05: "value_05", Field06: "value_06",
		Field07: "value_07", Field08: "value_08", Field09: "value_09",
		Field10: "value_10", Field11: "value_11", Field12: "value_12",
		Field13: "value_13", Field14: "value_14", Field15: "value_15",
		Field16: "value_16", Field17: "value_17", Field18: "value_18",
		Field19: "value_19", Field20: "value_20", Field21: "value_21",
		Field22: "value_22", Field23: "value_23", Field24: "value_24",
		Field25: "value_25", Field26: "password=secret", Field27: "value_27",
		Field28: "value_28", Field29: "value_29", Field30: "value_30",
		Field31: "value_31", Field32: "value_32", Field33: "value_33",
		Field34: "value_34", Field35: "value_35", Field36: "value_36",
		Field37: "value_37", Field38: "value_38", Field39: "value_39",
		Field40: "value_40", Field41: "value_41", Field42: "value_42",
		Field43: "value_43", Field44: "value_44", Field45: "value_45",
		Field46: "value_46", Field47: "value_47", Field48: "value_48",
		Field49: "value_49", Field50: "value_50",
	}

	b.ResetTimer()
	for range b.N {
		logger.Info("test message", "wide_struct", slog.AnyValue(ws))
	}
}
