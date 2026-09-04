package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Rule R1 (mock server construction): NewSlackHandler rejects any webhook URL
// that is not https, so mock endpoints are TLS servers. The handler is given
// server.Client(), which trusts the server's self-signed certificate, and
// AllowedHost gets the server's hostname without its port. InsecureSkipVerify
// is deliberately unused: server.Client() is the supported way and needs no
// new gosec suppression.
//
// Rule R2 (teardown): every server, handler and sender helper below registers
// its own shutdown with t.Cleanup at the point the resource is created, so a
// failing assertion cannot leak a worker into a later test.
//
// Rule R3 (observing worker termination): worker exit is observed through the
// sender's done channel or through drops being counted after close, never
// through an exact runtime.NumGoroutine() match, which is process-wide and
// moves under httptest connections and -p 4 parallelism.

// syncBuffer is an io.Writer a worker goroutine and the test goroutine can use
// at the same time.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// newMockSlackServer starts a TLS mock Slack endpoint (rule R1).
func newMockSlackServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	return server
}

// slackOptionsFor returns the options that make NewSlackHandler accept a mock
// server (rule R1). Callers add whatever else the case under test needs.
func slackOptionsFor(t *testing.T, server *httptest.Server) SlackHandlerOptions {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err, "mock server URL should parse")
	return SlackHandlerOptions{
		WebhookURL:    server.URL,
		RunID:         "test-run",
		AllowedHost:   parsed.Hostname(),
		HTTPClient:    server.Client(),
		BackoffConfig: testBackoffConfig,
	}
}

// newTestSlackHandler builds a handler and registers its shutdown immediately
// (rule R2).
func newTestSlackHandler(t *testing.T, opts SlackHandlerOptions) *SlackHandler {
	t.Helper()
	handler, err := NewSlackHandler(opts)
	require.NoError(t, err, "NewSlackHandler should accept the mock server options")
	t.Cleanup(func() { handler.Close() })
	return handler
}

// newTestSlackSender builds a sender with no queues and no worker, for tests
// that call send directly.
func newTestSlackSender(t *testing.T, server *httptest.Server) *slackSender {
	t.Helper()
	return &slackSender{
		webhookURL:    server.URL,
		httpClient:    server.Client(),
		backoffConfig: testBackoffConfig,
		failureLogger: slog.New(slog.NewJSONHandler(io.Discard, nil)),
		sendTimeout:   DefaultSendTimeout,
		sentByType:    make(map[string]int),
		failedByType:  make(map[string]int),
		droppedByType: make(map[string]int),
	}
}

// slackRecorder records what a mock Slack endpoint received.
type slackRecorder struct {
	mu       sync.Mutex
	messages []SlackMessage
}

func (r *slackRecorder) add(msg SlackMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
}

func (r *slackRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.messages)
}

func (r *slackRecorder) texts() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	texts := make([]string, len(r.messages))
	for i, msg := range r.messages {
		texts[i] = msg.Text
	}
	return texts
}

// newRecordingSlackServer starts a mock endpoint that records every message it
// receives and answers with status. When block is non-nil each request parks
// until block is closed or the request context is cancelled, which is how a
// test pins the worker inside a send. The message is recorded before parking,
// so "the send has started" is observable while it is still in flight.
//
// A parked request whose client goes away is aborted rather than answered.
// Returning from the handler would make net/http synthesize a 200 for a client
// that has already cancelled, and the client can still observe that response:
// the transport's read loop may deliver it before the cancellation tears the
// request down. A test that cancels a parked send to prove it was not
// delivered would then see it counted as sent. ErrAbortHandler closes the
// connection without a response, so there is no reply to race.
func newRecordingSlackServer(t *testing.T, status int, block chan struct{}) (*slackRecorder, *httptest.Server) {
	t.Helper()
	rec := &slackRecorder{}
	server := newMockSlackServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			panic(http.ErrAbortHandler)
		}
		var msg SlackMessage
		if err := json.Unmarshal(body, &msg); err == nil {
			rec.add(msg)
		}
		if block != nil {
			select {
			case <-block:
			case <-r.Context().Done():
				panic(http.ErrAbortHandler)
			}
		}
		w.WriteHeader(status)
	})
	return rec, server
}

// releaseFunc closes a block channel exactly once, so both an explicit release
// and the cleanup registered with it are safe.
func newBlock(t *testing.T) (chan struct{}, func()) {
	t.Helper()
	block := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(block) }) }
	t.Cleanup(release)
	return block, release
}

