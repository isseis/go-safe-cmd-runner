//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
)

// assertInterpolationProperties checks the output properties every role except
// bulk output must satisfy. role decides whether the length limit applies.
func assertInterpolationProperties(t *testing.T, role InterpolationRole, out string) {
	t.Helper()

	assert.True(t, utf8.ValidString(out), "output must be valid UTF-8: %q", out)
	assert.NotContains(t, out, "\n", "output must not contain a line feed")
	assert.NotContains(t, out, "\r", "output must not contain a carriage return")
	assert.NotContains(t, out, "\u2028", "output must not contain a line separator")
	assert.NotContains(t, out, "\u2029", "output must not contain a paragraph separator")
	assert.NotContains(t, out, "<", "output must not contain a bare <")
	assert.NotContains(t, out, ">", "output must not contain a bare >")

	for i, r := range out {
		assert.False(t, unicode.IsControl(r), "output must not contain Cc character %U at byte %d", r, i)
		assert.False(t, unicode.Is(unicode.Cf, r), "output must not contain Cf character %U at byte %d", r, i)
	}

	for i := range len(out) {
		if out[i] != '&' {
			continue
		}
		rest := out[i:]
		isEntity := false
		for _, entity := range interpolatedEntities {
			if strings.HasPrefix(rest, entity) {
				isEntity = true
				break
			}
		}
		assert.True(t, isEntity, "output must not contain a bare & at byte %d: %q", i, out)
	}

	if role == InterpolationRoleFreeText || role == InterpolationRoleEnvelopeValue {
		assert.LessOrEqual(t, len(out), interpolationMaxBytes, "output must respect the byte limit")
	}
}

func TestInterpolate_OutputProperties(t *testing.T) {
	tests := []struct {
		name  string
		input string
		role  InterpolationRole
		want  string
	}{
		{
			name:  "C0 control characters become spaces",
			input: "a\x00b\x01c\x1fd",
			role:  InterpolationRoleFreeText,
			want:  "a b c d",
		},
		{
			name:  "DEL and C1 control characters become spaces",
			input: "a\u007fb\u0085c\u009fd",
			role:  InterpolationRoleFreeText,
			want:  "a b c d",
		},
		{
			name:  "line and paragraph separators become spaces",
			input: "a\u2028b\u2029c",
			role:  InterpolationRoleFreeText,
			want:  "a b c",
		},
		{
			name:  "bidi and format controls become spaces",
			input: "a\u202Ab\u202Ec\u2066d\u2069e",
			role:  InterpolationRoleFreeText,
			want:  "a b c d e",
		},
		{
			name:  "line breaks are normalized to one line",
			input: "line1\nline2\r\n",
			role:  InterpolationRoleFreeText,
			want:  "line1 line2  ",
		},
		{
			name:  "entity characters are escaped",
			input: "&<>",
			role:  InterpolationRoleFreeText,
			want:  "&amp;&lt;&gt;",
		},
		{
			name:  "existing entities are escaped once more",
			input: "<b>&amp;</b>",
			role:  InterpolationRoleFreeText,
			want:  "&lt;b&gt;&amp;amp;&lt;/b&gt;",
		},
		{
			name:  "slack mentions are escaped",
			input: "<!channel> <@U012345>",
			role:  InterpolationRoleFreeText,
			want:  "&lt;!channel&gt; &lt;@U012345&gt;",
		},
		{
			name:  "spoofed links are escaped but keep their displayed text",
			input: "<https://evil.example|click>",
			role:  InterpolationRoleFreeText,
			want:  "&lt;https://evil.example|click&gt;",
		},
		{
			name:  "bare urls keep their spelling",
			input: "https://evil.example/path",
			role:  InterpolationRoleFreeText,
			want:  "https://evil.example/path",
		},
		{
			name:  "invalid utf8 run is replaced with the replacement character",
			input: "a\xff\xfeb",
			role:  InterpolationRoleFreeText,
			want:  "a\uFFFDb",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Interpolate(tt.input, tt.role)
			assert.Equal(t, tt.want, got)
			assertInterpolationProperties(t, tt.role, got)
		})
	}
}

