package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// HTTP status codes
	httpTimeout     = 5 * time.Second
	maxRetries      = 3
	outputMaxLength = 1000
	stderrMaxLength = 500

	// Color constants
	colorDanger  = "danger"
	colorWarning = "warning"
	colorGood    = "good"
)

// Static errors for linting compliance
var (
	ErrServerError = errors.New("server error")
	ErrClientError = errors.New("client error")
)

// SlackHandler is a slog.Handler that sends notifications to Slack
type SlackHandler struct {
	webhookURL string
	runID      string
	httpClient *http.Client
	level      slog.Level
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

// NewSlackHandler creates a new SlackHandler
func NewSlackHandler(webhookURL, runID string) *SlackHandler {
	slog.Debug("Creating Slack handler", "webhook_url", webhookURL, "run_id", runID, "timeout", httpTimeout)
	return &SlackHandler{
		webhookURL: webhookURL,
		runID:      runID,
		httpClient: &http.Client{
			Timeout: httpTimeout,
		},
		level: slog.LevelInfo, // Only handle info level and above
	}
}

// Enabled reports whether the handler handles records at the given level
func (s *SlackHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= s.level
}

// Handle processes the log record and sends it to Slack if appropriate
func (s *SlackHandler) Handle(ctx context.Context, r slog.Record) error {
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
func (s *SlackHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	// SlackHandler doesn't need to handle WithAttrs differently
	return s
}

// WithGroup returns a new SlackHandler with the given group name
func (s *SlackHandler) WithGroup(_ string) slog.Handler {
	// SlackHandler doesn't need to handle WithGroup differently
	return s
}

// buildCommandGroupSummary builds a Slack message for command group summary
func (s *SlackHandler) buildCommandGroupSummary(r slog.Record) SlackMessage {
	var status, group, command, output string
	var exitCode int
	var duration time.Duration

	r.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case "status":
			status = attr.Value.String()
		case "group":
			group = attr.Value.String()
		case "command":
			command = attr.Value.String()
		case "exit_code":
			if attr.Value.Kind() == slog.KindInt64 {
				exitCode = int(attr.Value.Int64())
			}
		case "duration_ms":
			if attr.Value.Kind() == slog.KindInt64 {
				duration = time.Duration(attr.Value.Int64()) * time.Millisecond
			}
		case "output":
			output = attr.Value.String()
		}
		return true
	})

	var color string
	var titleIcon string
	switch status {
	case "success":
		color = colorGood
		titleIcon = "✅"
	case "error":
		color = colorDanger
		titleIcon = "❌"
	default:
		color = colorWarning
		titleIcon = "⚠️"
	}

	// Truncate output if too long
	if len(output) > outputMaxLength {
		const truncationSuffix = "..."
		truncationPoint := outputMaxLength - len(truncationSuffix)
		output = output[:truncationPoint] + truncationSuffix
	}

	title := fmt.Sprintf("%s %s %s", titleIcon, strings.ToUpper(status), group)

	message := SlackMessage{
		Text: title,
		Attachments: []SlackAttachment{
			{
				Color: color,
				Fields: []SlackAttachmentField{
					{
						Title: "Command",
						Value: fmt.Sprintf("```%s```", command),
						Short: false,
					},
					{
						Title: "Exit Code",
						Value: fmt.Sprintf("%d", exitCode),
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
				},
			},
		},
	}

	if output != "" {
		message.Attachments[0].Fields = append(message.Attachments[0].Fields, SlackAttachmentField{
			Title: "Output",
			Value: fmt.Sprintf("```%s```", output),
			Short: false,
		})
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
		Text: fmt.Sprintf("🚨 Error: %s", errorType),
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
		Text: fmt.Sprintf("🚨 Security Alert: %s", eventType),
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
		stderr = stderr[:497] + "..."
	}

	hostname, _ := os.Hostname()

	message := SlackMessage{
		Text: fmt.Sprintf("❌ Privileged Command Failed: %s", commandName),
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
		Text: fmt.Sprintf("⚠️ Privilege Escalation Failed: %s", operation),
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

// sendToSlack sends a message to Slack with retry logic
func (s *SlackHandler) sendToSlack(ctx context.Context, message SlackMessage) error {
	payload, err := json.Marshal(message)
	if err != nil {
		slog.Error("Failed to marshal Slack message", "error", err, "run_id", s.runID)
		return fmt.Errorf("failed to marshal Slack message: %w", err)
	}

	slog.Debug("Sending Slack notification", "webhook_url", s.webhookURL, "run_id", s.runID, "message_text", message.Text)

	// Retry logic: maxRetries attempts with exponential backoff
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 2s, 4s, 8s
			const backoffBase = 2
			backoff := time.Duration(backoffBase<<(attempt-1)) * time.Second
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

	slog.Error("Failed to send Slack notification after all retries", "attempts", maxRetries, "last_error", lastErr, "run_id", s.runID)
	return fmt.Errorf("failed to send to Slack after %d attempts: %w", maxRetries, lastErr)
}