// failureLogHandlers returns the FailureHandlers value that routes send
// failures and drops into buf.
func failureLogHandlers(buf *syncBuffer) []slog.Handler {
	return []slog.Handler{slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})}
}

// waitForFailureLog blocks until the failure logger has written a record
// containing want. Tests that assert on Sent or Failed use it to reach a point
// where the worker is between sends: flushing mid-send cancels that send, which
// counts as Pending instead of the outcome under test.
func waitForFailureLog(t *testing.T, buf *syncBuffer, want string) {
	t.Helper()
	require.Eventually(t, func() bool {
		return strings.Contains(buf.String(), want)
	}, 5*time.Second, 5*time.Millisecond, "failure log should contain %q, got: %s", want, buf.String())
}

// waitForRequests blocks until the mock endpoint has received at least n
// requests.
func waitForRequests(t *testing.T, rec *slackRecorder, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return rec.count() >= n
	}, 5*time.Second, 5*time.Millisecond, "mock Slack endpoint should have received %d request(s)", n)
}

// gateWorkerAtDequeue parks the worker in the position between taking a
// request off a queue and registering the send, and reports when it gets
// there. Assigning afterDequeue before the first enqueue is race-free: the
// worker reads the field only after receiving from a queue, which
// happens-after the enqueue that follows this call.
func gateWorkerAtDequeue(sd *slackSender) (reached <-chan struct{}, release func()) {
	gate := make(chan struct{})
	arrived := make(chan struct{})
	var arriveOnce, releaseOnce sync.Once

	sd.afterDequeue = func() {
		arriveOnce.Do(func() { close(arrived) })
		<-gate
	}

	return arrived, func() { releaseOnce.Do(func() { close(gate) }) }
}

// waitForStopAccepting blocks until Flush or Close has taken effect. A test
// that parked the worker at the dequeue boundary uses it before releasing, so
// the worker is certain to observe the shutdown rather than racing it into a
// normal send.
func waitForStopAccepting(t *testing.T, sd *slackSender) {
	t.Helper()
	require.Eventually(t, func() bool {
		sd.mu.RLock()
		defer sd.mu.RUnlock()
		return sd.closed
	}, 5*time.Second, time.Millisecond, "Flush/Close should have stopped the sender accepting")
}

// slackRecord builds a record that SlackHandler will deliver.
func slackRecord(level slog.Level, messageType, text string) slog.Record {
	record := slog.NewRecord(time.Now(), level, text, 0)
	record.AddAttrs(slog.Bool("slack_notify", true))
	if messageType != "" {
		record.AddAttrs(slog.String("message_type", messageType))
	}
	return record
}

// securityAlertRecord builds a high-priority record whose rendered text
// carries eventType, so tests can tell alerts apart in delivery order.
func securityAlertRecord(eventType string) slog.Record {
	record := slog.NewRecord(time.Now(), slog.LevelError, "security alert", 0)
	record.AddAttrs(
		slog.Bool("slack_notify", true),
		slog.String("message_type", messageTypeSecurityAlert),
		slog.String(common.SecurityAlertAttrs.EventType, eventType),
		slog.String(common.SecurityAlertAttrs.Severity, common.SeverityCritical),
	)
	return record
}

func TestSlackHandler_HandleDoesNotBlockOnUnresponsiveServer(t *testing.T) {
	block, _ := newBlock(t)
	_, server := newRecordingSlackServer(t, http.StatusOK, block)

	handler := newTestSlackHandler(t, slackOptionsFor(t, server))

	const calls = 10
	// A synchronous implementation would wait on the blocked server until the
	// 5s HTTP timeout, so this still fails by an order of magnitude while
	// leaving room for scheduling noise on a loaded CI runner.
	const limit = time.Second

	start := time.Now()
	for range calls {
		require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "message")))
	}
	elapsed := time.Since(start)

	assert.Less(t, elapsed, limit, "Handle must not wait on the Slack round trip")
}

