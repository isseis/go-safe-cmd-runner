package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Environment variable names for the Slack delivery settings. This package only
// owns the names; reading the environment and interpreting the values is done
// by internal/runner/bootstrap, so every caller of NewSlackHandler gets the
// same behaviour from SlackHandlerOptions alone.
const (
	// SlackSendTimeoutEnvVar overrides the per-notification send deadline.
	SlackSendTimeoutEnvVar = "GSCR_SLACK_SEND_TIMEOUT"
	// SlackFlushTimeoutEnvVar overrides the deadline for the whole flush.
	SlackFlushTimeoutEnvVar = "GSCR_SLACK_FLUSH_TIMEOUT"
	// SlackSyncEnvVar selects synchronous sending, a debugging escape hatch.
	SlackSyncEnvVar = "GSCR_SLACK_SYNC"
)

const (
	// DefaultSendTimeout bounds one notification's delivery including retries.
	// The existing retry policy (5s HTTP timeout x 4 attempts, 2+4+8s backoff)
	// takes at most 34s when it runs to completion, so this leaves room for
	// scheduling jitter rather than cutting a healthy retry sequence short. It
	// is the last-resort safety net for a hung connection and normally never
	// fires. bootstrap reads it as the fallback for GSCR_SLACK_SEND_TIMEOUT.
	DefaultSendTimeout = 40 * time.Second
	// DefaultFlushTimeout bounds the whole flush at process exit. bootstrap
	// reads it as the fallback for GSCR_SLACK_FLUSH_TIMEOUT.
	DefaultFlushTimeout = 15 * time.Second
	// flushPerSendTimeout caps one send while draining. Flushing does not
	// retry: within a bounded deadline, sending more notifications once beats
	// insisting on one.
	flushPerSendTimeout = 5 * time.Second

	// Default send-queue capacities. The normal queue's 128 exceeds the command
	// count of a typical configuration; the high-priority queue's 32 rests on
	// the judgement that more than 32 security alerts in one run is already an
	// incident, where the count alone is enough.
	defaultHighPriorityQueueSize = 32
	defaultNormalQueueSize       = 128
)

// Message types carried by slack_notify records. They are named here because
// the queue-priority decision below and Handle's message builder switch must
// agree on the exact strings.
const (
	messageTypeCommandGroupSummary      = "command_group_summary"
	messageTypePreExecutionError        = "pre_execution_error"
	messageTypeSecurityAlert            = "security_alert"
	messageTypePrivilegedCommandFailure = "privileged_command_failure"
	messageTypePrivilegeEscalationFail  = "privilege_escalation_failure"
)

// Reasons recorded for a notification that was never delivered.
const (
	dropReasonQueueFull     = "queue_full"
	dropReasonSenderClosed  = "sender_closed"
	failureReasonSendFailed = "send_failed"
)

// slackSender owns the send queues and the worker goroutine for one webhook
// configuration. It is shared by pointer across handlers derived via
// WithAttrs / WithGroup so that the worker count stays bounded.
type slackSender struct {
	webhookURL    string
	httpClient    *http.Client
	backoffConfig BackoffConfig
	failureLogger *slog.Logger
	sendTimeout   time.Duration
	// runID is copied from SlackHandlerOptions.RunID at construction.
	// Per-request records take the run ID from slackRequest, but the aggregate
	// record that flush emits belongs to no single request, so it reads run_id
	// from here.
	runID string

	// synchronous senders hold no queues and no worker: Handle sends inline.
	synchronous bool

	// highPriority and normal are the two send queues. They are never closed,
	// so a send on a closed channel cannot happen; arrivals after the sender
	// stops accepting are excluded by the lock instead.
	highPriority chan slackRequest
	normal       chan slackRequest

	// shutdown carries the one and only termination request to a worker parked
	// on empty queues. Capacity 1, so the send never blocks.
	shutdown chan shutdownRequest
	// done is closed by the worker just before it returns. It is nil when there
	// is no worker (synchronous mode).
	done chan struct{}

	// afterDequeue is a test-only synchronisation point: the worker calls it,
	// when non-nil, right after taking a request off a queue and before the
	// send begins. Production never assigns it; only slack_sender_test.go does,
	// to park a worker in the dequeued-but-not-yet-registered position.
	afterDequeue func()

	mu sync.RWMutex
	// The following are guarded by mu.
	closed         bool
	shutdownState  shutdownRequest    // meaningful only once closed is true
	inFlightCancel context.CancelFunc // nil unless a send is in flight
	flushStats     FlushStats
	statsRecorded  bool
	// Per-message_type breakdowns reported by the aggregate record at flush.
	sentByType    map[string]int
	failedByType  map[string]int
	droppedByType map[string]int

	// aggregateOnce keeps the flush aggregate to one record per sender, so an
	// operator totalling the breakdown across a run cannot double-count a
	// sender whose Flush and Close were both called.
	aggregateOnce sync.Once

	counters slackCounters
}

