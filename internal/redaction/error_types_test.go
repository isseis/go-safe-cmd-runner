package redaction_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// TestRedactMessage_DependencyBodiesKeepLibraryNames verifies that the
// dependency- and shebang-verification errors no longer lose a library name
// that contains a sensitive word. The same name as a free Text segment is
// whole-value replaced, so having that control in the same test proves the Path
// role is what keeps the name visible.
func TestRedactMessage_DependencyBodiesKeepLibraryNames(t *testing.T) {
	cfg := redaction.DefaultConfig()
	const lib = "libkeyutils.so.1"

	tests := []struct {
		name  string
		cause error
	}{
		{"library hash mismatch", &dynlib.ErrLibraryHashMismatch{
			SOName: lib, Path: "/usr/lib/" + lib, ExpectedHash: "sha256:aa", ActualHash: "sha256:bb",
		}},
		{"deps required", &dynlib.ErrDynLibDepsRequired{BinaryPath: "/usr/bin/" + lib}},
		{"empty library path", &dynlib.ErrEmptyLibraryPath{SOName: lib}},
		{"recursion depth", &dynlib.ErrRecursionDepthExceeded{SOName: lib, Depth: 3, MaxDepth: 2}},
		{"resolution changed", &verification.ErrDynLibDepsResolutionChanged{
			SOName: lib, RecordedPath: "/old/" + lib, ResolvedPath: "/new/" + lib,
		}},
		{"interpreter record not found", &verification.ErrInterpreterRecordNotFound{Path: "/usr/bin/" + lib}},
		{"interpreter symlink redirected", &verification.ErrInterpreterSymlinkRedirected{
			RawPath: "/bin/" + lib, RecordedPath: "/usr/bin/" + lib, ActualPath: "/usr/local/bin/" + lib,
		}},
		{"interpreter path mismatch", &verification.ErrInterpreterPathMismatch{
			CommandName: lib, RecordedPath: "/usr/bin/" + lib, ActualPath: "/usr/local/bin/" + lib,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The control: the same text as one Text segment is whole-value
			// replaced. Only the other layer can save the name.
			textOut, err := cfg.RedactMessage(errmsg.NewMessage(errmsg.Text(lib)))
			require.NoError(t, err)
			assert.Equal(t, redaction.DefaultPlaceholder, textOut)

			msg := errmsg.NewMessage(errmsg.Const("failed: "), errmsg.Cause(tt.cause))
			out, err := cfg.RedactMessage(msg)
			require.NoError(t, err)
			assert.Contains(t, out, lib, "the library name declared as a Path must survive")
		})
	}
}

// TestRedactMessage_PathSegmentStillMasksValueFormats pins that declaring a
// library name as a Path does not disable RedactText: both a keyed value and a
// value-format secret inside the Path segment are still masked.
func TestRedactMessage_PathSegmentStillMasksValueFormats(t *testing.T) {
	cfg := redaction.DefaultConfig()

	t.Run("keyed value", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Cause(&verification.ErrInterpreterRecordNotFound{Path: "/tmp/token=abc123/interp"}))
		out, err := cfg.RedactMessage(m)
		require.NoError(t, err)
		assert.Contains(t, out, "token=[REDACTED]")
	})

	t.Run("value format", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Cause(&verification.ErrInterpreterRecordNotFound{Path: "/tmp/AKIAIOSFODNN7EXAMPLE/interp"}))
		out, err := cfg.RedactMessage(m)
		require.NoError(t, err)
		assert.NotContains(t, out, "AKIAIOSFODNN7EXAMPLE")
		assert.Contains(t, out, redaction.DefaultPlaceholder)
	})
}