func TestSlackHandler_UnreachableSlackDoesNotDelayOtherHandlers(t *testing.T) {
	block, _ := newBlock(t)
	_, server := newRecordingSlackServer(t, http.StatusOK, block)

	slackHandler := newTestSlackHandler(t, slackOptionsFor(t, server))

	var other syncBuffer
	multi, err := NewMultiHandler(slackHandler, slog.NewJSONHandler(&other, &slog.HandlerOptions{Level: slog.LevelDebug}))
	require.NoError(t, err)

	// See TestSlackHandler_HandleDoesNotBlockOnUnresponsiveServer for the bound.
	const limit = time.Second
	start := time.Now()
	require.NoError(t, multi.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "sibling handler must not wait")))
	elapsed := time.Since(start)

	assert.Less(t, elapsed, limit, "the non-Slack handler must not wait on Slack")
	assert.Contains(t, other.String(), "sibling handler must not wait", "the non-Slack handler should have written the record")
}

func TestSlackSender_SendTimeout(t *testing.T) {
	block, _ := newBlock(t)
	_, server := newRecordingSlackServer(t, http.StatusOK, block)

	var failureLog syncBuffer
	opts := slackOptionsFor(t, server)
	opts.SendTimeout = 100 * time.Millisecond
	opts.FailureHandlers = failureLogHandlers(&failureLog)
	handler := newTestSlackHandler(t, opts)

	require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "message")))

	waitForFailureLog(t, &failureLog, "Slack notification not delivered")

	stats := handler.Flush(context.Background())
	assert.Equal(t, int64(1), stats.Failed, "an expired send deadline is a delivery failure")
	assert.Equal(t, int64(0), stats.Sent)
}

func TestSlackHandler_MessageIdenticalToSynchronousMode(t *testing.T) {
	asyncRec, asyncServer := newRecordingSlackServer(t, http.StatusOK, nil)
	syncRec, syncServer := newRecordingSlackServer(t, http.StatusOK, nil)

	asyncHandler := newTestSlackHandler(t, slackOptionsFor(t, asyncServer))

	syncOpts := slackOptionsFor(t, syncServer)
	syncOpts.Synchronous = true
	syncHandler := newTestSlackHandler(t, syncOpts)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "Command group execution completed", 0)
	record.AddAttrs(
		slog.Bool("slack_notify", true),
		slog.String("message_type", messageTypeCommandGroupSummary),
		slog.String(common.GroupSummaryAttrs.Status, "success"),
		slog.String(common.GroupSummaryAttrs.Group, "test-group"),
		slog.Int64(common.GroupSummaryAttrs.DurationMs, 100),
	)

	require.NoError(t, asyncHandler.Handle(context.Background(), record.Clone()))
	require.NoError(t, syncHandler.Handle(context.Background(), record.Clone()))

	asyncHandler.Flush(context.Background())

	require.Equal(t, 1, asyncRec.count(), "async handler should have delivered one message")
	require.Equal(t, 1, syncRec.count(), "sync handler should have delivered one message")

	asyncRec.mu.Lock()
	defer asyncRec.mu.Unlock()
	syncRec.mu.Lock()
	defer syncRec.mu.Unlock()
	assert.Equal(t, syncRec.messages[0], asyncRec.messages[0], "the payload must not depend on the delivery mode")
}

func TestSlackHandler_DerivedHandlersShareOneSender(t *testing.T) {
	_, server := newRecordingSlackServer(t, http.StatusOK, nil)
	handler := newTestSlackHandler(t, slackOptionsFor(t, server))

	const derivations = 100
	goroutinesBefore := runtime.NumGoroutine()

	derived := slog.Handler(handler)
	for i := range derivations {
		derived = derived.WithAttrs([]slog.Attr{slog.Int("i", i)})
		derived = derived.WithGroup("group")

		slackDerived, ok := derived.(*SlackHandler)
		require.True(t, ok, "derived handler should still be a *SlackHandler")
		assert.Same(t, handler.sender, slackDerived.sender, "every derived handler must share the one sender")
	}

	// Rule R3: an upper bound, not an exact match. NumGoroutine is
	// process-wide, so only its growth is meaningful here.
	require.Eventually(t, func() bool {
		return runtime.NumGoroutine() <= goroutinesBefore+2
	}, 2*time.Second, 10*time.Millisecond, "deriving handlers must not start more workers")
}

func TestSlackHandler_DryRunCreatesNoSenderAndSendsNothing(t *testing.T) {
	rec, server := newRecordingSlackServer(t, http.StatusOK, nil)

	opts := slackOptionsFor(t, server)
	opts.IsDryRun = true
	handler := newTestSlackHandler(t, opts)

	require.Nil(t, handler.sender, "dry-run must not build a sender")
	require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "message")))

	assert.Equal(t, FlushStats{}, handler.Flush(context.Background()), "Flush on a dry-run handler returns the zero value")
	assert.Equal(t, FlushStats{}, handler.Close(), "Close on a dry-run handler returns the zero value")
	assert.Equal(t, 0, rec.count(), "dry-run must not reach Slack")
}

