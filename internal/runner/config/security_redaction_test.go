//go:build test

package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTextSegmentsAreWholeValueReplaced pins that a Text part declared for a
// raw config value or a rejected name still receives whole-value replacement,
// while the same string declared as an Identifier is exempt. The value trips
// only the whole-value layer, so the replacement proves that layer is in use.
func TestTextSegmentsAreWholeValueReplaced(t *testing.T) {
	cfg := redaction.DefaultConfig()
	const value = "API_TOKEN"

	// Layer isolation: the value-format and key=value layers do not react to
	// this value on their own.
	require.True(t, redaction.DefaultSensitivePatterns().IsSensitiveValue(value))
	require.Equal(t, value, cfg.RedactText(value),
		"the value-format layer must not react, or the whole-value layer is not isolated")

	message := errmsg.NewMessage(
		errmsg.Const("prefix "),
		errmsg.Text(value),
		errmsg.Const(" vs "),
		errmsg.Ident(value),
	)
	out, err := cfg.RedactMessage(message)
	require.NoError(t, err)
	assert.Equal(t,
		"prefix "+redaction.DefaultPlaceholder+" vs "+value,
		out,
		"the Text part must be whole-value replaced and the Identifier part kept")
}

// TestNonIdentifierSegmentsMaskValueFormats pins that a value that only the
// value-format layer detects (a GitHub token shape) is still masked in the
// non-Identifier parts a config error carries: a template input string (Text)
// and a resolved path (Path).
func TestNonIdentifierSegmentsMaskValueFormats(t *testing.T) {
	cfg := redaction.DefaultConfig()
	const token = "ghp_AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	require.False(t, redaction.DefaultSensitivePatterns().IsSensitiveValue(token),
		"the token must not trip the whole-value layer, or masking proves nothing")

	t.Run("template input string", func(t *testing.T) {
		err := &ErrTemplateInvalidEnvFormat{TemplateName: "tpl", Field: envVarsField(0), Entry: token}
		out, redactErr := cfg.RedactMessage(err.StructuredMessage())
		require.NoError(t, redactErr)
		assert.NotContains(t, out, token)
		assert.Contains(t, out, redaction.DefaultPlaceholder)
	})

	t.Run("resolved path", func(t *testing.T) {
		err := &ErrDuplicateResolvedPathDetail{
			Level:        groupLevel("g"),
			Field:        cmdAllowedFieldNoIndex(),
			OriginalPath: "orig",
			ResolvedPath: token,
		}
		out, redactErr := cfg.RedactMessage(err.StructuredMessage())
		require.NoError(t, redactErr)
		assert.NotContains(t, out, token)
		assert.Contains(t, out, redaction.DefaultPlaceholder)
	})
}