// slackRequest is one queued notification. It carries only what the worker
// needs: the payload to POST, and the fields the failure logger records when
// the send fails or the notification is dropped. The record body is
// deliberately absent from the failure path, so nothing beyond these fields is
// retained for logging.
type slackRequest struct {
	message     *SlackMessage
	messageType string
	runID       string
	level       slog.Level
}

// shutdownRequest tells the worker how to terminate. It is both the element of
// the shutdown channel and the value stored in slackSender.shutdownState, so a
// worker that has just dequeued a request observes the same instruction as one
// parked in select.
type shutdownRequest struct {
	// abandon is false for a drain (Flush) and true for an abandon (Close).
	abandon bool
	// ctx carries the flush deadline. The worker derives each send's timeout
	// from it while draining.
	ctx context.Context
}

// slackCounters holds the cumulative counters behind FlushStats. They are
// atomic rather than mu-guarded because Handle increments Submitted and
// Dropped while holding only the read lock.
type slackCounters struct {
	submitted atomic.Int64
	enqueued  atomic.Int64
	sent      atomic.Int64
	failed    atomic.Int64
	dropped   atomic.Int64
}

// FlushStats reports a sender's delivery accounting. Every counter except
// Pending is cumulative since the sender was created. Dropped notifications
// never enter a queue, so they are not part of Enqueued; the accounting is a
// two-level partition and both equations hold when Flush returns:
//
//	Submitted == Enqueued + Dropped
//	Enqueued  == Sent + Failed + Pending
//
// Substituting the second into the first gives the flat breakdown of every
// notification that ever reached the enqueue decision point:
//
//	Submitted == Sent + Failed + Dropped + Pending
type FlushStats struct {
	// Submitted is the total number of notifications that reached the enqueue
	// decision point, i.e. those that passed the slack_notify, dry-run and
	// nil-sender checks. Each one is either enqueued or dropped.
	Submitted int64
	// Enqueued is the number of notifications accepted into a send queue.
	Enqueued int64
	// Sent is the number of notifications delivered successfully.
	Sent int64
	// Failed is the number of notifications whose send attempts all failed.
	Failed int64
	// Dropped is the number of notifications discarded without any send
	// attempt, either because the queue was full or because the sender had
	// stopped accepting.
	Dropped int64
	// Pending is the number of enqueued notifications still undelivered when
	// Flush returned, including one that was in flight when the flush deadline
	// or the in-flight send's re-bounded budget expired.
	Pending int64
}

// newSlackSender builds the sender for one webhook configuration and, unless
// synchronous is set, starts its worker goroutine.
func newSlackSender(opts SlackHandlerOptions, httpClient *http.Client, backoffConfig BackoffConfig, failureLogger *slog.Logger) *slackSender {
	sendTimeout := opts.SendTimeout
	if sendTimeout <= 0 {
		sendTimeout = DefaultSendTimeout
	}

	sd := &slackSender{
		webhookURL:    opts.WebhookURL,
		httpClient:    httpClient,
		backoffConfig: backoffConfig,
		failureLogger: failureLogger,
		sendTimeout:   sendTimeout,
		runID:         opts.RunID,
		synchronous:   opts.Synchronous,
		sentByType:    make(map[string]int),
		failedByType:  make(map[string]int),
		droppedByType: make(map[string]int),
	}

	if opts.Synchronous {
		// No queues, no worker: Handle sends inline.
		return sd
	}

	highSize := opts.HighPriorityQueueSize
	if highSize <= 0 {
		highSize = defaultHighPriorityQueueSize
	}
	normalSize := opts.NormalQueueSize
	if normalSize <= 0 {
		normalSize = defaultNormalQueueSize
	}

	sd.highPriority = make(chan slackRequest, highSize)
	sd.normal = make(chan slackRequest, normalSize)
	sd.shutdown = make(chan shutdownRequest, 1)
	sd.done = make(chan struct{})

	go sd.run()

	return sd
}

