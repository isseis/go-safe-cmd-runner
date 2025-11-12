package logging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// Test-specific backoff configuration constants
	testBackoffBase = 10 * time.Millisecond
	testRetryCount  = 3
)

// testBackoffConfig is the test backoff configuration with shorter intervals
var testBackoffConfig = BackoffConfig{
	Base:       testBackoffBase,
	RetryCount: testRetryCount,
}

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
	assert.Len(t, newHandler.attrs, 2, "New handler should have 2 attributes")

	// Verify the original handler is unchanged
	assert.Empty(t, handler.attrs, "Original handler should not be modified")

	// Test chaining WithAttrs
	moreAttrs := []slog.Attr{
		slog.String("key3", "value3"),
	}

	chainedHandler := newHandler.WithAttrs(moreAttrs).(*SlackHandler)

	assert.Len(t, chainedHandler.attrs, 3, "Chained handler should have 3 attributes")
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
	require.Len(t, newHandler.groups, 1, "New handler should have 1 group")
	assert.Equal(t, "group1", newHandler.groups[0], "Group name should be 'group1'")

	// Verify the original handler is unchanged
	assert.Empty(t, handler.groups, "Original handler should not be modified")

	// Test chaining WithGroup
	chainedHandler := newHandler.WithGroup("group2").(*SlackHandler)

	require.Len(t, chainedHandler.groups, 2, "Chained handler should have 2 groups")
	assert.Equal(t, "group2", chainedHandler.groups[1], "Second group name should be 'group2'")
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

	assert.Len(t, combined.attrs, 1, "Combined handler should have 1 attribute")
	require.Len(t, combined.groups, 1, "Combined handler should have 1 group")
	assert.Equal(t, "testgroup", combined.groups[0], "Group name should be 'testgroup'")
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
	var hasAccumulated, hasOriginal, hasGroup bool

	newRecord.Attrs(func(attr slog.Attr) bool {
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

	assert.True(t, hasOriginal, "Original attribute should be present")
	assert.True(t, hasGroup, "Group should be present")
	assert.True(t, hasAccumulated, "Accumulated attribute should be present in group")
}

func TestSlackHandler_WithAttrsEmptySlice(t *testing.T) {
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
	}

	// WithAttrs with empty slice should return the same handler
	newHandler := handler.WithAttrs([]slog.Attr{})

	assert.Same(t, handler, newHandler, "WithAttrs with empty slice should return the same handler")
}

