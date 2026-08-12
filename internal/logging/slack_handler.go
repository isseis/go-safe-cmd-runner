package logging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

const (
	// HTTP status codes
	httpTimeout      = 5 * time.Second
	outputMaxLength  = 1000
	stderrMaxLength  = 500
	truncationSuffix = "..."

	// Backoff configuration constants
	defaultBackoffBase = 2 * time.Second
	defaultRetryCount  = 3

	// Color constants
	colorDanger  = "danger"
	colorWarning = "warning"
	colorGood    = "good"

	// Emoji icon constants
	emojiSuccess = "✅"
	emojiFailure = "❌"
	emojiWarning = "⚠️"
	emojiAlert   = "🚨"

	// Special character constants
	arrowIndent = "  ↳"

	// Slack attachment field titles reused across message builders
	fieldTitleHostname = "Hostname"
	fieldTitleRunID    = "Run ID"
)

// BackoffConfig defines the retry backoff configuration
type BackoffConfig struct {
	Base       time.Duration // Base interval for exponential backoff
	RetryCount int           // Number of retry attempts
}

// DefaultBackoffConfig is the production backoff configuration
var DefaultBackoffConfig = BackoffConfig{
	Base:       defaultBackoffBase,
	RetryCount: defaultRetryCount,
}

// Static errors for linting compliance
var (
	ErrServerError       = errors.New("server error")
	ErrClientError       = errors.New("client error")
	ErrInvalidWebhookURL = errors.New("invalid webhook URL")
)

// SlackHandlerLevelMode defines how the handler filters log levels
type SlackHandlerLevelMode int

const (
	// LevelModeDefault handles all levels >= configured level (existing behavior)
	LevelModeDefault SlackHandlerLevelMode = iota

	// LevelModeExactInfo handles only INFO level (for success webhook)
	LevelModeExactInfo

	// LevelModeWarnAndAbove handles only WARN and above (for error webhook)
	LevelModeWarnAndAbove
)

// SlackHandler is a slog.Handler that sends notifications to Slack
type SlackHandler struct {
	runID     string
	level     slog.Level
	attrs     []slog.Attr           // Accumulated attributes from WithAttrs calls
	groups    []string              // Accumulated group names from WithGroup calls
	isDryRun  bool                  // Whether running in dry-run mode (suppresses actual notifications)
	levelMode SlackHandlerLevelMode // Level filtering mode
	// sender owns the delivery machinery. Handlers derived via WithAttrs /
	// WithGroup share it by pointer, so one webhook configuration has one
	// worker no matter how many derived handlers exist. It is nil for a
	// handler built as a struct literal and in dry-run mode; see Handle.
	sender *slackSender
}

// SlackMessage represents the structure of a Slack webhook message
type SlackMessage struct {
	Text        string            `json:"text"`
	Blocks      []SlackBlock      `json:"blocks,omitempty"`
	Attachments []SlackAttachment `json:"attachments,omitempty"`
}

// SlackBlock represents a Slack block
type SlackBlock struct {
	Type string          `json:"type"`
	Text *SlackTextBlock `json:"text,omitempty"`
}

// SlackTextBlock represents text within a Slack block
type SlackTextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SlackAttachment represents a Slack attachment
type SlackAttachment struct {
	Color  string                 `json:"color,omitempty"`
	Fields []SlackAttachmentField `json:"fields,omitempty"`
}