func TestSlackSender_HighPriorityBypassesFullNormalQueue(t *testing.T) {
	block, release := newBlock(t)
	rec, server := newRecordingSlackServer(t, http.StatusOK, block)

	opts := slackOptionsFor(t, server)
	opts.NormalQueueSize = 1
	opts.HighPriorityQueueSize = 4
	handler := newTestSlackHandler(t, opts)

	ctx := context.Background()

	// The first notification pins the worker inside a send.
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "in flight")))
	waitForRequests(t, rec, 1)

	// The normal queue (capacity 1) now fills up and then overflows.
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "queued normal")))
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "dropped normal")))

	// A full normal queue must not keep an alert out.
	require.NoError(t, handler.Handle(ctx, securityAlertRecord("intrusion")))

	release()
	waitForRequests(t, rec, 3)
	stats := handler.Flush(ctx)

	texts := rec.texts()
	require.Len(t, texts, 3, "the overflowing normal notification should have been dropped: %v", texts)
	assert.Contains(t, texts[0], "in flight")
	assert.Contains(t, texts[1], "intrusion", "the alert must be sent before the queued normal notification")
	assert.Contains(t, texts[2], "queued normal")
	assert.Equal(t, int64(1), stats.Dropped, "exactly the overflowing normal notification is dropped")
}

func TestSlackSender_QueueOverflowDropsAndRecords(t *testing.T) {
	tests := []struct {
		name        string
		opts        func(*SlackHandlerOptions)
		record      func() slog.Record
		messageType string
	}{
		{
			name:        "normal queue",
			opts:        func(o *SlackHandlerOptions) { o.NormalQueueSize = 1 },
			record:      func() slog.Record { return slackRecord(slog.LevelInfo, "", "normal") },
			messageType: "",
		},
		{
			name:        "high priority queue",
			opts:        func(o *SlackHandlerOptions) { o.HighPriorityQueueSize = 1 },
			record:      func() slog.Record { return securityAlertRecord("intrusion") },
			messageType: messageTypeSecurityAlert,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			block, release := newBlock(t)
			rec, server := newRecordingSlackServer(t, http.StatusOK, block)

			var failureLog syncBuffer
			opts := slackOptionsFor(t, server)
			opts.FailureHandlers = failureLogHandlers(&failureLog)
			tt.opts(&opts)
			handler := newTestSlackHandler(t, opts)

			ctx := context.Background()

			// One in flight, one queued, one over capacity.
			require.NoError(t, handler.Handle(ctx, tt.record()))
			waitForRequests(t, rec, 1)
			require.NoError(t, handler.Handle(ctx, tt.record()))
			require.NoError(t, handler.Handle(ctx, tt.record()))

			waitForFailureLog(t, &failureLog, dropReasonQueueFull)

			release()
			stats := handler.Flush(ctx)

			assert.Equal(t, int64(1), stats.Dropped, "exactly one notification should be dropped")
			assert.Equal(t, int64(3), stats.Submitted)
			assert.Equal(t, int64(2), stats.Enqueued)
			assert.Equal(t, 1, strings.Count(failureLog.String(), dropReasonQueueFull), "the drop should be recorded once")
			assert.Contains(t, failureLog.String(), `"message_type":"`+tt.messageType+`"`)
		})
	}
}

