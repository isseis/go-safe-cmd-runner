//go:build test

package config

import (
	"strings"
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
// The errors are the real config error types, not a hand-built message.
func TestTextSegmentsAreWholeValueReplaced(t *testing.T) {
	cfg := redaction.DefaultConfig()
	const value = "API_TOKEN"

	// Layer isolation: the value-format and key=value layers do not react to
	// this value on their own.
	require.True(t, redaction.DefaultSensitivePatterns().IsSensitiveValue(value))
	require.Equal(t, value, cfg.RedactText(value),
		"the value-format layer must not react, or the whole-value layer is not isolated")

	// A rejected variable name is carried as a Text part; the same string
	// declared as an Identifier in the same message must still be exempt.
	err := &ErrInvalidVariableNameDetail{
		Level:        groupLevel("deploy"),
		Field:        envField(),
		VariableName: value,
		Reason:       "bad",
	}
	msg := errmsg.Merge(err.StructuredMessage(),
		errmsg.NewMessage(errmsg.Const(" vs "), errmsg.Ident(value)))
	out, redactErr := cfg.RedactMessage(msg)
	require.NoError(t, redactErr)
	assert.Contains(t, out, value, "the Identifier control must be kept")
	assert.Contains(t, out, redaction.DefaultPlaceholder,
		"the Text part must be whole-value replaced")
	assert.Equal(t, 1, strings.Count(out, value),
		"only the Identifier control may keep the raw value")

	// A raw config value is carried as a Text part too.
	rawErr := &ErrInvalidEnvFormatDetail{Level: globalLevel(), Mapping: value, Reason: "bad"}
	out, redactErr = cfg.RedactMessage(rawErr.StructuredMessage())
	require.NoError(t, redactErr)
	assert.NotContains(t, out, value, "the raw value Text part must be replaced")
	assert.Contains(t, out, redaction.DefaultPlaceholder)
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

	t.Run("env entry", func(t *testing.T) {
		err := &ErrInvalidEnvFormatDetail{Level: globalLevel(), Mapping: token, Reason: "bad"}
		out, redactErr := cfg.RedactMessage(err.StructuredMessage())
		require.NoError(t, redactErr)
		assert.NotContains(t, out, token)
		assert.Contains(t, out, redaction.DefaultPlaceholder)
	})

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
