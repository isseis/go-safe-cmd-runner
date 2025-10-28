package logging

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSlackHandler_WithAttrs(t *testing.T) {
	// Create a SlackHandler (we don't need a real webhook URL for this test)
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
		attrs:      nil,
		groups:     nil,
	}

	// Test WithAttrs
	attrs := []slog.Attr{
		slog.String("key1", "value1"),
		slog.String("key2", "value2"),
	}

	newHandler := handler.WithAttrs(attrs).(*SlackHandler)

	// Verify the new handler has the attributes
	if len(newHandler.attrs) != 2 {
		t.Errorf("Expected 2 attributes, got %d", len(newHandler.attrs))
	}

	// Verify the original handler is unchanged
	if len(handler.attrs) != 0 {
		t.Errorf("Original handler should not be modified")
	}

	// Test chaining WithAttrs
	moreAttrs := []slog.Attr{
		slog.String("key3", "value3"),
	}

	chainedHandler := newHandler.WithAttrs(moreAttrs).(*SlackHandler)

	if len(chainedHandler.attrs) != 3 {
		t.Errorf("Expected 3 attributes after chaining, got %d", len(chainedHandler.attrs))
	}
}

func TestSlackHandler_WithGroup(t *testing.T) {
	// Create a SlackHandler
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
		attrs:      nil,
		groups:     nil,
	}

	// Test WithGroup
	newHandler := handler.WithGroup("group1").(*SlackHandler)

	// Verify the new handler has the group
	if len(newHandler.groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(newHandler.groups))
	}

	if newHandler.groups[0] != "group1" {
		t.Errorf("Expected group name 'group1', got '%s'", newHandler.groups[0])
	}

	// Verify the original handler is unchanged
	if len(handler.groups) != 0 {
		t.Errorf("Original handler should not be modified")
	}

	// Test chaining WithGroup
	chainedHandler := newHandler.WithGroup("group2").(*SlackHandler)

	if len(chainedHandler.groups) != 2 {
		t.Errorf("Expected 2 groups after chaining, got %d", len(chainedHandler.groups))
	}

	if chainedHandler.groups[1] != "group2" {
		t.Errorf("Expected second group name 'group2', got '%s'", chainedHandler.groups[1])
	}
}

func TestSlackHandler_WithAttrsAndGroups(t *testing.T) {
	// Create a SlackHandler
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
		attrs:      nil,
		groups:     nil,
	}

	// Test combining WithAttrs and WithGroup
	attrs := []slog.Attr{
		slog.String("key1", "value1"),
	}

	combined := handler.WithAttrs(attrs).WithGroup("testgroup").(*SlackHandler)

	if len(combined.attrs) != 1 {
		t.Errorf("Expected 1 attribute, got %d", len(combined.attrs))
	}

	if len(combined.groups) != 1 {
		t.Errorf("Expected 1 group, got %d", len(combined.groups))
	}

	if combined.groups[0] != "testgroup" {
		t.Errorf("Expected group name 'testgroup', got '%s'", combined.groups[0])
	}
}

func TestSlackHandler_ApplyAccumulatedContext(t *testing.T) {
	// Create a SlackHandler with some accumulated context
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
		attrs: []slog.Attr{
			slog.String("accumulated_key", "accumulated_value"),
		},
		groups: []string{"testgroup"},
	}

	// Create a test record
	originalRecord := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	originalRecord.AddAttrs(slog.String("original_key", "original_value"))

	// Apply accumulated context
	newRecord := handler.applyAccumulatedContext(originalRecord)

	// Verify the new record has both accumulated and original attributes
	attrCount := 0
	var hasAccumulated, hasOriginal, hasGroup bool

	newRecord.Attrs(func(attr slog.Attr) bool {
		attrCount++
		switch attr.Key {
		case "original_key":
			hasOriginal = true
		case "testgroup":
			hasGroup = true
			// Check if the group contains the accumulated attribute
			if attr.Value.Kind() == slog.KindGroup {
				groupAttrs := attr.Value.Group()
				for _, groupAttr := range groupAttrs {
					if groupAttr.Key == "accumulated_key" {
						hasAccumulated = true
					}
				}
			}
		}
		return true
	})

	if !hasOriginal {
		t.Error("Expected original attribute to be present")
	}

	if !hasGroup {
		t.Error("Expected group to be present")
	}

	if !hasAccumulated {
		t.Error("Expected accumulated attribute to be present in group")
	}
}