// SlackAttachmentField represents a field within a Slack attachment
type SlackAttachmentField struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// SlackHandlerOptions holds configuration for creating a SlackHandler
type SlackHandlerOptions struct {
	WebhookURL    string                // Slack webhook URL (required)
	RunID         string                // Run ID for tracking (required)
	HTTPClient    *http.Client          // Custom HTTP client (optional, defaults to client with httpTimeout)
	BackoffConfig BackoffConfig         // Retry backoff configuration (optional, defaults to DefaultBackoffConfig)
	IsDryRun      bool                  // If true, suppresses actual Slack notifications (used for dry-run mode)
	LevelMode     SlackHandlerLevelMode // Level filtering mode (optional, defaults to LevelModeDefault)

	// AllowedHost is the permitted hostname used to validate the webhook URL.
	// An empty string causes all URLs to be rejected.
	AllowedHost string

	// FailureHandlers are the leaf handlers that receive send failures and
	// drops. NewSlackHandler builds the failure logger over them itself, so the
	// failure path cannot be an arbitrary *slog.Logger whose chain would have to
	// be inferred by a scan. Every element must be verifiably Slack-free:
	// a stdlib leaf handler, one of this package's leaf handlers, a MultiHandler
	// over such handlers, or a handler that opts in via SlackFreeHandler.
	// NewSlackHandler rejects anything else. When empty, a stderr-only logger is
	// used.
	FailureHandlers []slog.Handler
	// SendTimeout bounds one notification's delivery including retries
	// (optional, defaults to DefaultSendTimeout).
	SendTimeout time.Duration
	// HighPriorityQueueSize and NormalQueueSize override the two send queues'
	// capacities. They exist as test seams for exercising overflow behaviour
	// without enqueueing the full production capacity; production code leaves
	// both zero and gets defaultHighPriorityQueueSize / defaultNormalQueueSize.
	HighPriorityQueueSize int
	NormalQueueSize       int
	// Synchronous disables the worker and sends inline. It is a debugging
	// escape hatch selected by GSCR_SLACK_SYNC, not a supported mode.
	Synchronous bool
}

// validateWebhookURL validates that the webhook URL is a valid HTTPS URL with allowed host.
// allowedHost must be a pure hostname without port (pre-validated by normalizeSlackAllowedHost).
func validateWebhookURL(webhookURL string, allowedHost string) error {
	if webhookURL == "" {
		return fmt.Errorf("%w: empty URL", ErrInvalidWebhookURL)
	}

	parsedURL, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("%w: failed to parse URL: %v", ErrInvalidWebhookURL, err)
	}

	if parsedURL.Scheme != "https" {
		return fmt.Errorf("%w: URL must use HTTPS scheme, got: %s", ErrInvalidWebhookURL, parsedURL.Scheme)
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("%w: URL must have a host", ErrInvalidWebhookURL)
	}

	if allowedHost == "" {
		return fmt.Errorf("%w: allowed host is not configured", ErrInvalidWebhookURL)
	}

	hostname := strings.ToLower(parsedURL.Hostname())
	normalizedAllowedHost := strings.ToLower(allowedHost)
	if hostname != normalizedAllowedHost {
		return fmt.Errorf("%w: host not allowed: %s (allowed: %s)", ErrInvalidWebhookURL, hostname, normalizedAllowedHost)
	}

	return nil
}

// NewSlackHandler creates a new SlackHandler with the provided options
// This is the preferred way to create a SlackHandler as it allows for easy addition of new configuration options
func NewSlackHandler(opts SlackHandlerOptions) (*SlackHandler, error) {
	if err := validateWebhookURL(opts.WebhookURL, opts.AllowedHost); err != nil {
		return nil, fmt.Errorf("invalid webhook URL: %w", err)
	}

	// Apply defaults for optional fields
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: httpTimeout}
	}

	backoffConfig := opts.BackoffConfig
	// If backoff config is zero-valued, use defaults
	if backoffConfig.Base == 0 && backoffConfig.RetryCount == 0 {
		backoffConfig = DefaultBackoffConfig
	}

	if err := verifySlackFreeHandlers(opts.FailureHandlers); err != nil {
		return nil, fmt.Errorf("invalid failure handlers: %w", err)
	}

	failureLogger, err := newFailureLogger(opts.FailureHandlers)
	if err != nil {
		return nil, err
	}

	slog.Debug("Creating Slack handler",
		slog.Bool("webhook_configured", opts.WebhookURL != ""),
		slog.String("run_id", opts.RunID),
		slog.Duration("timeout", httpClient.Timeout),
		slog.Duration("backoff_base", backoffConfig.Base),
		slog.Int("retry_count", backoffConfig.RetryCount),
		slog.Bool("dry_run", opts.IsDryRun),
		slog.Int("level_mode", int(opts.LevelMode)))

	handler := &SlackHandler{
		runID:     opts.RunID,
		level:     slog.LevelInfo, // Only handle info level and above
		isDryRun:  opts.IsDryRun,
		levelMode: opts.LevelMode,
	}

	// Dry-run is defined as having no external side effects, so it gets no
	// sender at all: no queues, no worker, nothing to flush.
	if !opts.IsDryRun {
		handler.sender = newSlackSender(opts, httpClient, backoffConfig, failureLogger)
	}

	return handler, nil
}

