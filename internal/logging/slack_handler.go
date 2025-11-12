package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
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

// SlackHandler is a slog.Handler that sends notifications to Slack
type SlackHandler struct {
	webhookURL    string
	runID         string
	httpClient    *http.Client
	level         slog.Level
	attrs         []slog.Attr // Accumulated attributes from WithAttrs calls
	groups        []string    // Accumulated group names from WithGroup calls
	backoffConfig BackoffConfig
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

// validateWebhookURL validates that the webhook URL is a valid HTTPS URL
func validateWebhookURL(webhookURL string) error {
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

	return nil
}

// NewSlackHandler creates a new SlackHandler with URL validation and default backoff configuration
func NewSlackHandler(webhookURL, runID string) (*SlackHandler, error) {
	return NewSlackHandlerWithConfig(webhookURL, runID, DefaultBackoffConfig)
}

// NewSlackHandlerWithConfig creates a new SlackHandler with URL validation and custom backoff configuration
func NewSlackHandlerWithConfig(webhookURL, runID string, config BackoffConfig) (*SlackHandler, error) {
	if err := validateWebhookURL(webhookURL); err != nil {
		return nil, fmt.Errorf("invalid webhook URL: %w", err)
	}

	slog.Debug("Creating Slack handler", "webhook_url", webhookURL, "run_id", runID, "timeout", httpTimeout, "backoff_base", config.Base, "retry_count", config.RetryCount)
	return &SlackHandler{
		webhookURL: webhookURL,
		runID:      runID,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
		level:         slog.LevelInfo, // Only handle info level and above
		backoffConfig: config,
	}, nil
}

// Enabled reports whether the handler handles records at the given level
func (s *SlackHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= s.level
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

	var message SlackMessage
	switch messageType {
	case "command_group_summary":
		message = s.buildCommandGroupSummary(r)
	case "pre_execution_error":
		message = s.buildPreExecutionError(r)
	case "security_alert":
		message = s.buildSecurityAlert(r)
	case "privileged_command_failure":
		message = s.buildPrivilegedCommandFailure(r)
	case "privilege_escalation_failure":
		message = s.buildPrivilegeEscalationFailure(r)
	default:
		// Generic message
		message = s.buildGenericMessage(r)
	}

	return s.sendToSlack(ctx, message)
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
		webhookURL:    s.webhookURL,
		runID:         s.runID,
		httpClient:    s.httpClient,
		level:         s.level,
		attrs:         newAttrs,
		groups:        s.groups, // Copy existing groups
		backoffConfig: s.backoffConfig,
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
		webhookURL:    s.webhookURL,
		runID:         s.runID,
		httpClient:    s.httpClient,
		level:         s.level,
		attrs:         s.attrs, // Copy existing attributes
		groups:        newGroups,
		backoffConfig: s.backoffConfig,
	}
}

// commandResultInfo holds command execution result information extracted from log attributes
// It embeds common.CommandResultFields to ensure type consistency with runner.CommandResult
type commandResultInfo struct {
	common.CommandResultFields
}

// extractCommandResults extracts command results from slog.Value containing []runner.CommandResult
// Each CommandResult implements slog.LogValuer, so we need to call LogValue() to get the group.
//
// IMPORTANT: Field keys must match common.LogField* constants defined in internal/common/logschema.go:
//   - common.LogFieldName     -> string (command name)
//   - common.LogFieldExitCode -> int (command exit code)
//   - common.LogFieldOutput   -> string (command stdout)
//   - common.LogFieldStderr   -> string (command stderr)
func extractCommandResults(value slog.Value) []commandResultInfo {
	var commands []commandResultInfo

	// The value is slog.KindAny containing a slice type
	if value.Kind() != slog.KindAny {
		return commands
	}

	anyVal := value.Any()

	// slog doesn't automatically resolve LogValuer interfaces in slices,
	// so we need to use reflection or type assertion to get each element
	// and manually call its LogValue() method if it implements LogValuer
	//
	// However, since we're in a handler, we receive the already-resolved values.
	// When slog processes a slice of LogValuer items, each item becomes a Group.
	// But that only happens if we iterate through the slice properly.
	//
	// For now, we work with the raw slice and resolve each element
	if slice, ok := anyVal.([]any); ok {
		for _, item := range slice {
			// Each item should be a []slog.Attr (the resolved Group from LogValue())
			if attrs, ok := item.([]slog.Attr); ok {
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
				commands = append(commands, cmdInfo)
			}
		}
	}

	return commands
}

