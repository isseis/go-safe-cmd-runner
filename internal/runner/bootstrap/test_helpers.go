//go:build test

package bootstrap

import "github.com/isseis/go-safe-cmd-runner/internal/logging"

// SetSlackHandlerFactory replaces the factory AddSlackHandlers uses to build
// Slack handlers and returns a function that restores the previous one.
//
// It exists for tests outside this package. AddSlackHandlers assembles the
// SlackHandlerOptions itself and deliberately sets no HTTP client, so a test
// that wants the real handler to talk to an httptest server has no other place
// to inject that server's client.
//
// It writes package state without synchronisation, like the bootstrap path it
// belongs to, so it must be called before any logging that could run
// concurrently with it.
func SetSlackHandlerFactory(factory func(logging.SlackHandlerOptions) (*logging.SlackHandler, error)) func() {
	previous := newSlackHandlerFunc
	newSlackHandlerFunc = factory
	return func() { newSlackHandlerFunc = previous }
}