// TestSlackSender_RecordsOmitMessageBody covers both records that name a lost
// notification. The marker is carried as the record message with no
// message_type, so the generic builder puts it into SlackMessage.Text: with a
// typed message_type the builders ignore r.Message and the assertion would hold
// no matter what the failure logger wrote. The first case proves the marker
// really does reach the payload.
func TestSlackSender_RecordsOmitMessageBody(t *testing.T) {
	const secret = "unique-body-marker-9f3a"

	t.Run("the marker reaches the Slack payload", func(t *testing.T) {
		rec, server := newRecordingSlackServer(t, http.StatusOK, nil)
		handler := newTestSlackHandler(t, slackOptionsFor(t, server))

		require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelWarn, "", secret)))
		waitForRequests(t, rec, 1)
		require.Contains(t, rec.texts()[0], secret, "the body assertions below would be vacuous otherwise")
	})

	tests := []struct {
		name       string
		serverCode int
		// act drives the handler into the record under test and returns the
		// reason that record should carry.
		act func(t *testing.T, handler *SlackHandler, failureLog *syncBuffer) string
	}{
		{
			name:       "drop record",
			serverCode: http.StatusOK,
			act: func(t *testing.T, handler *SlackHandler, _ *syncBuffer) string {
				// After the flush the sender no longer accepts.
				handler.Flush(context.Background())
				require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelWarn, "", secret)))
				return dropReasonSenderClosed
			},
		},
		{
			name:       "send failure record",
			serverCode: http.StatusBadRequest,
			act: func(t *testing.T, handler *SlackHandler, _ *syncBuffer) string {
				require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelWarn, "", secret)))
				return failureReasonSendFailed
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, server := newRecordingSlackServer(t, tt.serverCode, nil)

			var failureLog syncBuffer
			opts := slackOptionsFor(t, server)
			opts.FailureHandlers = failureLogHandlers(&failureLog)
			handler := newTestSlackHandler(t, opts)

			reason := tt.act(t, handler, &failureLog)
			waitForFailureLog(t, &failureLog, reason)

			record := findFailureRecord(t, &failureLog, reason)
			assert.Equal(t, "", record["message_type"], "the record names the notification's type")
			assert.Equal(t, "test-run", record["run_id"])
			assert.Equal(t, "WARN", record["level"])
			assert.NotContains(t, failureLog.String(), secret,
				"a record naming a lost notification must not carry its body")
		})
	}
}

// findFailureRecordByMessage returns the first record in buf with the given
// msg field.
func findFailureRecordByMessage(t *testing.T, buf *syncBuffer, msg string) map[string]any {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record["msg"] == msg {
			return record
		}
	}
	t.Fatalf("no record with msg %q in: %s", msg, buf.String())
	return nil
}

// findFailureRecord returns the first record in buf whose reason field matches.
func findFailureRecord(t *testing.T, buf *syncBuffer, reason string) map[string]any {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if record["reason"] == reason {
			return record
		}
	}
	t.Fatalf("no record with reason %q in: %s", reason, buf.String())
	return nil
}

func TestSlackSender_FailureLogGoesToNonSlackDestination(t *testing.T) {
	rec, server := newRecordingSlackServer(t, http.StatusBadRequest, nil)

	var failureLog syncBuffer
	opts := slackOptionsFor(t, server)
	opts.FailureHandlers = failureLogHandlers(&failureLog)
	handler := newTestSlackHandler(t, opts)

	ctx := context.Background()
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "message")))
	waitForFailureLog(t, &failureLog, "Slack notification not delivered")

	stats := handler.Flush(ctx)
	require.Equal(t, int64(1), stats.Failed)

	assert.Contains(t, failureLog.String(), failureReasonSendFailed, "the failure must be recorded to the non-Slack destination")
	assert.Equal(t, 1, rec.count(), "recording the failure must not itself produce a Slack request")
}

func TestSlackSender_FlushLogsMessageTypeBreakdown(t *testing.T) {
	rec, server := newRecordingSlackServer(t, http.StatusOK, nil)

	var failureLog syncBuffer
	opts := slackOptionsFor(t, server)
	opts.RunID = "run-breakdown"
	opts.FailureHandlers = failureLogHandlers(&failureLog)
	handler := newTestSlackHandler(t, opts)

	ctx := context.Background()
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, messageTypeCommandGroupSummary, "summary")))
	require.NoError(t, handler.Handle(ctx, securityAlertRecord("intrusion")))
	waitForRequests(t, rec, 2)

	handler.Flush(ctx)

	aggregate := findFailureRecordByMessage(t, &failureLog, "Slack delivery summary")
	assert.Equal(t, "run-breakdown", aggregate["run_id"], "the aggregate carries the sender's run ID")
	assert.Equal(t, map[string]any{
		messageTypeCommandGroupSummary: float64(1),
		messageTypeSecurityAlert:       float64(1),
	}, aggregate["sent_by_message_type"], "each delivered notification counts under its own type")
	assert.Empty(t, aggregate["failed_by_message_type"])
	assert.Empty(t, aggregate["dropped_by_message_type"])
}