// Flush stops accepting new notifications, drains what is already queued under
// ctx, and waits for the worker to terminate. A send already in flight is
// re-bounded to the drain budget rather than cancelled, so it still counts as
// Sent if it completes. Notifications still undelivered when Flush returns are
// reported as Pending. It is safe to call concurrently and
// repeatedly; calls after the first return the same accounting without
// re-draining. A handler with no sender (dry-run, or built as a struct
// literal) returns the zero value.
func (s *SlackHandler) Flush(ctx context.Context) FlushStats {
	if s.sender == nil {
		return FlushStats{}
	}
	return s.sender.flush(ctx)
}

// Close stops accepting notifications and terminates the worker without
// draining. It exists for teardown paths that have no notifications worth
// delivering, such as AddSlackHandlers unwinding after a partial failure. When
// Flush already requested a drain, Close does not override it; it waits for the
// worker and returns the same accounting.
func (s *SlackHandler) Close() FlushStats {
	if s.sender == nil {
		return FlushStats{}
	}
	return s.sender.close()
}

// Enabled reports whether the handler handles records at the given level
func (s *SlackHandler) Enabled(_ context.Context, level slog.Level) bool {
	switch s.levelMode {
	case LevelModeExactInfo:
		return level == slog.LevelInfo
	case LevelModeWarnAndAbove:
		return level >= slog.LevelWarn
	default:
		return level >= s.level
	}
}

// Handle processes the log record and sends it to Slack if appropriate
func (s *SlackHandler) Handle(ctx context.Context, r slog.Record) error {
	// Apply accumulated attributes and groups to the record
	r = s.applyAccumulatedContext(r)

	// Only send specific types of messages to Slack
	var shouldSend bool
	var messageType string

	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case "slack_notify":
			if attr.Value.Kind() == slog.KindBool && attr.Value.Bool() {
				shouldSend = true
			}
		case "message_type":
			if attr.Value.Kind() == slog.KindString {
				messageType = attr.Value.String()
			}
		}
		return true
	})

	if !shouldSend {
		return nil
	}

	// No sender means no delivery: dry-run mode, or a handler built as a struct
	// literal. Building the message would be wasted work, and panicking inside
	// a log path is the worst possible failure mode, so this returns quietly.
	if s.sender == nil {
		if s.isDryRun {
			slog.Debug("Skipping Slack notification in dry-run mode", slog.String("message_type", messageType))
		}
		return nil
	}

	// A closed sender drops every request it receives (see enqueue/sendSync),
	// so building the message here -- e.g. buildCommandGroupSummary iterating
	// every command result -- would only be discarded work on the shutdown
	// path where the process wants to exit quickly.
	if s.sender.isClosed() {
		req := slackRequest{messageType: messageType, runID: s.runID, level: r.Level}
		if s.sender.synchronous {
			return s.sender.sendSync(ctx, req)
		}
		s.sender.enqueue(req)
		return nil
	}

	var message SlackMessage
	switch messageType {
	case messageTypeCommandGroupSummary:
		message = s.buildCommandGroupSummary(r)
	case messageTypePreExecutionError:
		message = s.buildPreExecutionError(r)
	case messageTypeSecurityAlert:
		message = s.buildSecurityAlert(r)
	case messageTypePrivilegedCommandFailure:
		message = s.buildPrivilegedCommandFailure(r)
	case messageTypePrivilegeEscalationFail:
		message = s.buildPrivilegeEscalationFailure(r)
	default:
		// Generic message
		message = s.buildGenericMessage(r)
	}

	req := slackRequest{
		message:     &message,
		messageType: messageType,
		runID:       s.runID,
		level:       r.Level,
	}

	if s.sender.synchronous {
		return s.sender.sendSync(ctx, req)
	}

	s.sender.enqueue(req)
	return nil
}