// hasWorker reports whether this sender runs a worker goroutine. Synchronous
// senders do not.
func (sd *slackSender) hasWorker() bool {
	return sd.done != nil
}

// isHighPriority reports whether a message type goes to the high-priority
// queue. Security alerts, privilege-escalation failures and pre-execution
// errors must not be pushed out by a flood of ordinary command notifications.
func isHighPriority(messageType string) bool {
	switch messageType {
	case messageTypeSecurityAlert, messageTypePrivilegeEscalationFail, messageTypePreExecutionError:
		return true
	default:
		return false
	}
}

// queueFor selects the send queue for a request.
func (sd *slackSender) queueFor(messageType string) chan slackRequest {
	if isHighPriority(messageType) {
		return sd.highPriority
	}
	return sd.normal
}

// enqueue submits one notification for asynchronous delivery. It never blocks:
// a full queue or a sender that has stopped accepting drops the notification
// and records it.
func (sd *slackSender) enqueue(req slackRequest) {
	if reason, ok := sd.tryEnqueue(req); !ok {
		// Recorded outside the read lock: recordDrop takes the write lock for
		// the breakdown map, which would deadlock against a held read lock.
		sd.recordDrop(req, reason)
	}
}

// tryEnqueue performs the accepting check and the non-blocking put in one
// critical section under the read lock. Holding the lock across both is what
// makes a notification that slipped in after Flush observed an empty queue
// impossible: Flush cannot take the write lock while any enqueue is in
// progress.
func (sd *slackSender) tryEnqueue(req slackRequest) (string, bool) {
	sd.mu.RLock()
	defer sd.mu.RUnlock()

	sd.counters.submitted.Add(1)

	if sd.closed {
		return dropReasonSenderClosed, false
	}

	select {
	case sd.queueFor(req.messageType) <- req:
		sd.counters.enqueued.Add(1)
		return "", true
	default:
		return dropReasonQueueFull, false
	}
}

// sendSync delivers one notification inline, for synchronous mode.
func (sd *slackSender) sendSync(ctx context.Context, req slackRequest) error {
	sd.counters.submitted.Add(1)

	sendCtx, cancel := context.WithTimeout(ctx, sd.sendTimeout)
	defer cancel()

	err := sd.send(sendCtx, req, false)
	if err != nil {
		sd.recordFailure(req, err)
		return err
	}
	sd.recordSent(req)
	return nil
}

// run is the worker loop. It serves the high-priority queue ahead of the
// normal one, and parks on a select that also watches the shutdown channel so
// that a Flush on an idle sender still returns.
func (sd *slackSender) run() {
	defer close(sd.done)

	for {
		// Priority pass: take from the high-priority queue whenever it has
		// anything, without giving the normal queue a chance to win the select.
		select {
		case req := <-sd.highPriority:
			drainCtx, stop := sd.serve(req)
			if stop {
				return
			}
			if drainCtx != nil {
				sd.drain(drainCtx)
				return
			}
			continue
		default:
		}

		select {
		case req := <-sd.highPriority:
			drainCtx, stop := sd.serve(req)
			if stop {
				return
			}
			if drainCtx != nil {
				sd.drain(drainCtx)
				return
			}
		case req := <-sd.normal:
			drainCtx, stop := sd.serve(req)
			if stop {
				return
			}
			if drainCtx != nil {
				sd.drain(drainCtx)
				return
			}
		case sr := <-sd.shutdown:
			if sr.abandon {
				return
			}
			sd.drain(sr.ctx)
			return
		}
	}
}

