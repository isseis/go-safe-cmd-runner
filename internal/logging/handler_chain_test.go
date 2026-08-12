package logging

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slackFreeTestDouble implements SlackFreeHandler so verifySlackFreeHandlers
// accepts it without having to recognise its concrete type.
type slackFreeTestDouble struct {
	*slog.TextHandler
}

func (slackFreeTestDouble) SlackFree() {}

// opaqueTestHandler hides its chain: it has neither Handler() nor Handlers(),
// so a scan-based design would pass it through unseen. verifySlackFreeHandlers
// must reject it (fail closed).
type opaqueTestHandler struct{}

func (opaqueTestHandler) Enabled(context.Context, slog.Level) bool  { return false }
func (opaqueTestHandler) Handle(context.Context, slog.Record) error { return nil }
func (opaqueTestHandler) WithAttrs([]slog.Attr) slog.Handler        { return opaqueTestHandler{} }
func (opaqueTestHandler) WithGroup(string) slog.Handler             { return opaqueTestHandler{} }

func TestVerifySlackFreeHandlers(t *testing.T) {
	jsonHandler := slog.NewJSONHandler(io.Discard, nil)
	jsonHandlerWithAttrs := jsonHandler.WithAttrs([]slog.Attr{slog.String("k", "v")})
	textHandler := slog.NewTextHandler(io.Discard, nil)

	condHandler, err := NewConditionalTextHandler(ConditionalTextHandlerOptions{
		Capabilities: &conditionalTestCapabilities{interactive: false},
		Writer:       io.Discard,
	})
	require.NoError(t, err)

	interactiveHandler, err := NewInteractiveHandler(InteractiveHandlerOptions{
		Writer:       io.Discard,
		Capabilities: &interactiveTestCapabilities{},
		Formatter:    &interactiveTestMessageFormatter{},
		LineTracker:  &interactiveTestLogLineTracker{},
	})
	require.NoError(t, err)

	wrapped, err := NewMultiHandler(jsonHandler, textHandler, condHandler, interactiveHandler)
	require.NoError(t, err)

	tests := []struct {
		name      string
		handlers  []slog.Handler
		wantError error
	}{
		{
			name:     "json handler",
			handlers: []slog.Handler{jsonHandler},
		},
		{
			name:     "json handler with attrs",
			handlers: []slog.Handler{jsonHandlerWithAttrs},
		},
		{
			name:     "text handler",
			handlers: []slog.Handler{textHandler},
		},
		{
			name:     "discard handler",
			handlers: []slog.Handler{slog.DiscardHandler},
		},
		{
			// Pins the assumption the default branch's equality check rests
			// on: a derived DiscardHandler still compares equal to the
			// original, so it is still accepted.
			name:     "discard handler with attrs",
			handlers: []slog.Handler{slog.DiscardHandler.WithAttrs([]slog.Attr{slog.String("k", "v")})},
		},
		{
			// Accepting the empty slice is what will let NewSlackHandler fall
			// back to a stderr-only failure logger in step 4-5.
			name:     "no handlers",
			handlers: nil,
		},
		{
			name:     "conditional text handler",
			handlers: []slog.Handler{condHandler},
		},
		{
			name:     "interactive handler",
			handlers: []slog.Handler{interactiveHandler},
		},
		{
			name:     "multi handler wrapping accepted handlers",
			handlers: []slog.Handler{wrapped},
		},
		{
			name:     "slack free test double",
			handlers: []slog.Handler{slackFreeTestDouble{}},
		},
		{
			name:      "slack handler directly",
			handlers:  []slog.Handler{&SlackHandler{}},
			wantError: ErrFailureLoggerContainsSlackHandler,
		},
		{
			name: "slack handler wrapped in multi handler",
			handlers: []slog.Handler{
				func() slog.Handler {
					m, err := NewMultiHandler(&SlackHandler{}, textHandler)
					require.NoError(t, err)
					return m
				}(),
			},
			wantError: ErrFailureLoggerContainsSlackHandler,
		},
		{
			// The rejected element is not first, so this fails for any
			// implementation that inspects only Handlers()[0].
			name: "slack handler not first in multi handler",
			handlers: []slog.Handler{
				func() slog.Handler {
					m, err := NewMultiHandler(textHandler, &SlackHandler{})
					require.NoError(t, err)
					return m
				}(),
			},
			wantError: ErrFailureLoggerContainsSlackHandler,
		},
		{
			name: "slack handler nested two multi handler levels deep",
			handlers: []slog.Handler{
				func() slog.Handler {
					inner, err := NewMultiHandler(textHandler, &SlackHandler{})
					require.NoError(t, err)
					outer, err := NewMultiHandler(inner, condHandler)
					require.NoError(t, err)
					return outer
				}(),
			},
			wantError: ErrFailureLoggerContainsSlackHandler,
		},
		{
			name: "opaque handler not first in multi handler",
			handlers: []slog.Handler{
				func() slog.Handler {
					m, err := NewMultiHandler(condHandler, opaqueTestHandler{})
					require.NoError(t, err)
					return m
				}(),
			},
			wantError: ErrFailureLoggerUnverifiableHandler,
		},
		{
			name:      "opaque handler",
			handlers:  []slog.Handler{opaqueTestHandler{}},
			wantError: ErrFailureLoggerUnverifiableHandler,
		},
		{
			name: "nil handler",
			handlers: []slog.Handler{
				nil,
			},
			wantError: ErrFailureLoggerUnverifiableHandler,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifySlackFreeHandlers(tt.handlers)
			if tt.wantError == nil {
				assert.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantError)
		})
	}
}