// WithAttrs returns a new SlackHandler with the given attributes
func (s *SlackHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return s
	}

	// Create a new SlackHandler with accumulated attributes
	newAttrs := make([]slog.Attr, len(s.attrs)+len(attrs))
	copy(newAttrs, s.attrs)
	copy(newAttrs[len(s.attrs):], attrs)

	return &SlackHandler{
		runID:     s.runID,
		level:     s.level,
		attrs:     newAttrs,
		groups:    s.groups, // Copy existing groups
		isDryRun:  s.isDryRun,
		levelMode: s.levelMode, // Preserve levelMode
		sender:    s.sender,    // Shared by pointer: one worker per webhook
	}
}

// WithGroup returns a new SlackHandler with the given group name
func (s *SlackHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return s
	}

	// Create a new SlackHandler with accumulated group names
	newGroups := make([]string, len(s.groups)+1)
	copy(newGroups, s.groups)
	newGroups[len(s.groups)] = name

	return &SlackHandler{
		runID:     s.runID,
		level:     s.level,
		attrs:     s.attrs, // Copy existing attributes
		groups:    newGroups,
		isDryRun:  s.isDryRun,
		levelMode: s.levelMode, // Preserve levelMode
		sender:    s.sender,    // Shared by pointer: one worker per webhook
	}
}

// commandResultInfo holds command execution result information extracted from log attributes
// It embeds common.CommandResultFields to ensure type consistency with runner.CommandResult
type commandResultInfo struct {
	common.CommandResultFields
}

// extractCommandResultsFromGroup extracts command result information from a slog.Group value.
//
// The input groupValue is expected to have the structure generated by CommandResults.LogValue(),
// containing metadata (total_count / truncated) and Group attributes in cmd_N format.
// Returns nil for invalid formats and logs details to debug log.
func extractCommandResultsFromGroup(groupValue slog.Value) []commandResultInfo {
	if groupValue.Kind() != slog.KindGroup {
		slog.Debug(
			"Command results extraction failed: unexpected value kind",
			"expected", slog.KindGroup,
			"actual", groupValue.Kind(),
			"function", "extractCommandResultsFromGroup",
		)
		return nil
	}

	attrs := groupValue.Group()
	if len(attrs) == 0 {
		slog.Debug(
			"Command results extraction: empty group",
			"function", "extractCommandResultsFromGroup",
		)
		return nil
	}

	estimatedCmdCount := max(len(attrs)-common.CommandResultsMetadataAttrCount, 0)

	commands := make([]commandResultInfo, 0, estimatedCmdCount)
	skipped := 0

	for i, attr := range attrs {
		if attr.Key == "total_count" || attr.Key == "truncated" {
			continue
		}

		if attr.Value.Kind() != slog.KindGroup {
			slog.Debug(
				"Skipping non-group attribute in command results",
				"index", i,
				"key", attr.Key,
				"kind", attr.Value.Kind(),
				"function", "extractCommandResultsFromGroup",
			)
			skipped++
			continue
		}

		cmdAttrs := attr.Value.Group()
		cmdInfo := extractFromAttrs(cmdAttrs)

		if cmdInfo.Name == "" {
			slog.Debug(
				"Skipping command result with missing name",
				"index", i,
				"key", attr.Key,
				"function", "extractCommandResultsFromGroup",
			)
			skipped++
			continue
		}

		commands = append(commands, cmdInfo)
	}

	if skipped > 0 {
		slog.Debug(
			"Command results extraction completed with some skipped items",
			"extracted", len(commands),
			"skipped", skipped,
			"total_attrs", len(attrs),
			"function", "extractCommandResultsFromGroup",
		)
	}

	return commands
}

// extractCommandResults extracts command result information from log values.
//
// Only supports the Group structure generated by CommandResults.LogValue(),
// and does not support legacy formats []CommandResult / []any
func extractCommandResults(value slog.Value) []commandResultInfo {
	return extractCommandResultsFromGroup(value.Resolve())
}

// extractFromAttrs extracts commandResultInfo from a slice of slog.Attr
func extractFromAttrs(attrs []slog.Attr) commandResultInfo {
	cmdInfo := commandResultInfo{}
	for _, attr := range attrs {
		switch attr.Key {
		case common.LogFieldName:
			cmdInfo.Name = attr.Value.String()
		case common.LogFieldExitCode:
			if attr.Value.Kind() == slog.KindInt64 {
				cmdInfo.ExitCode = int(attr.Value.Int64())
			}
		case common.LogFieldOutput:
			cmdInfo.Output = attr.Value.String()
		case common.LogFieldStderr:
			cmdInfo.Stderr = attr.Value.String()
		}
	}
	return cmdInfo
}