// TestSlackHandler_SendContextIsDetachedFromTheLogCall pins that the worker
// does not inherit the caller's context. Carrying it through would let
// cmd/runner's signal cancellation abort exactly the notifications that
// shutdown needs to deliver, and every other test here passes a live context,
// so nothing else would notice the change.
func TestSlackHandler_SendContextIsDetachedFromTheLogCall(t *testing.T) {
	rec, server := newRecordingSlackServer(t, http.StatusOK, nil)
	handler := newTestSlackHandler(t, slackOptionsFor(t, server))

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	require.NoError(t, handler.Handle(cancelled, slackRecord(slog.LevelInfo, "", "issued under a cancelled context")))

	stats := handler.Flush(context.Background())
	assert.Equal(t, int64(1), stats.Sent, "a cancelled log-call context must not abort the send")
	assert.Equal(t, 1, rec.count())
}

func TestSlackHandler_FlushDeliversPendingAndReturnsStats(t *testing.T) {
	rec, server := newRecordingSlackServer(t, http.StatusOK, nil)
	handler := newTestSlackHandler(t, slackOptionsFor(t, server))

	// Park the worker before its first send so that everything submitted is
	// still queued when Flush runs: this is the drain path under test.
	reached, release := gateWorkerAtDequeue(handler.sender)

	ctx := context.Background()
	const submitted = 5
	for range submitted {
		require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "queued")))
	}
	<-reached

	statsCh := make(chan FlushStats, 1)
	go func() {
		flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		statsCh <- handler.Flush(flushCtx)
	}()

	waitForStopAccepting(t, handler.sender)
	release()

	var stats FlushStats
	select {
	case stats = <-statsCh:
	case <-time.After(15 * time.Second):
		t.Fatal("Flush did not return")
	}

	assert.Equal(t, int64(submitted), stats.Sent, "the flush should deliver everything queued")
	assert.Equal(t, int64(0), stats.Pending)
	assert.Equal(t, submitted, rec.count())
}

func TestSlackHandler_FlushDeadlineReportsPending(t *testing.T) {
	block, _ := newBlock(t)
	rec, server := newRecordingSlackServer(t, http.StatusOK, block)
	handler := newTestSlackHandler(t, slackOptionsFor(t, server))

	ctx := context.Background()
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "in flight")))
	waitForRequests(t, rec, 1)
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "queued")))

	flushCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	stats := handler.Flush(flushCtx)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 5*time.Second, "Flush must return near its deadline, not the send deadline")
	assert.Equal(t, int64(2), stats.Pending, "neither notification was delivered")
	assert.Equal(t, int64(0), stats.Sent)
	assertWorkerStopped(t, handler.sender)
}

func TestSlackHandler_FlushIsIdempotent(t *testing.T) {
	rec, server := newRecordingSlackServer(t, http.StatusOK, nil)

	var failureLog syncBuffer
	opts := slackOptionsFor(t, server)
	opts.FailureHandlers = failureLogHandlers(&failureLog)
	handler := newTestSlackHandler(t, opts)

	ctx := context.Background()
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "message")))
	waitForRequests(t, rec, 1)

	first := handler.Flush(ctx)
	second := handler.Flush(ctx)
	afterClose := handler.Close()

	assert.Equal(t, first, second, "repeated Flush returns the same accounting")
	assert.Equal(t, first, afterClose, "Close after Flush returns the same accounting")
	assert.Equal(t, 1, strings.Count(failureLog.String(), "Slack delivery summary"),
		"the aggregate must be emitted once per sender, so operators cannot double-count it")
}

func TestSlackHandler_EnqueueAfterFlushIsDropped(t *testing.T) {
	rec, server := newRecordingSlackServer(t, http.StatusOK, nil)

	var failureLog syncBuffer
	opts := slackOptionsFor(t, server)
	opts.FailureHandlers = failureLogHandlers(&failureLog)
	handler := newTestSlackHandler(t, opts)

	ctx := context.Background()
	handler.Flush(ctx)

	// No panic even though the worker is gone: the send queues are never
	// closed, so this cannot be a send on a closed channel.
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "after flush")))

	waitForFailureLog(t, &failureLog, dropReasonSenderClosed)
	assert.Equal(t, int64(1), handler.sender.counters.submitted.Load())
	assert.Equal(t, int64(0), handler.sender.counters.enqueued.Load(), "nothing may enter a queue after the flush")
	assert.Equal(t, 0, rec.count())
}

