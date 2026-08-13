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

// slowEndpointDelay is how long the mock endpoint takes to answer. It is long
// enough that the send cannot complete before the assertion by chance, and
// short enough to stay well inside the default flush deadline.
const slowEndpointDelay = 300 * time.Millisecond

// TestIntegration_RunnerFlushesSlackOnNormalExit covers the exit path this task
// adds: notifications are queued during the run, so a notification issued at
// the very end of a run only reaches Slack because main flushes before exiting.
//
// The test drives that sequence in-process -- SetupLoggerWithConfig,
// AddSlackHandlers, emit, FlushSlackNotifications -- rather than starting the
// binary with `go run .` the way integration_pre_execution_error_test.go does.
// The production send path verifies the server's certificate, and a separate
// process gives no place to hand it the mock server's client, so a real run
// against httptest would fail on `x509: certificate signed by unknown
// authority` before reaching what is under test. Replacing the Slack handler
// factory is the one seam where that client can be injected, because
// AddSlackHandlers builds the handler options itself.
func TestIntegration_RunnerFlushesSlackOnNormalExit(t *testing.T) {
	received := &receivedNotifications{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// A slow endpoint is what makes the assertion below about the flush
		// rather than about timing: the worker cannot have finished this send
		// on its own by the time the test reaches the assertion, so the count
		// is only non-zero because the flush waited for the drain.
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
	// The flush below is what stops the worker; this cleanup covers the paths
	// where an assertion fails before it runs.
	t.Cleanup(bootstrap.FlushSlackNotifications)

	// The last thing a failing run does before returning to main.
	slog.Error("run failed",
		"slack_notify", true,
		"message_type", "pre_execution_error")

	bootstrap.FlushSlackNotifications()

	assert.Equal(t, 1, received.count(),
		"the notification issued during the run should have been delivered by the exit flush")
}

// receivedNotifications counts the requests the mock endpoint served. The
// worker sends from its own goroutine, so the counter needs a lock.
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