// buildCommandGroupSummary builds a Slack message for command group summary
func (s *SlackHandler) buildCommandGroupSummary(r slog.Record) SlackMessage {
	var status, group string
	var duration time.Duration
	var commandsAttr slog.Attr
	var hasCommandsAttr bool

	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case common.GroupSummaryAttrs.Status:
			status = attr.Value.String()
		case common.GroupSummaryAttrs.Group:
			group = attr.Value.String()
		case common.GroupSummaryAttrs.DurationMs:
			if attr.Value.Kind() == slog.KindInt64 {
				duration = time.Duration(attr.Value.Int64()) * time.Millisecond
			}
		case common.GroupSummaryAttrs.Commands:
			commandsAttr = attr
			hasCommandsAttr = true
		}
		return true
	})

	// Extract command results from the commands attribute
	var commands []commandResultInfo
	if hasCommandsAttr {
		// Extract commands from the slog.Value
		commands = extractCommandResults(commandsAttr.Value)
	}

	var color string
	var titleIcon string
	switch status {
	case "success":
		color = colorGood
		titleIcon = emojiSuccess
	case "error":
		color = colorDanger
		titleIcon = emojiFailure
	default:
		color = colorWarning
		titleIcon = emojiWarning
	}

	title := fmt.Sprintf("### %s %s %s", titleIcon, strings.ToUpper(status), group)

	hostname := common.GetHostname()

	// Build fields for the attachment
	fields := []SlackAttachmentField{
		{
			Title: "Command Count",
			Value: fmt.Sprintf("%d", len(commands)),
			Short: true,
		},
		{
			Title: "Duration",
			Value: duration.String(),
			Short: true,
		},
		{
			Title: fieldTitleHostname,
			Value: hostname,
			Short: true,
		},
		{
			Title: fieldTitleRunID,
			Value: s.runID,
			Short: true,
		},
	}

	// Add individual command results
	for _, cmd := range commands {
		statusIcon := emojiSuccess
		if cmd.ExitCode != 0 {
			statusIcon = emojiFailure
		}

		// Build command summary
		cmdSummary := fmt.Sprintf("%s `%s` (exit: %d)", statusIcon, cmd.Name, cmd.ExitCode)

		fields = append(fields, SlackAttachmentField{
			Title: "Command",
			Value: cmdSummary,
			Short: false,
		})

		// Add output if present and not too long
		if cmd.Output != "" {
			output := cmd.Output
			if len(output) > outputMaxLength {
				truncationPoint := outputMaxLength - len(truncationSuffix)
				output = output[:truncationPoint] + truncationSuffix
			}
			fields = append(fields, SlackAttachmentField{
				Title: arrowIndent + " Output",
				Value: fmt.Sprintf("```\n%s\n```", output),
				Short: false,
			})
		}

		// Add stderr if present and command failed
		if cmd.Stderr != "" && cmd.ExitCode != 0 {
			stderr := cmd.Stderr
			if len(stderr) > stderrMaxLength {
				truncationPoint := stderrMaxLength - len(truncationSuffix)
				stderr = stderr[:truncationPoint] + truncationSuffix
			}
			fields = append(fields, SlackAttachmentField{
				Title: arrowIndent + " Error",
				Value: fmt.Sprintf("```\n%s\n```", stderr),
				Short: false,
			})
		}
	}

	message := SlackMessage{
		Text: title,
		Attachments: []SlackAttachment{
			{
				Color:  color,
				Fields: fields,
			},
		},
	}

	return message
}

