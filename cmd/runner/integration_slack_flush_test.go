//go:build test

package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// slowEndpointDelay makes the assertion below about the flush rather than about
// timing: the worker cannot finish the send on its own before the test reaches
// it, and the delay still fits well inside the default flush deadline.
const slowEndpointDelay = 300 * time.Millisecond

// TestIntegration_RunnerFlushesSlackOnNormalExit covers the exit path this task
// adds: notifications are queued during the run, so a notification issued at
// the very end of a run only reaches Slack because main flushes before exiting.
//
// It drives that sequence in-process rather than starting the binary with
// `go run .` as integration_pre_execution_error_test.go does: the production
// send path verifies the server's certificate, and a separate process offers
// nowhere to hand it the mock server's client, so the run would fail on
// `x509: certificate signed by unknown authority` before reaching what is under
// test. The handler factory is that seam, since AddSlackHandlers builds the
// handler options itself.
func TestIntegration_RunnerFlushesSlackOnNormalExit(t *testing.T) {
	received := &receivedNotifications{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(slowEndpointDelay)
		received.add()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	restoreFactory := bootstrap.SetSlackHandlerFactory(func(opts logging.SlackHandlerOptions) (*logging.SlackHandler, error) {
		opts.WebhookURL = server.URL
		opts.AllowedHost = serverURL.Hostname()
		opts.HTTPClient = server.Client()
		return logging.NewSlackHandler(opts)
	})
	t.Cleanup(restoreFactory)

	originalLogger := slog.Default()
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	require.NoError(t, bootstrap.SetupLoggerWithConfig(bootstrap.LoggerConfig{
		Level:         slog.LevelInfo,
		LogDir:        t.TempDir(),
		RunID:         "test-flush-on-exit-001",
		ConsoleWriter: io.Discard,
	}, false, true))

	_, err = bootstrap.AddSlackHandlers(bootstrap.SlackLoggerConfig{
		WebhookURLError: "https://hooks.slack.com/services/error",
		AllowedHost:     "hooks.slack.com",
		RunID:           "test-flush-on-exit-001",
	})
	require.NoError(t, err)
	// For the paths where an assertion fails before the flush below runs.
	t.Cleanup(bootstrap.FlushSlackNotifications)

	// The last thing a failing run does before returning to main.
	slog.Error("run failed",
		"slack_notify", true,
		"message_type", "pre_execution_error")

	bootstrap.FlushSlackNotifications()

	assert.Equal(t, 1, received.count(),
		"the notification issued during the run should have been delivered by the exit flush")
}

// receivedNotifications is locked because the worker sends from its own
// goroutine.
type receivedNotifications struct {
	mu sync.Mutex
	n  int
}

func (r *receivedNotifications) add() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.n++
}

func (r *receivedNotifications) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.n
}