// serve sends one dequeued request. It returns the flush context when a drain
// was observed while registering the send (the caller then finishes the queues
// under it), and stop=true when an abandon was observed.
//
// Observing the shutdown state and registering the cancel function happen in
// one critical section on purpose. Split in two, a Flush landing between them
// would see no cancel function to call, park waiting for a worker that never
// returns to its select, and blow through the flush deadline with the worker
// still alive.
func (sd *slackSender) serve(req slackRequest) (context.Context, bool) {
	if sd.afterDequeue != nil {
		sd.afterDequeue()
	}

	sd.mu.Lock()
	state := sd.shutdownState
	draining := sd.closed && !state.abandon
	if sd.closed && state.abandon {
		sd.mu.Unlock()
		// Not sent and not failed: it stays in the Pending remainder.
		return nil, true
	}

	var sendCtx context.Context
	var cancel context.CancelFunc
	if draining {
		sendCtx, cancel = context.WithTimeout(state.ctx, perSendTimeout(state.ctx))
	} else {
		// Detached from the caller's context on purpose: carrying the log
		// call's context through would let cmd/runner's signal cancellation
		// abort exactly the notifications that shutdown needs to deliver.
		sendCtx, cancel = context.WithTimeout(context.Background(), sd.sendTimeout)
	}
	sd.inFlightCancel = cancel
	sd.mu.Unlock()

	err := sd.send(sendCtx, req, draining)

	sd.mu.Lock()
	sd.inFlightCancel = nil
	// A send terminated by shutdown -- Flush cancelling the in-flight attempt,
	// or the flush deadline expiring -- is not a delivery failure. It is left
	// out of both Sent and Failed so that it lands in the Pending remainder.
	interrupted := err != nil && sendCtx.Err() != nil && sd.closed
	sd.mu.Unlock()

	cancel()

	switch {
	case err == nil:
		sd.recordSent(req)
	case interrupted:
		// counted as Pending; no record beyond the flush aggregate.
	default:
		sd.recordFailure(req, err)
	}

	if draining {
		return state.ctx, false
	}
	return nil, false
}

// drain sends the remaining queued notifications, high priority first, until
// the queues are empty or the flush deadline expires.
func (sd *slackSender) drain(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}

		req, ok := sd.dequeue()
		if !ok {
			return
		}

		if _, stop := sd.serve(req); stop {
			return
		}
	}
}

// dequeue takes the next queued request without blocking, high priority first.
func (sd *slackSender) dequeue() (slackRequest, bool) {
	select {
	case req := <-sd.highPriority:
		return req, true
	default:
	}
	select {
	case req := <-sd.normal:
		return req, true
	default:
		return slackRequest{}, false
	}
}

// perSendTimeout is the deadline for one send while draining: the smaller of
// the remaining flush deadline and flushPerSendTimeout.
func perSendTimeout(flushCtx context.Context) time.Duration {
	deadline, ok := flushCtx.Deadline()
	if !ok {
		return flushPerSendTimeout
	}
	return min(time.Until(deadline), flushPerSendTimeout)
}

// flush stops accepting notifications, asks the worker to drain what is queued
// under ctx, and waits for it to terminate.
func (sd *slackSender) flush(ctx context.Context) FlushStats {
	return sd.terminate(shutdownRequest{abandon: false, ctx: ctx})
}

// close stops accepting notifications and asks the worker to terminate without
// draining. A drain already requested by flush is not overridden.
func (sd *slackSender) close() FlushStats {
	return sd.terminate(shutdownRequest{abandon: true, ctx: context.Background()})
}

// terminate implements flush and close. Only the caller that actually set the
// stop-accepting flag sends the termination request, so the capacity-1 channel
// never blocks and a later close cannot downgrade an in-progress drain.
func (sd *slackSender) terminate(req shutdownRequest) FlushStats {
	sd.mu.Lock()
	first := !sd.closed
	if first {
		sd.closed = true
		sd.shutdownState = req
		sd.boundInFlightLocked(req)
	}
	sd.mu.Unlock()

	if !sd.hasWorker() {
		// Synchronous mode has no worker and no queued remainder, so there is
		// nothing to account for.
		return FlushStats{}
	}

	if first {
		sd.shutdown <- req
	}

	<-sd.done

	sd.mu.Lock()
	if !sd.statsRecorded {
		sd.flushStats = sd.statsLocked()
		sd.statsRecorded = true
	}
	stats := sd.flushStats
	sd.mu.Unlock()

	sd.aggregateOnce.Do(func() { sd.logAggregate(stats) })

	return stats
}