// buildPreExecutionError builds a Slack message for pre-execution errors
func (s *SlackHandler) buildPreExecutionError(r slog.Record) SlackMessage {
	var errorType, errorMsg, component string

	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case common.PreExecErrorAttrs.ErrorType:
			errorType = attr.Value.String()
		case common.PreExecErrorAttrs.ErrorMessage:
			errorMsg = attr.Value.String()
		case common.PreExecErrorAttrs.Component:
			component = attr.Value.String()
		}
		return true
	})

	hostname := common.GetHostname()

	message := SlackMessage{
		Text: fmt.Sprintf("%s Error: %s", emojiAlert, errorType),
		Attachments: []SlackAttachment{
			{
				Color: colorDanger,
				Fields: []SlackAttachmentField{
					{
						Title: "Error Message",
						Value: errorMsg,
						Short: false,
					},
					{
						Title: "Component",
						Value: component,
						Short: true,
					},
					{
						Title: fieldTitleHostname,
						Value: hostname,
						Short: true,
					},
					{
						Title: fieldTitleRunID,
						Value: s.runID,
						Short: true,
					},
				},
			},
		},
	}

	return message
}

// buildSecurityAlert builds a Slack message for security alerts
func (s *SlackHandler) buildSecurityAlert(r slog.Record) SlackMessage {
	var eventType, severity, details string

	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case common.SecurityAlertAttrs.EventType:
			eventType = attr.Value.String()
		case common.SecurityAlertAttrs.Severity:
			severity = attr.Value.String()
		case common.SecurityAlertAttrs.Message:
			details = attr.Value.String()
		}
		return true
	})

	color := colorDanger
	switch severity {
	case common.SeverityCritical:
		color = colorDanger
	case common.SeverityHigh:
		color = colorWarning
	}

	hostname := common.GetHostname()

	message := SlackMessage{
		Text: fmt.Sprintf("%s Security Alert: %s", emojiAlert, eventType),
		Attachments: []SlackAttachment{
			{
				Color: color,
				Fields: []SlackAttachmentField{
					{
						Title: "Severity",
						Value: strings.ToUpper(severity),
						Short: true,
					},
					{
						Title: "Event Type",
						Value: eventType,
						Short: true,
					},
					{
						Title: "Details",
						Value: details,
						Short: false,
					},
					{
						Title: fieldTitleHostname,
						Value: hostname,
						Short: true,
					},
					{
						Title: fieldTitleRunID,
						Value: s.runID,
						Short: true,
					},
				},
			},
		},
	}

	return message
}

// buildPrivilegedCommandFailure builds a Slack message for privileged command failures
func (s *SlackHandler) buildPrivilegedCommandFailure(r slog.Record) SlackMessage {
	var commandName, commandPath, stderr string
	var exitCode int

	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case common.PrivilegedCommandFailureAttrs.CommandName:
			commandName = attr.Value.String()
		case common.PrivilegedCommandFailureAttrs.CommandPath:
			commandPath = attr.Value.String()
		case common.PrivilegedCommandFailureAttrs.Stderr:
			stderr = attr.Value.String()
		case common.PrivilegedCommandFailureAttrs.ExitCode:
			if attr.Value.Kind() == slog.KindInt64 {
				exitCode = int(attr.Value.Int64())
			}
		}
		return true
	})

	// Truncate stderr if too long
	if len(stderr) > stderrMaxLength {
		const truncationSuffix = "..."
		truncationPoint := stderrMaxLength - len(truncationSuffix)
		stderr = stderr[:truncationPoint] + truncationSuffix
	}

	hostname := common.GetHostname()

	message := SlackMessage{
		Text: fmt.Sprintf("%s Privileged Command Failed: %s", emojiFailure, commandName),
		Attachments: []SlackAttachment{
			{
				Color: colorDanger,
				Fields: []SlackAttachmentField{
					{
						Title: "Command",
						Value: fmt.Sprintf("`%s`", commandPath),
						Short: false,
					},
					{
						Title: "Exit Code",
						Value: fmt.Sprintf("%d", exitCode),
						Short: true,
					},
					{
						Title: fieldTitleHostname,
						Value: hostname,
						Short: true,
					},
					{
						Title: "Error Output",
						Value: fmt.Sprintf("```\n%s\n```", stderr),
						Short: false,
					},
					{
						Title: fieldTitleRunID,
						Value: s.runID,
						Short: true,
					},
				},
			},
		},
	}

	return message
}