// buildCommandGroupSummary builds a Slack message for command group summary
func (s *SlackHandler) buildCommandGroupSummary(r slog.Record) SlackMessage {
	var status, group string
	var duration time.Duration
	var commandsAttr slog.Attr
	var hasCommandsAttr bool

	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case "status":
			status = attr.Value.String()
		case "group":
			group = attr.Value.String()
		case "duration_ms":
			if attr.Value.Kind() == slog.KindInt64 {
				duration = time.Duration(attr.Value.Int64()) * time.Millisecond
			}
		case "commands":
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
		titleIcon = "[OK]"
	case "error":
		color = colorDanger
		titleIcon = "[FAIL]"
	default:
		color = colorWarning
		titleIcon = "[WARN]"
	}

	title := fmt.Sprintf("%s %s %s", titleIcon, strings.ToUpper(status), group)

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
			Title: "Run ID",
			Value: s.runID,
			Short: true,
		},
	}

	// Add individual command results
	for _, cmd := range commands {
		statusIcon := "[OK]"
		if cmd.ExitCode != 0 {
			statusIcon = "[FAIL]"
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
				Title: "  ↳ Output",
				Value: fmt.Sprintf("```%s```", output),
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
				Title: "  ↳ Error",
				Value: fmt.Sprintf("```%s```", stderr),
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
		case "error_type":
			errorType = attr.Value.String()
		case "error_message":
			errorMsg = attr.Value.String()
		case "component":
			component = attr.Value.String()
		}
		return true
	})

	hostname, _ := os.Hostname()

	message := SlackMessage{
		Text: fmt.Sprintf("[ERROR] %s", errorType),
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
						Title: "Hostname",
						Value: hostname,
						Short: true,
					},
					{
						Title: "Run ID",
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
		case "event_type":
			eventType = attr.Value.String()
		case "severity":
			severity = attr.Value.String()
		case "message":
			details = attr.Value.String()
		}
		return true
	})

	color := colorDanger
	switch severity {
	case "critical":
		color = colorDanger
	case "high":
		color = colorWarning
	}

	hostname, _ := os.Hostname()

	message := SlackMessage{
		Text: fmt.Sprintf("[SECURITY ALERT] %s", eventType),
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
						Title: "Hostname",
						Value: hostname,
						Short: true,
					},
					{
						Title: "Run ID",
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
		case "command_name":
			commandName = attr.Value.String()
		case "command_path":
			commandPath = attr.Value.String()
		case "stderr":
			stderr = attr.Value.String()
		case "exit_code":
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

	hostname, _ := os.Hostname()

	message := SlackMessage{
		Text: fmt.Sprintf("[FAIL] Privileged Command Failed: %s", commandName),
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
						Title: "Hostname",
						Value: hostname,
						Short: true,
					},
					{
						Title: "Error Output",
						Value: fmt.Sprintf("```%s```", stderr),
						Short: false,
					},
					{
						Title: "Run ID",
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
		case "operation":
			operation = attr.Value.String()
		case "command_name":
			commandName = attr.Value.String()
		case "original_uid":
			if attr.Value.Kind() == slog.KindInt64 {
				originalUID = int(attr.Value.Int64())
			}
		case "target_uid":
			if attr.Value.Kind() == slog.KindInt64 {
				targetUID = int(attr.Value.Int64())
			}
		}
		return true
	})

	hostname, _ := os.Hostname()

	message := SlackMessage{
		Text: fmt.Sprintf("[WARN] Privilege Escalation Failed: %s", operation),
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
						Title: "Hostname",
						Value: hostname,
						Short: true,
					},
					{
						Title: "Run ID",
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

// sendToSlack sends a message to Slack with retry logic
func (s *SlackHandler) sendToSlack(ctx context.Context, message SlackMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		slog.Error("Failed to marshal Slack message", "error", err, "run_id", s.runID)
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	slog.Debug("Sending Slack notification", "webhook_url", s.webhookURL, "run_id", s.runID, "message_text", message.Text)

	var lastErr error

	backoffIntervals := generateBackoffIntervals(s.backoffConfig.Base, s.backoffConfig.RetryCount)
	for attempt := 0; attempt <= s.backoffConfig.RetryCount; attempt++ {
		if attempt > 0 {
			// Get backoff interval from predefined list
			backoff := backoffIntervals[attempt-1]
			slog.Debug("Retrying Slack notification", "attempt", attempt+1, "backoff_seconds", backoff.Seconds(), "run_id", s.runID)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", s.webhookURL, bytes.NewBuffer(payload))
		if err != nil {
			lastErr = fmt.Errorf("failed to create request: %w", err)
			slog.Warn("Failed to create Slack request", "error", err, "attempt", attempt+1, "run_id", s.runID)
			continue
		}

		req.Header.Set("Content-Type", "application/json")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("failed to send request: %w", err)
			slog.Warn("Failed to send Slack request", "error", err, "attempt", attempt+1, "run_id", s.runID)
			continue
		}

		statusCode := resp.StatusCode
		if err := resp.Body.Close(); err != nil {
			slog.Warn("Failed to close response body", "error", err)
		}

		if statusCode >= 200 && statusCode < 300 {
			slog.Info("Slack notification sent successfully", "status_code", statusCode, "run_id", s.runID)
			return nil // Success
		}

		if statusCode == 429 || statusCode >= 500 {
			lastErr = fmt.Errorf("%w: %d", ErrServerError, statusCode)
			slog.Warn("Slack server error, retrying", "status_code", statusCode, "attempt", attempt+1, "run_id", s.runID)
			continue // Retry for rate limiting and server errors
		}

		// Client error (4xx except 429) - don't retry
		slog.Error("Slack client error", "status_code", statusCode, "run_id", s.runID)
		return fmt.Errorf("%w: %d", ErrClientError, statusCode)
	}

	slog.Error("Failed to send Slack notification after all retries", "attempts", len(backoffIntervals)+1, "last_error", lastErr, "run_id", s.runID)
	return fmt.Errorf("failed to send to Slack after %d attempts: %w", len(backoffIntervals)+1, lastErr)
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