// boundInFlightLocked keeps a send that is already in flight from outliving the
// flush, which it otherwise could: it is running under the 40s send deadline.
// A drain re-bounds it to the same budget a drained send gets rather than
// cancelling it outright, because the notification issued just before exit is
// usually milliseconds from delivery and is exactly the one the flush exists to
// deliver. An abandon has nothing to deliver, so it cancels immediately.
// Cancelling a send that has already finished is a no-op, so the timer is safe
// to leave armed.
func (sd *slackSender) boundInFlightLocked(req shutdownRequest) {
	cancel := sd.inFlightCancel
	if cancel == nil {
		return
	}

	if req.abandon {
		cancel()
		return
	}

	budget := perSendTimeout(req.ctx)
	if budget <= 0 {
		cancel()
		return
	}
	time.AfterFunc(budget, cancel)
}

// statsLocked snapshots the counters. Pending is the remainder: everything
// accepted into a queue that neither succeeded nor failed.
func (sd *slackSender) statsLocked() FlushStats {
	submitted := sd.counters.submitted.Load()
	enqueued := sd.counters.enqueued.Load()
	sent := sd.counters.sent.Load()
	failed := sd.counters.failed.Load()

	return FlushStats{
		Submitted: submitted,
		Enqueued:  enqueued,
		Sent:      sent,
		Failed:    failed,
		Dropped:   sd.counters.dropped.Load(),
		// Pending is the remainder rather than a counter of its own: a
		// notification is pending precisely when it entered a queue and the
		// worker neither delivered nor failed it, which includes the one cut
		// short at the flush budget and the one an abandon left unsent.
		Pending: enqueued - sent - failed,
	}
}

// recordSent accounts for a delivered notification.
func (sd *slackSender) recordSent(req slackRequest) {
	sd.counters.sent.Add(1)
	sd.mu.Lock()
	sd.sentByType[req.messageType]++
	sd.mu.Unlock()
}

// recordFailure accounts for a notification whose send attempts all failed and
// records it individually, so an operator can tell which notification was lost
// rather than only how many. The notification body is deliberately absent: it
// is already in this run's JSON log, reachable by run_id and message_type.
func (sd *slackSender) recordFailure(req slackRequest, err error) {
	sd.counters.failed.Add(1)
	sd.mu.Lock()
	sd.failedByType[req.messageType]++
	sd.mu.Unlock()

	sd.failureLogger.Warn("Slack notification not delivered",
		slog.String("reason", failureReasonSendFailed),
		slog.String("message_type", req.messageType),
		slog.String("run_id", req.runID),
		slog.String("level", req.level.String()),
		slog.String("error", sanitizeErrorForLog(err)), //nolint:gosec // G706: error is sanitized via sanitizeErrorForLog, run_id is an internal identifier
	)
}

// recordDrop accounts for a notification discarded without any send attempt.
// See recordFailure for why the body is not recorded.
func (sd *slackSender) recordDrop(req slackRequest, reason string) {
	sd.counters.dropped.Add(1)
	sd.mu.Lock()
	sd.droppedByType[req.messageType]++
	sd.mu.Unlock()

	sd.failureLogger.Warn("Slack notification dropped",
		slog.String("reason", reason),
		slog.String("message_type", req.messageType),
		slog.String("run_id", req.runID), //nolint:gosec // G706: run_id is an internal identifier, not user input
		slog.String("level", req.level.String()),
	)
}

// logAggregate emits the one per-sender summary of the flush, with the
// per-message_type breakdown. Its run_id comes from the sender rather than any
// single request, so it lines up with the individual records above.
func (sd *slackSender) logAggregate(stats FlushStats) {
	sd.mu.RLock()
	sent := maps.Clone(sd.sentByType)
	failed := maps.Clone(sd.failedByType)
	dropped := maps.Clone(sd.droppedByType)
	sd.mu.RUnlock()

	sd.failureLogger.Info("Slack delivery summary",
		slog.String("run_id", sd.runID), //nolint:gosec // G706: run_id is an internal identifier, not user input
		slog.Int64("submitted", stats.Submitted),
		slog.Int64("sent", stats.Sent),
		slog.Int64("failed", stats.Failed),
		slog.Int64("dropped", stats.Dropped),
		slog.Int64("pending", stats.Pending),
		slog.Any("sent_by_message_type", sent),
		slog.Any("failed_by_message_type", failed),
		slog.Any("dropped_by_message_type", dropped),
	)
}