func TestSlackHandler_FlushReturnsWhenWorkerIsIdle(t *testing.T) {
	const limit = time.Second

	t.Run("nothing ever enqueued", func(t *testing.T) {
		_, server := newRecordingSlackServer(t, http.StatusOK, nil)
		handler := newTestSlackHandler(t, slackOptionsFor(t, server))

		start := time.Now()
		handler.Flush(context.Background())
		assert.Less(t, time.Since(start), limit, "Flush must wake a worker parked on empty queues")
		assertWorkerStopped(t, handler.sender)
	})

	t.Run("everything already sent", func(t *testing.T) {
		rec, server := newRecordingSlackServer(t, http.StatusOK, nil)
		handler := newTestSlackHandler(t, slackOptionsFor(t, server))

		require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "message")))
		waitForRequests(t, rec, 1)

		start := time.Now()
		handler.Close()
		assert.Less(t, time.Since(start), limit, "Close must wake a worker parked on empty queues")
		assertWorkerStopped(t, handler.sender)
	})
}

func TestSlackHandler_FlushCancelsInFlightSend(t *testing.T) {
	t.Run("unresponsive server is cut short at the flush deadline", func(t *testing.T) {
		block, _ := newBlock(t)
		rec, server := newRecordingSlackServer(t, http.StatusOK, block)
		handler := newTestSlackHandler(t, slackOptionsFor(t, server))

		ctx := context.Background()
		require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "in flight")))
		waitForRequests(t, rec, 1)

		flushCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()

		start := time.Now()
		stats := handler.Flush(flushCtx)
		elapsed := time.Since(start)

		// Without re-bounding the in-flight send this would sit on the 40s
		// send deadline instead.
		assert.Less(t, elapsed, 2*time.Second, "Flush must re-bound the in-flight send to the flush budget")
		assert.Equal(t, int64(1), stats.Pending, "the interrupted notification is pending, not failed")
		assert.Equal(t, int64(0), stats.Failed)
		assertWorkerStopped(t, handler.sender)
	})

	t.Run("responsive server still delivers the in-flight notification", func(t *testing.T) {
		// The notification issued just before exit is normally still in flight
		// when the flush starts. Cancelling it outright would lose exactly the
		// message the flush exists to deliver.
		block, release := newBlock(t)
		rec, server := newRecordingSlackServer(t, http.StatusOK, block)
		handler := newTestSlackHandler(t, slackOptionsFor(t, server))

		ctx := context.Background()
		require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "in flight")))
		waitForRequests(t, rec, 1)

		flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		statsCh := make(chan FlushStats, 1)
		go func() { statsCh <- handler.Flush(flushCtx) }()

		waitForStopAccepting(t, handler.sender)
		release()

		select {
		case stats := <-statsCh:
			assert.Equal(t, int64(1), stats.Sent, "an in-flight send that completes within the flush budget is delivered")
			assert.Equal(t, int64(0), stats.Pending)
		case <-time.After(15 * time.Second):
			t.Fatal("Flush did not return")
		}
		assertWorkerStopped(t, handler.sender)
	})
}

func TestSlackSender_DequeueRegisterBoundary(t *testing.T) {
	// The worker is parked between taking a request off the queue and
	// registering its cancel function. If those were two critical sections, a
	// Flush landing here would find nothing to cancel and would wait past its
	// deadline for a worker that never returns to its select.
	t.Run("flush", func(t *testing.T) {
		block, _ := newBlock(t)
		_, server := newRecordingSlackServer(t, http.StatusOK, block)
		handler := newTestSlackHandler(t, slackOptionsFor(t, server))

		reached, release := gateWorkerAtDequeue(handler.sender)
		require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "at the boundary")))
		<-reached

		statsCh := make(chan FlushStats, 1)
		go func() {
			flushCtx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			defer cancel()
			statsCh <- handler.Flush(flushCtx)
		}()

		waitForStopAccepting(t, handler.sender)
		release()

		select {
		case stats := <-statsCh:
			assert.Equal(t, int64(1), stats.Pending, "the request held at the boundary is pending")
		case <-time.After(5 * time.Second):
			t.Fatal("Flush did not return within the flush deadline")
		}
		assertWorkerStopped(t, handler.sender)
	})

	t.Run("close", func(t *testing.T) {
		block, _ := newBlock(t)
		rec, server := newRecordingSlackServer(t, http.StatusOK, block)
		handler := newTestSlackHandler(t, slackOptionsFor(t, server))

		reached, release := gateWorkerAtDequeue(handler.sender)
		require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "at the boundary")))
		<-reached

		statsCh := make(chan FlushStats, 1)
		go func() { statsCh <- handler.Close() }()

		waitForStopAccepting(t, handler.sender)
		release()

		select {
		case stats := <-statsCh:
			assert.Equal(t, int64(1), stats.Pending, "an abandoned request is pending")
		case <-time.After(5 * time.Second):
			t.Fatal("Close did not return")
		}
		assert.Equal(t, 0, rec.count(), "abandoning must not send the request it was holding")
		assertWorkerStopped(t, handler.sender)
	})
}