func TestSlackHandler_WithGroupEmptyString(t *testing.T) {
	handler := &SlackHandler{
		webhookURL: "https://hooks.slack.com/test",
		runID:      "test-run",
		level:      slog.LevelInfo,
	}

	// WithGroup with empty string should return the same handler
	newHandler := handler.WithGroup("")

	assert.Same(t, handler, newHandler, "WithGroup with empty string should return the same handler")
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
				require.Error(t, err, "Expected error for URL: %s", tt.url)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType, "Expected specific error type")
				}
			} else {
				assert.NoError(t, err, "Unexpected error for valid URL %s", tt.url)
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
				require.Error(t, err, "Expected error for URL: %s", tt.url)
				assert.Nil(t, handler, "Expected nil handler when error occurs")
			} else {
				require.NoError(t, err, "Unexpected error for valid input")
				require.NotNil(t, handler, "Expected non-nil handler for valid input")
				assert.Equal(t, tt.url, handler.webhookURL, "Webhook URL should match")
				assert.Equal(t, tt.runID, handler.runID, "Run ID should match")
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

			assert.Equal(t, tt.expectEnabled, enabled, "Enabled should return expected value")
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
	assert.NoError(t, err, "Expected no error when slack_notify is missing")
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
	assert.NoError(t, err, "Expected no error when slack_notify is false")
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
				expectedText := fmt.Sprintf("%s: test message (Run ID: test-run)", slog.LevelInfo.String())
				assert.Equal(t, expectedText, msg.Text, "Message text should match expected format")
			},
		},
		{
			name:        "command group summary",
			messageType: "command_group_summary",
			recordAttrs: []slog.Attr{
				slog.Bool("slack_notify", true),
				slog.String("message_type", "command_group_summary"),
				slog.String(common.LogAttrStatus, "success"),
				slog.String(common.LogAttrGroup, "test-group"),
				slog.String("command", "echo test"),
				slog.Int("exit_code", 0),
				slog.Int64(common.LogAttrDurationMs, 100),
			},
			expectSuccess: true,
			serverStatus:  http.StatusOK,
			validateMessage: func(t *testing.T, msg SlackMessage) {
				assert.Contains(t, msg.Text, "test-group", "Message should contain group name")
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
				assert.Equal(t, http.MethodPost, r.Method, "Request method should be POST")

				body, err := io.ReadAll(r.Body)
				require.NoError(t, err, "Failed to read request body")

				err = json.Unmarshal(body, &receivedMessage)
				require.NoError(t, err, "Failed to unmarshal JSON")

				w.WriteHeader(tt.serverStatus)
			}))
			defer server.Close()

			handler := &SlackHandler{
				webhookURL:    server.URL,
				runID:         "test-run",
				httpClient:    &http.Client{Timeout: 5 * time.Second},
				level:         slog.LevelInfo,
				backoffConfig: testBackoffConfig,
			}

			ctx := context.Background()
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
			for _, attr := range tt.recordAttrs {
				record.AddAttrs(attr)
			}

			err := handler.Handle(ctx, record)

			if tt.expectSuccess {
				assert.NoError(t, err, "Expected success, got error")
				if tt.validateMessage != nil {
					tt.validateMessage(t, receivedMessage)
				}
			} else {
				assert.Error(t, err, "Expected error for server failure")
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
			webhookURL:    server.URL,
			runID:         "test-run",
			httpClient:    &http.Client{Timeout: 5 * time.Second},
			level:         slog.LevelInfo,
			backoffConfig: testBackoffConfig,
		}

		ctx := context.Background()
		message := SlackMessage{Text: "test"}

		err := handler.sendToSlack(ctx, message)
		assert.NoError(t, err, "Expected success after retry")
		assert.GreaterOrEqual(t, attemptCount, 2, "Expected at least 2 attempts")
	})

	t.Run("no retry on client error", func(t *testing.T) {
		attemptCount := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attemptCount++
			w.WriteHeader(http.StatusBadRequest)
		}))
		defer server.Close()

		handler := &SlackHandler{
			webhookURL:    server.URL,
			runID:         "test-run",
			httpClient:    &http.Client{Timeout: 5 * time.Second},
			level:         slog.LevelInfo,
			backoffConfig: testBackoffConfig,
		}

		ctx := context.Background()
		message := SlackMessage{Text: "test"}

		err := handler.sendToSlack(ctx, message)
		assert.Error(t, err, "Expected error for client error status")
		assert.Equal(t, 1, attemptCount, "Expected exactly 1 attempt for client error")
	})
}

func TestSlackHandler_GenerateBackoffIntervals(t *testing.T) {
	tests := []struct {
		name        string
		base        time.Duration
		retryCount  int
		description string
	}{
		{
			name:        "default config",
			base:        DefaultBackoffConfig.Base,
			retryCount:  DefaultBackoffConfig.RetryCount,
			description: "Should generate intervals for default configuration",
		},
		{
			name:        "test config",
			base:        testBackoffConfig.Base,
			retryCount:  testBackoffConfig.RetryCount,
			description: "Should generate intervals for test configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intervals := generateBackoffIntervals(tt.base, tt.retryCount)

			assert.Len(t, intervals, tt.retryCount,
				"Should generate correct number of intervals for %s", tt.name)

			// Check exponential backoff formula: base * 2^i
			for i := range len(intervals) {
				expected := tt.base * time.Duration(1<<i)
				assert.Equal(t, expected, intervals[i],
					"Interval[%d] should follow exponential backoff formula (base * 2^%d)", i, i)
			}
		})
	}
}