// send posts one message to Slack. With singleAttempt set (flushing) it tries
// exactly once; otherwise it applies the configured retry and backoff policy.
func (sd *slackSender) send(ctx context.Context, req slackRequest, singleAttempt bool) error {
	payload, err := json.Marshal(req.message)
	if err != nil {
		sd.failureLogger.Error("Failed to marshal Slack message", slog.String("error", sanitizeErrorForLog(err)), slog.String("run_id", req.runID))
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	sd.failureLogger.Debug("Sending Slack notification", slog.String("run_id", req.runID), slog.String("message_text", req.message.Text))

	var lastErr error

	retryCount := sd.backoffConfig.RetryCount
	if singleAttempt {
		retryCount = 0
	}

	backoffIntervals := generateBackoffIntervals(sd.backoffConfig.Base, retryCount)
	for attempt := 0; attempt <= retryCount; attempt++ {
		if attempt > 0 {
			// Get backoff interval from predefined list
			backoff := backoffIntervals[attempt-1]
			sd.failureLogger.Debug("Retrying Slack notification", slog.Int("attempt", attempt+1), slog.Float64("backoff_seconds", backoff.Seconds()), slog.String("run_id", req.runID))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, sd.webhookURL, bytes.NewBuffer(payload))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			sd.failureLogger.Warn("Failed to create Slack request", slog.String("error", sanitizeErrorForLog(err)), slog.Int("attempt", attempt+1), slog.String("run_id", req.runID))
			continue
		}

		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := sd.httpClient.Do(httpReq) //nolint:gosec // G704: URL is a configured Slack webhook, not user-controlled
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			sd.failureLogger.Warn("Failed to send Slack request", slog.String("error", sanitizeErrorForLog(err)), slog.Int("attempt", attempt+1), slog.String("run_id", req.runID)) //nolint:gosec // G706: error is sanitized via sanitizeErrorForLog, run_id is an internal identifier
			continue
		}

		statusCode := resp.StatusCode
		if err := resp.Body.Close(); err != nil {
			sd.failureLogger.Warn("Failed to close response body", slog.String("error", sanitizeErrorForLog(err))) //nolint:gosec // G706: error is sanitized via sanitizeErrorForLog
		}

		if statusCode >= 200 && statusCode < 300 {
			sd.failureLogger.Info("Slack notification sent successfully", slog.Int("status_code", statusCode), slog.String("run_id", req.runID)) //nolint:gosec // G706: run_id is an internal identifier, not user input
			return nil                                                                                                                           // Success
		}

		if statusCode == 429 || statusCode >= 500 {
			lastErr = fmt.Errorf("%w: %d", ErrServerError, statusCode)
			sd.failureLogger.Warn("Slack server error, retrying", slog.Int("status_code", statusCode), slog.Int("attempt", attempt+1), slog.String("run_id", req.runID)) //nolint:gosec // G706: run_id is an internal identifier, not user input
			continue                                                                                                                                                     // Retry for rate limiting and server errors
		}

		// Client error (4xx except 429) - don't retry
		sd.failureLogger.Error("Slack client error", slog.Int("status_code", statusCode), slog.String("run_id", req.runID)) //nolint:gosec // G706: run_id is an internal identifier, not user input
		return fmt.Errorf("%w: %d", ErrClientError, statusCode)
	}

	sd.failureLogger.Error("Failed to send Slack notification after all retries", slog.Int("attempts", len(backoffIntervals)+1), slog.String("last_error", sanitizeErrorForLog(lastErr)), slog.String("run_id", req.runID)) //nolint:gosec // G706: error is sanitized via sanitizeErrorForLog, run_id is an internal identifier
	return fmt.Errorf("failed to send to Slack after %d attempts: %w", len(backoffIntervals)+1, lastErr)
}

// newFailureLogger builds the logger that records send failures and drops. The
// handlers have already been verified Slack-free by the caller. With none
// given it writes to stderr only, rather than falling back to slog.Default(),
// which by then includes the Slack handlers themselves.
func newFailureLogger(handlers []slog.Handler) (*slog.Logger, error) {
	if len(handlers) == 0 {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), nil
	}

	multi, err := NewMultiHandler(handlers...)
	if err != nil {
		return nil, fmt.Errorf("failed to build failure logger: %w", err)
	}
	return slog.New(multi), nil
}