// buildPrivilegeEscalationFailure builds a Slack message for privilege escalation failures
func (s *SlackHandler) buildPrivilegeEscalationFailure(r slog.Record) SlackMessage {
	var operation, commandName string
	var originalUID, targetUID int

	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case common.PrivilegeEscalationFailureAttrs.Operation:
			operation = attr.Value.String()
		case common.PrivilegeEscalationFailureAttrs.CommandName:
			commandName = attr.Value.String()
		case common.PrivilegeEscalationFailureAttrs.OriginalUID:
			if attr.Value.Kind() == slog.KindInt64 {
				originalUID = int(attr.Value.Int64())
			}
		case common.PrivilegeEscalationFailureAttrs.TargetUID:
			if attr.Value.Kind() == slog.KindInt64 {
				targetUID = int(attr.Value.Int64())
			}
		}
		return true
	})

	hostname := common.GetHostname()

	message := SlackMessage{
		Text: fmt.Sprintf("%s Privilege Escalation Failed: %s", emojiWarning, operation),
		Attachments: []SlackAttachment{
			{
				Color: colorWarning,
				Fields: []SlackAttachmentField{
					{
						Title: "Operation",
						Value: operation,
						Short: true,
					},
					{
						Title: "Command",
						Value: commandName,
						Short: true,
					},
					{
						Title: "From UID",
						Value: fmt.Sprintf("%d", originalUID),
						Short: true,
					},
					{
						Title: "To UID",
						Value: fmt.Sprintf("%d", targetUID),
						Short: true,
					},
					{
						Title: fieldTitleHostname,
						Value: hostname,
						Short: true,
					},
					{
						Title: fieldTitleRunID,
						Value: s.runID,
						Short: true,
					},
				},
			},
		},
	}

	return message
}

// buildGenericMessage builds a generic Slack message
func (s *SlackHandler) buildGenericMessage(r slog.Record) SlackMessage {
	return SlackMessage{
		Text: fmt.Sprintf("%s: %s (Run ID: %s)", r.Level.String(), r.Message, s.runID),
	}
}

// generateBackoffIntervals creates exponential backoff intervals
// For backoffBase=2s, count=3: returns [2s, 4s, 8s]
// Formula: [base*2^0, base*2^1, base*2^2, ...]
func generateBackoffIntervals(base time.Duration, count int) []time.Duration {
	intervals := make([]time.Duration, count)
	for i := range count {
		// Exponential backoff: base * 2^i
		intervals[i] = base * time.Duration(1<<i)
	}
	return intervals
}

// sanitizeErrorForLog extracts a safe error string for logging,
// stripping webhook URLs from *url.Error values.
//
// TODO(0154-import-cycle): Apply RedactText to non-URL errors as a
// defense-in-depth fallback. This requires resolving the import cycle
// between internal/redaction and internal/logging (redactor_test.go
// imports logging for NewMultiHandler tests).
func sanitizeErrorForLog(err error) string {
	if err == nil {
		return ""
	}
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		if urlErr.Err != nil {
			return urlErr.Err.Error()
		}
		return "url error: " + urlErr.Op + " without URL"
	}
	return err.Error()
}

// applyAccumulatedContext applies accumulated attributes and groups to the record
func (s *SlackHandler) applyAccumulatedContext(r slog.Record) slog.Record {
	if len(s.attrs) == 0 && len(s.groups) == 0 {
		return r // No accumulated context to apply
	}

	// Create a new record with the same basic properties
	newRecord := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)

	// Apply groups by creating nested attribute groups
	currentAttrs := s.attrs
	for i := len(s.groups) - 1; i >= 0; i-- {
		groupName := s.groups[i]
		if groupName != "" {
			// Convert []slog.Attr to []any for slog.Group
			groupArgs := make([]any, len(currentAttrs))
			for j, attr := range currentAttrs {
				groupArgs[j] = attr
			}
			// Wrap current attributes in a group
			currentAttrs = []slog.Attr{slog.Group(groupName, groupArgs...)}
		}
	}

	// Add accumulated attributes (possibly grouped) to the new record
	for _, attr := range currentAttrs {
		newRecord.AddAttrs(attr)
	}

	// Add original record's attributes
	r.Attrs(func(attr slog.Attr) bool {
		newRecord.AddAttrs(attr)
		return true
	})

	return newRecord
}