func TestSlackHandler_NilSenderHandleReturnsNil(t *testing.T) {
	// A handler built as a struct literal has no sender. Panicking inside a
	// log path is the worst possible failure mode, so it stays quiet instead.
	handler := &SlackHandler{runID: "test-run", level: slog.LevelInfo}

	require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "message")))
	assert.Equal(t, FlushStats{}, handler.Flush(context.Background()))
	assert.Equal(t, FlushStats{}, handler.Close())
}

func TestSlackHandler_SynchronousMode(t *testing.T) {
	rec, server := newRecordingSlackServer(t, http.StatusOK, nil)

	opts := slackOptionsFor(t, server)
	opts.Synchronous = true
	handler := newTestSlackHandler(t, opts)

	require.NotNil(t, handler.sender, "synchronous mode still needs the sender's HTTP state")
	assert.Nil(t, handler.sender.highPriority, "synchronous mode allocates no send queue")
	assert.Nil(t, handler.sender.normal, "synchronous mode allocates no send queue")
	assert.Nil(t, handler.sender.done, "synchronous mode starts no worker")

	require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "message")))

	// No flush needed: Handle only returns once the request has been answered.
	assert.Equal(t, 1, rec.count(), "synchronous mode sends inline")

	stats := handler.Flush(context.Background())
	assert.Equal(t, FlushStats{Submitted: 1, Enqueued: 1, Sent: 1}, stats,
		"switching to synchronous mode for debugging must not cost the delivery accounting")

	// Stopping accepting applies here too: without it a handler kept alive past
	// its Close would go on making blocking HTTP calls.
	require.NoError(t, handler.Handle(context.Background(), slackRecord(slog.LevelInfo, "", "after flush")))
	assert.Equal(t, 1, rec.count(), "a closed synchronous sender must not send")
}

func TestSlackSender_CounterInvariants(t *testing.T) {
	block, release := newBlock(t)
	rec, server := newRecordingSlackServer(t, http.StatusOK, block)

	opts := slackOptionsFor(t, server)
	opts.NormalQueueSize = 1
	handler := newTestSlackHandler(t, opts)

	ctx := context.Background()

	// Pin the worker inside a send so the queue overflows for sure.
	require.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "in flight")))
	waitForRequests(t, rec, 1)

	const concurrent = 20
	var wg sync.WaitGroup
	for range concurrent {
		wg.Go(func() {
			assert.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "concurrent")))
		})
	}
	wg.Wait()

	release()
	stats := handler.Flush(ctx)

	assert.Equal(t, int64(concurrent+1), stats.Submitted)
	assert.Equal(t, stats.Submitted, stats.Enqueued+stats.Dropped, "dropped notifications never enter a queue")
	assert.Equal(t, stats.Enqueued, stats.Sent+stats.Failed+stats.Pending, "every enqueued notification is accounted for")
	assert.Positive(t, stats.Dropped, "a queue of capacity 1 must overflow under this load")
}

func TestSlackHandler_ConcurrentHandleAndFlush(t *testing.T) {
	_, server := newRecordingSlackServer(t, http.StatusOK, nil)
	handler := newTestSlackHandler(t, slackOptionsFor(t, server))

	ctx := context.Background()

	const writers = 8
	var wg sync.WaitGroup
	for range writers {
		wg.Go(func() {
			for range 10 {
				assert.NoError(t, handler.Handle(ctx, slackRecord(slog.LevelInfo, "", "concurrent")))
			}
		})
	}

	wg.Go(func() {
		flushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		handler.Flush(flushCtx)
	})

	wg.Wait()

	stats := handler.Flush(ctx)
	assert.Equal(t, stats.Submitted, stats.Enqueued+stats.Dropped)
	assert.Equal(t, stats.Enqueued, stats.Sent+stats.Failed+stats.Pending)
	assertWorkerStopped(t, handler.sender)
}

// assertWorkerStopped observes worker termination through the done channel
// (rule R3).
func assertWorkerStopped(t *testing.T, sd *slackSender) {
	t.Helper()
	select {
	case <-sd.done:
	case <-time.After(time.Second):
		t.Fatal("worker goroutine is still running after the sender terminated")
	}
}