func TestSlackHandler_WithAttrsEmptySlice(t *testing.T) {
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
	}

	// WithAttrs with empty slice should return the same handler
	newHandler := handler.WithAttrs([]slog.Attr{})

	if newHandler != handler {
		t.Error("WithAttrs with empty slice should return the same handler")
	}
}

func TestSlackHandler_WithGroupEmptyString(t *testing.T) {
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
	}

	// WithGroup with empty string should return the same handler
	newHandler := handler.WithGroup("")

	if newHandler != handler {
		t.Error("WithGroup with empty string should return the same handler")
	}
}

func TestValidateWebhookURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		expectError bool
		errorType   error
	}{
		{
			name:        "valid HTTPS URL",
			url:         "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
			expectError: false,
		},
		{
			name:        "empty URL",
			url:         "",
			expectError: true,
			errorType:   ErrInvalidWebhookURL,
		},
		{
			name:        "HTTP URL (should be rejected)",
			url:         "http://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
			expectError: true,
			errorType:   ErrInvalidWebhookURL,
		},
		{
			name:        "URL without scheme",
			url:         "hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
			expectError: true,
			errorType:   ErrInvalidWebhookURL,
		},
		{
			name:        "URL without host",
			url:         "https:///services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
			expectError: true,
			errorType:   ErrInvalidWebhookURL,
		},
		{
			name:        "malformed URL with invalid characters",
			url:         "https://hooks.slack.com/services/T00000000/B00000000/XXXX\x00XX",
			expectError: true,
			errorType:   ErrInvalidWebhookURL,
		},
		{
			name:        "URL with only protocol",
			url:         "https://",
			expectError: true,
			errorType:   ErrInvalidWebhookURL,
		},
		{
			name:        "FTP protocol (should be rejected)",
			url:         "ftp://example.com/webhook",
			expectError: true,
			errorType:   ErrInvalidWebhookURL,
		},
		{
			name:        "localhost HTTPS URL (valid for testing)",
			url:         "https://localhost:8080/webhook",
			expectError: false,
		},
		{
			name:        "URL with special characters in path (valid)",
			url:         "https://hooks.slack.com/services/T00000000/B00000000/XXXX%20XX",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebhookURL(tt.url)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for URL: %s", tt.url)
					return
				}

				if tt.errorType != nil && !errors.Is(err, tt.errorType) {
					t.Errorf("Expected error type %v, got %v", tt.errorType, err)
				}
			} else if err != nil {
				t.Errorf("Unexpected error for valid URL %s: %v", tt.url, err)
			}
		})
	}
}

func TestNewSlackHandler_URLValidation(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		runID       string
		expectError bool
	}{
		{
			name:        "valid URL and run ID",
			url:         "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
			runID:       "test-run-123",
			expectError: false,
		},
		{
			name:        "invalid URL",
			url:         "http://invalid-url",
			runID:       "test-run-123",
			expectError: true,
		},
		{
			name:        "empty URL",
			url:         "",
			runID:       "test-run-123",
			expectError: true,
		},
		{
			name:        "valid URL with empty run ID (should work)",
			url:         "https://hooks.slack.com/services/T00000000/B00000000/XXXXXXXXXXXXXXXXXXXXXXXX",
			runID:       "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewSlackHandler(tt.url, tt.runID)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for URL: %s", tt.url)
				}
				if handler != nil {
					t.Errorf("Expected nil handler when error occurs")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid input: %v", err)
				}
				if handler == nil {
					t.Errorf("Expected non-nil handler for valid input")
				}
				if handler != nil {
					if handler.webhookURL != tt.url {
						t.Errorf("Expected webhook URL %s, got %s", tt.url, handler.webhookURL)
					}
					if handler.runID != tt.runID {
						t.Errorf("Expected run ID %s, got %s", tt.runID, handler.runID)
					}
				}
			}
		})
	}
}

