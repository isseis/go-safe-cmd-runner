package logging

import (
	"errors"
	"fmt"
	"log/slog"
)

// ErrFailureLoggerContainsSlackHandler is returned by the failure-handler
// verification below when a handler is, or wraps, a SlackHandler and would
// route send failures back into Slack. The verification is wired into
// NewSlackHandler in a later step; it has no production caller yet.
var ErrFailureLoggerContainsSlackHandler = errors.New("failure handler contains a SlackHandler")

// ErrFailureLoggerUnverifiableHandler is returned by the failure-handler
// verification below when a handler is of a type it cannot verify as
// Slack-free. The check fails closed: an unrecognised handler is rejected
// rather than assumed safe, because a wrapper that hides what it wraps is
// exactly the case a scan would silently pass.
var ErrFailureLoggerUnverifiableHandler = errors.New("failure handler cannot be verified as Slack-free")

// SlackFreeHandler is implemented by handlers that assert they never route
// records into Slack, directly or through anything they wrap. It is the opt-in
// that lets a handler this package cannot recognise -- a test double, say --
// serve as a failure handler. Implementing it is an assertion by the handler's
// author, not something the verification can check.
type SlackFreeHandler interface {
	slog.Handler
	// SlackFree is a marker method; it does nothing.
	SlackFree()
}

// verifySlackFreeHandlers accepts only failure handlers whose type
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
// package, so a logging -> redaction import would create a cycle in redaction's
// test build. It also differs structurally: that function walks arbitrary
// handler chains looking for one type, while this check accepts a closed set of
// known types and rejects everything else.
// Rejections name the offending element's position and concrete type, and
// nesting wraps one prefix per level, so a rejection from deep inside a chain
// can be bisected from the error text alone.
func verifySlackFreeHandlers(handlers []slog.Handler) error {
	for i, handler := range handlers {
		if err := verifySlackFreeHandler(handler); err != nil {
			return fmt.Errorf("handler[%d] (%T): %w", i, handler, err)
		}
	}
	return nil
}

// verifySlackFreeHandler applies the accept/reject decision to a single
// handler. See verifySlackFreeHandlers for the accepted set.
func verifySlackFreeHandler(handler slog.Handler) error {
	switch h := handler.(type) {
	case *SlackHandler:
		// *SlackHandler must stay ahead of SlackFreeHandler below: if
		// SlackHandler ever gains the marker method, the specific
		// "contains a SlackHandler" rejection must still win over the
		// blanket SlackFreeHandler acceptance.
		return ErrFailureLoggerContainsSlackHandler
	case *slog.JSONHandler, *slog.TextHandler:
		// Stdlib leaf handlers: they wrap no other handler.
		return nil
	case *ConditionalTextHandler:
		// Leaf handler of this package: its textHandler field is typed
		// *slog.TextHandler, so it cannot hold a caller-supplied handler
		// and cannot route records into Slack.
		return nil
	case *InteractiveHandler:
		// Leaf handler of this package: it wraps no slog.Handler at all
		// (it formats records onto an io.Writer itself), so it cannot
		// route records into Slack.
		return nil
	case *MultiHandler:
		if h == nil {
			// A typed-nil *MultiHandler would panic in Handlers(). It cannot
			// be verified, so it is rejected like any other unknown shape
			// rather than crashing the check.
			return ErrFailureLoggerUnverifiableHandler
		}
		return verifySlackFreeHandlers(h.Handlers())
	case SlackFreeHandler:
		// The author asserted Slack-freedom explicitly.
		return nil
	default:
		if handler == slog.DiscardHandler {
			// slog.DiscardHandler's concrete type is unexported, so it
			// cannot be named in a case above and has to be compared by
			// value here. Its WithAttrs/WithGroup are documented to return
			// the receiver, so derived handlers compare equal too.
			return nil
		}
		return ErrFailureLoggerUnverifiableHandler
	}
}
