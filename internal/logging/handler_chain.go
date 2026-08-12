package logging

import (
	"errors"
	"log/slog"
)

// ErrFailureLoggerContainsSlackHandler is returned by NewSlackHandler when a
// FailureHandlers element is, or wraps, a SlackHandler and would route send
// failures back into Slack.
var ErrFailureLoggerContainsSlackHandler = errors.New("failure handler contains a SlackHandler")

// ErrFailureLoggerUnverifiableHandler is returned by NewSlackHandler when a
// FailureHandlers element is of a type it cannot verify as Slack-free. The
// check fails closed: an unrecognised handler is rejected rather than assumed
// safe, because a wrapper that hides what it wraps is exactly the case a scan
// would silently pass.
var ErrFailureLoggerUnverifiableHandler = errors.New("failure handler cannot be verified as Slack-free")

// SlackFreeHandler is implemented by handlers that assert they never route
// records into Slack, directly or through anything they wrap. It is the opt-in
// that lets a handler this package cannot recognise -- a test double, say -- be
// used as a FailureHandlers element. Implementing it is an assertion by the
// handler's author, not something NewSlackHandler can verify.
type SlackFreeHandler interface {
	slog.Handler
	// SlackFree is a marker method; it does nothing.
	SlackFree()
}

// verifySlackFreeHandlers accepts only FailureHandlers elements whose type
// guarantees they cannot route records into Slack, and rejects everything
// else (fail closed). The accepted shapes are the leaf handlers of the stdlib
// and this package, a MultiHandler over accepted handlers, and handlers that
// opt in via SlackFreeHandler. An unrecognised handler is rejected rather than
// assumed safe: a wrapper that hides what it wraps is exactly the case a scan
// would silently pass. A MultiHandler is accepted only when every element it
// wraps is itself accepted, so the recursion is complete.
//
// This is deliberately not shared with internal/redaction.containsRedactingHandler.
// This package cannot import internal/redaction: redactor_test.go imports this
// package, so a logging → redaction import would create a cycle in redaction's
// test build. It also differs structurally: that function walks arbitrary
// handler chains looking for one type, while this check accepts a closed set of
// known types and rejects everything else.
func verifySlackFreeHandlers(handlers []slog.Handler) error {
	for _, handler := range handlers {
		switch h := handler.(type) {
		case *SlackHandler:
			// *SlackHandler must stay ahead of SlackFreeHandler below: if
			// SlackHandler ever gains the marker method, the specific
			// "contains a SlackHandler" rejection must still win over the
			// blanket SlackFreeHandler acceptance.
			return ErrFailureLoggerContainsSlackHandler
		case *slog.JSONHandler, *slog.TextHandler:
			// Stdlib leaf handlers: they wrap no other handler.
			continue
		case *ConditionalTextHandler, *InteractiveHandler:
			// This package's leaf handlers: each wraps only a *slog.TextHandler
			// this package constructs internally, never a caller-supplied
			// handler, so they cannot route records into Slack.
			continue
		case *MultiHandler:
			if err := verifySlackFreeHandlers(h.Handlers()); err != nil {
				return err
			}
		case SlackFreeHandler:
			// The author asserted Slack-freedom explicitly.
			continue
		default:
			if handler == slog.DiscardHandler {
				// slog.DiscardHandler is a value, not a pointer type, so it
				// cannot be matched in the type switch above.
				continue
			}
			return ErrFailureLoggerUnverifiableHandler
		}
	}
	return nil
}