func TestGetSlackWebhookURL(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected string
	}{
		{
			name:     "with environment variable",
			envValue: "https://hooks.slack.com/services/TEST/WEBHOOK/URL",
			expected: "https://hooks.slack.com/services/TEST/WEBHOOK/URL",
		},
		{
			name:     "without environment variable",
			envValue: "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original environment variable
			originalValue := os.Getenv("GSCR_SLACK_WEBHOOK_URL")
			defer func() {
				if originalValue != "" {
					_ = os.Setenv("GSCR_SLACK_WEBHOOK_URL", originalValue)
				} else {
					_ = os.Unsetenv("GSCR_SLACK_WEBHOOK_URL")
				}
			}()

			// Set test environment variable
			if tt.envValue != "" {
				_ = os.Setenv("GSCR_SLACK_WEBHOOK_URL", tt.envValue)
			} else {
				_ = os.Unsetenv("GSCR_SLACK_WEBHOOK_URL")
			}

			result := GetSlackWebhookURL()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSlackHandler_Enabled(t *testing.T) {
	tests := []struct {
		name          string
		handlerLevel  slog.Level
		recordLevel   slog.Level
		expectEnabled bool
	}{
		{
			name:          "Info handler with Info record",
			handlerLevel:  slog.LevelInfo,
			recordLevel:   slog.LevelInfo,
			expectEnabled: true,
		},
		{
			name:          "Info handler with Warn record",
			handlerLevel:  slog.LevelInfo,
			recordLevel:   slog.LevelWarn,
			expectEnabled: true,
		},
		{
			name:          "Info handler with Debug record",
			handlerLevel:  slog.LevelInfo,
			recordLevel:   slog.LevelDebug,
			expectEnabled: false,
		},
		{
			name:          "Warn handler with Info record",
			handlerLevel:  slog.LevelWarn,
			recordLevel:   slog.LevelInfo,
			expectEnabled: false,
		},
		{
			name:          "Error handler with Error record",
			handlerLevel:  slog.LevelError,
			recordLevel:   slog.LevelError,
			expectEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &SlackHandler{
				webhookURL: "https://hooks.slack.com/test",
				runID:      "test-run",
				level:      tt.handlerLevel,
			}

			ctx := context.Background()
			enabled := handler.Enabled(ctx, tt.recordLevel)

			if enabled != tt.expectEnabled {
				t.Errorf("Expected enabled=%v, got %v", tt.expectEnabled, enabled)
			}
		})
	}
}

func TestSlackHandler_Handle_NoSlackNotify(t *testing.T) {
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
	}

	ctx := context.Background()
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	// No slack_notify attribute

	err := handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Expected no error when slack_notify is missing, got %v", err)
	}
}

func TestSlackHandler_Handle_SlackNotifyFalse(t *testing.T) {
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
	}

	ctx := context.Background()
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	record.AddAttrs(slog.Bool("slack_notify", false))

	err := handler.Handle(ctx, record)
	if err != nil {
		t.Errorf("Expected no error when slack_notify is false, got %v", err)
	}
}