func TestInterpolate_FreeTextTruncation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "ascii just below the limit is untouched",
			input: strings.Repeat("a", interpolationMaxBytes-1),
			want:  strings.Repeat("a", interpolationMaxBytes-1),
		},
		{
			name:  "ascii at the limit is untouched",
			input: strings.Repeat("a", interpolationMaxBytes),
			want:  strings.Repeat("a", interpolationMaxBytes),
		},
		{
			name:  "ascii over the limit is cut to the limit",
			input: strings.Repeat("a", interpolationMaxBytes+50),
			want:  strings.Repeat("a", interpolationMaxBytes),
		},
		{
			name:  "multibyte rune cut at the boundary is dropped whole",
			input: strings.Repeat("あ", interpolationMaxBytes/3+1),
			want:  strings.Repeat("あ", interpolationMaxBytes/3),
		},
		{
			name:  "complete entity ending at the limit is kept",
			input: strings.Repeat("a", 495) + "<",
			want:  strings.Repeat("a", 495) + "&lt;",
		},
		{
			name:  "partial entity at the boundary is removed with its &",
			input: strings.Repeat("a", 498) + "<",
			want:  strings.Repeat("a", 498),
		},
		{
			name:  "single & at the boundary is removed",
			input: strings.Repeat("a", 499) + "&",
			want:  strings.Repeat("a", 499),
		},
		{
			name:  "invalid byte in the middle is replaced before the tail is cut",
			input: "a\xff" + strings.Repeat("b", 550),
			want:  "a\uFFFD" + strings.Repeat("b", 496),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Interpolate(tt.input, InterpolationRoleFreeText)
			assert.Equal(t, tt.want, got)
			assertInterpolationProperties(t, InterpolationRoleFreeText, got)
		})
	}
}

func TestInterpolate_RoleTruncation(t *testing.T) {
	long := strings.Repeat("x", interpolationMaxBytes+100)

	tests := []struct {
		name string
		role InterpolationRole
		want string
	}{
		{
			name: "free text is truncated",
			role: InterpolationRoleFreeText,
			want: strings.Repeat("x", interpolationMaxBytes),
		},
		{
			name: "envelope value is truncated",
			role: InterpolationRoleEnvelopeValue,
			want: strings.Repeat("x", interpolationMaxBytes),
		},
		{
			name: "identifier is not truncated",
			role: InterpolationRoleIdentifier,
			want: long,
		},
		{
			name: "bulk output is passed through",
			role: InterpolationRoleBulkOutput,
			want: long,
		},
		{
			name: "unknown role falls back to free text",
			role: InterpolationRole(99),
			want: strings.Repeat("x", interpolationMaxBytes),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Interpolate(long, tt.role))
		})
	}
}

func TestInterpolate_BulkOutputIsPassedThrough(t *testing.T) {
	input := "line1\nline2<&>"
	assert.Equal(t, input, Interpolate(input, InterpolationRoleBulkOutput))
}

func TestInterpolate_ZeroRoleIsFreeText(t *testing.T) {
	long := strings.Repeat("y", interpolationMaxBytes+1)
	assert.Equal(t, long[:interpolationMaxBytes], Interpolate(long, 0))
}

func TestHasDisplayableContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "empty string", input: "", want: false},
		{name: "spaces only", input: "   ", want: false},
		{name: "control characters only", input: "\x00\x01\x02", want: false},
		{name: "format controls only", input: "\u202A\u202E\u2066", want: false},
		{name: "line separators only", input: "\u2028\u2029", want: false},
		{name: "mixed whitespace and controls", input: " \t\n\x00 ", want: false},
		{name: "plain name", input: "backup", want: true},
		{name: "name surrounded by whitespace", input: "  backup  ", want: true},
		{name: "name with a control character", input: "back\nup", want: true},
		{name: "name with a format control character", input: "back\u202Eup", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, HasDisplayableContent(tt.input))
		})
	}
}