func TestSlackHandler_Handle_WithMockServer(t *testing.T) {
	tests := []struct {
		name            string
		messageType     string
		recordAttrs     []slog.Attr
		expectSuccess   bool
		serverStatus    int
		validateMessage func(t *testing.T, msg SlackMessage)
	}{
		{
			name:        "generic message success",
			messageType: "",
			recordAttrs: []slog.Attr{
				slog.Bool("slack_notify", true),
			},
			expectSuccess: true,
			serverStatus:  http.StatusOK,
			validateMessage: func(t *testing.T, msg SlackMessage) {
				expectedText := "INFO: test message (Run ID: test-run)"
				if msg.Text != expectedText {
					t.Errorf("Expected text %q, got %q", expectedText, msg.Text)
				}
			},
		},
		{
			name:        "command group summary",
			messageType: "command_group_summary",
			recordAttrs: []slog.Attr{
				slog.Bool("slack_notify", true),
				slog.String("message_type", "command_group_summary"),
				slog.String("status", "success"),
				slog.String("group", "test-group"),
				slog.String("command", "echo test"),
				slog.Int("exit_code", 0),
				slog.Int64("duration_ms", 100),
			},
			expectSuccess: true,
			serverStatus:  http.StatusOK,
			validateMessage: func(t *testing.T, msg SlackMessage) {
				if !strings.Contains(msg.Text, "test-group") {
					t.Error("Expected message to contain group name")
				}
			},
		},
		{
			name:        "pre execution error",
			messageType: "pre_execution_error",
			recordAttrs: []slog.Attr{
				slog.Bool("slack_notify", true),
				slog.String("message_type", "pre_execution_error"),
				slog.String("error", "test error"),
			},
			expectSuccess: true,
			serverStatus:  http.StatusOK,
		},
		{
			name:        "security alert",
			messageType: "security_alert",
			recordAttrs: []slog.Attr{
				slog.Bool("slack_notify", true),
				slog.String("message_type", "security_alert"),
				slog.String("alert_type", "test_alert"),
			},
			expectSuccess: true,
			serverStatus:  http.StatusOK,
		},
		{
			name:        "privileged command failure",
			messageType: "privileged_command_failure",
			recordAttrs: []slog.Attr{
				slog.Bool("slack_notify", true),
				slog.String("message_type", "privileged_command_failure"),
			},
			expectSuccess: true,
			serverStatus:  http.StatusOK,
		},
		{
			name:        "privilege escalation failure",
			messageType: "privilege_escalation_failure",
			recordAttrs: []slog.Attr{
				slog.Bool("slack_notify", true),
				slog.String("message_type", "privilege_escalation_failure"),
			},
			expectSuccess: true,
			serverStatus:  http.StatusOK,
		},
		{
			name:        "server error",
			messageType: "",
			recordAttrs: []slog.Attr{
				slog.Bool("slack_notify", true),
			},
			expectSuccess: false,
			serverStatus:  http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedMessage SlackMessage
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("Failed to read request body: %v", err)
				}

				if err := json.Unmarshal(body, &receivedMessage); err != nil {
					t.Errorf("Failed to unmarshal JSON: %v", err)
				}

				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			handler := &SlackHandler{
				webhookURL: server.URL,
				runID:      "test-run",
				httpClient: &http.Client{Timeout: 5 * time.Second},
				level:      slog.LevelInfo,
			}

			ctx := context.Background()
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
			for _, attr := range tt.recordAttrs {
				record.AddAttrs(attr)
			}

			err := handler.Handle(ctx, record)

			if tt.expectSuccess {
				if err != nil {
					t.Errorf("Expected success, got error: %v", err)
				}
				if tt.validateMessage != nil {
					tt.validateMessage(t, receivedMessage)
				}
			} else if err == nil {
				t.Error("Expected error for server failure, got nil")
			}
		})
	}
}

func TestSlackHandler_SendToSlack_Retry(t *testing.T) {
	t.Run("retry on temporary failure", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attemptCount++
			if attemptCount < 2 {
				w.WriteHeader(http.StatusServiceUnavailable)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))
		defer server.Close()

		handler := &SlackHandler{
			webhookURL: server.URL,
			runID:      "test-run",
			httpClient: &http.Client{Timeout: 5 * time.Second},
			level:      slog.LevelInfo,
		}

		ctx := context.Background()
		message := SlackMessage{Text: "test"}

		err := handler.sendToSlack(ctx, message)
		if err != nil {
			t.Errorf("Expected success after retry, got error: %v", err)
		}

		if attemptCount < 2 {
			t.Errorf("Expected at least 2 attempts, got %d", attemptCount)
		}
	})

	t.Run("no retry on client error", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attemptCount++
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		handler := &SlackHandler{
			webhookURL: server.URL,
			runID:      "test-run",
			httpClient: &http.Client{Timeout: 5 * time.Second},
			level:      slog.LevelInfo,
		}

		ctx := context.Background()
		message := SlackMessage{Text: "test"}

		err := handler.sendToSlack(ctx, message)
		if err == nil {
			t.Error("Expected error for client error status")
		}

		if attemptCount != 1 {
			t.Errorf("Expected exactly 1 attempt for client error, got %d", attemptCount)
		}
	})
}

func TestSlackHandler_GenerateBackoffIntervals(t *testing.T) {
	intervals := generateBackoffIntervals(backoffBase, retryCount)

	if len(intervals) != retryCount {
		t.Errorf("Expected %d intervals, got %d", retryCount, len(intervals))
	}

	// Check exponential backoff formula: base * 2^i
	for i := range len(intervals) {
		expected := backoffBase * time.Duration(1<<i)
		if intervals[i] != expected {
			t.Errorf("Expected intervals[%d] == %v (base * 2^%d), got %v",
				i, expected, i, intervals[i])
		}
	}
}
