//go:build e2e && test

// Package runner contains E2E tests for Slack webhook separation functionality.
//
// These tests verify that INFO logs are sent to the SUCCESS webhook and
// WARN/ERROR logs are sent to the ERROR webhook.
//
// Run E2E tests with:
//   go test -tags 'e2e test' -v ./internal/runner -run TestE2E_SlackWebhookSeparation

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestE2E_SlackWebhookSeparation_SuccessOnly tests IT-01: SUCCESS webhook receives INFO logs
func TestE2E_SlackWebhookSeparation_SuccessOnly(t *testing.T) {
	var successPayloads []string
	var errorPayloads []string

	// Create mock servers for both webhooks
	successServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		successPayloads = append(successPayloads, string(body))
		t.Logf("SUCCESS webhook received: %s", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	errorServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		errorPayloads = append(errorPayloads, string(body))
		t.Logf("ERROR webhook received: %s", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer errorServer.Close()

	// Create two Slack handlers
	successHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: successServer.URL,
		RunID:      "test-success-" + timeBasedID(),
		HTTPClient: successServer.Client(),
		LevelMode:  logging.LevelModeExactInfo,
		IsDryRun:   false,
	})
	require.NoError(t, err)

	errorHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: errorServer.URL,
		RunID:      "test-error-" + timeBasedID(),
		HTTPClient: errorServer.Client(),
		LevelMode:  logging.LevelModeWarnAndAbove,
		IsDryRun:   false,
	})
	require.NoError(t, err)

	// Create multi-handler with both webhooks
	multiHandler, err := logging.NewMultiHandler(successHandler, errorHandler)
	require.NoError(t, err)

	// Wrap with redacting handler
	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)
	redactingHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Create configuration with successful command
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: common.Int32Ptr(30),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name: "success-test-group",
				Commands: []runnertypes.CommandSpec{
					{
						Name: "success-cmd",
						Cmd:  "/usr/bin/echo",
						Args: []string{"test successful"},
					},
				},
			},
		},
	}

	verificationManager, err := verification.NewManagerForDryRun()
	require.NoError(t, err)

	runner, err := NewRunner(
		config,
		WithRunID("test-success-"+timeBasedID()),
		WithVerificationManager(verificationManager),
	)
	require.NoError(t, err)

	ctx := context.Background()
	err = runner.Execute(ctx, nil)
	require.NoError(t, err)

	// Wait for async Slack notifications
	time.Sleep(500 * time.Millisecond)

	// Verify: SUCCESS webhook should receive INFO logs
	require.NotEmpty(t, successPayloads, "SUCCESS webhook should receive notifications")
	assert.Empty(t, errorPayloads, "ERROR webhook should NOT receive notifications for successful execution")

	// Verify payload content
	allSuccessPayloads := strings.Join(successPayloads, "\n")
	assert.Contains(t, allSuccessPayloads, "success-test-group", "should contain group name")
	assert.Contains(t, allSuccessPayloads, "SUCCESS", "should indicate success")

	t.Logf("✓ IT-01: SUCCESS webhook correctly receives INFO logs only")
}

// TestE2E_SlackWebhookSeparation_ErrorOnly tests IT-02: ERROR webhook receives ERROR logs
func TestE2E_SlackWebhookSeparation_ErrorOnly(t *testing.T) {
	var successPayloads []string
	var errorPayloads []string

	successServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		successPayloads = append(successPayloads, string(body))
		t.Logf("SUCCESS webhook received: %s", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	errorServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		errorPayloads = append(errorPayloads, string(body))
		t.Logf("ERROR webhook received: %s", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer errorServer.Close()

	successHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: successServer.URL,
		RunID:      "test-error-" + timeBasedID(),
		HTTPClient: successServer.Client(),
		LevelMode:  logging.LevelModeExactInfo,
		IsDryRun:   false,
	})
	require.NoError(t, err)

	errorHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: errorServer.URL,
		RunID:      "test-error-" + timeBasedID(),
		HTTPClient: errorServer.Client(),
		LevelMode:  logging.LevelModeWarnAndAbove,
		IsDryRun:   false,
	})
	require.NoError(t, err)

	multiHandler, err := logging.NewMultiHandler(successHandler, errorHandler)
	require.NoError(t, err)

	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)
	redactingHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Create configuration with failing command
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: common.Int32Ptr(30),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name: "error-test-group",
				Commands: []runnertypes.CommandSpec{
					{
						Name: "failing-cmd",
						Cmd:  "/usr/bin/false", // Always exits with error
						Args: []string{},
					},
				},
			},
		},
	}

	verificationManager, err := verification.NewManagerForDryRun()
	require.NoError(t, err)

	runner, err := NewRunner(
		config,
		WithRunID("test-error-"+timeBasedID()),
		WithVerificationManager(verificationManager),
	)
	require.NoError(t, err)

	ctx := context.Background()
	_ = runner.Execute(ctx, nil) // Expect error, don't check

	// Wait for async Slack notifications
	time.Sleep(500 * time.Millisecond)

	// Verify: ERROR webhook should receive ERROR logs
	assert.Empty(t, successPayloads, "SUCCESS webhook should NOT receive notifications for failed execution")
	require.NotEmpty(t, errorPayloads, "ERROR webhook should receive notifications")

	allErrorPayloads := strings.Join(errorPayloads, "\n")
	assert.Contains(t, allErrorPayloads, "error-test-group", "should contain group name")

	t.Logf("✓ IT-02: ERROR webhook correctly receives ERROR logs only")
}

// TestE2E_SlackWebhookSeparation_WarnToError tests IT-03: WARN logs go to ERROR webhook
func TestE2E_SlackWebhookSeparation_WarnToError(t *testing.T) {
	var successPayloads []string
	var errorPayloads []string

	successServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		successPayloads = append(successPayloads, string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	errorServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		errorPayloads = append(errorPayloads, string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer errorServer.Close()

	successHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: successServer.URL,
		RunID:      "test-warn-" + timeBasedID(),
		HTTPClient: successServer.Client(),
		LevelMode:  logging.LevelModeExactInfo,
		IsDryRun:   false,
	})
	require.NoError(t, err)

	errorHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: errorServer.URL,
		RunID:      "test-warn-" + timeBasedID(),
		HTTPClient: errorServer.Client(),
		LevelMode:  logging.LevelModeWarnAndAbove,
		IsDryRun:   false,
	})
	require.NoError(t, err)

	multiHandler, err := logging.NewMultiHandler(successHandler, errorHandler)
	require.NoError(t, err)

	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)
	redactingHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Log a WARN message directly
	slog.Warn("Test warning message", "slack_notify", true, "message_type", "test_warning")

	// Wait for async Slack notifications
	time.Sleep(500 * time.Millisecond)

	// Verify: ERROR webhook should receive WARN logs
	assert.Empty(t, successPayloads, "SUCCESS webhook should NOT receive WARN logs")
	require.NotEmpty(t, errorPayloads, "ERROR webhook should receive WARN logs")

	allErrorPayloads := strings.Join(errorPayloads, "\n")
	assert.Contains(t, allErrorPayloads, "Test warning message", "should contain warning message")

	t.Logf("✓ IT-03: WARN logs correctly sent to ERROR webhook")
}

// TestE2E_SlackWebhookSeparation_ErrorOnlyConfig tests IT-04: ERROR webhook only (no SUCCESS)
func TestE2E_SlackWebhookSeparation_ErrorOnlyConfig(t *testing.T) {
	var errorPayloads []string

	errorServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		errorPayloads = append(errorPayloads, string(body))
		t.Logf("ERROR webhook received: %s", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer errorServer.Close()

	// Create only ERROR handler (no SUCCESS handler)
	errorHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: errorServer.URL,
		RunID:      "test-error-only-" + timeBasedID(),
		HTTPClient: errorServer.Client(),
		LevelMode:  logging.LevelModeWarnAndAbove,
		IsDryRun:   false,
	})
	require.NoError(t, err)

	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)
	redactingHandler := redaction.NewRedactingHandler(errorHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Log both INFO and ERROR messages
	slog.Info("Success message", "slack_notify", true)
	slog.Error("Error message", "slack_notify", true)

	// Wait for async Slack notifications
	time.Sleep(500 * time.Millisecond)

	// Verify: ERROR webhook should only receive ERROR logs, not INFO
	require.NotEmpty(t, errorPayloads, "ERROR webhook should receive notifications")

	allErrorPayloads := strings.Join(errorPayloads, "\n")
	assert.Contains(t, allErrorPayloads, "Error message", "should contain error message")
	assert.NotContains(t, allErrorPayloads, "Success message", "should NOT contain info message")

	t.Logf("✓ IT-04: ERROR-only configuration works correctly")
}

// TestE2E_SlackWebhookSeparation_DryRunMode tests IT-05: dry-run disables both webhooks
func TestE2E_SlackWebhookSeparation_DryRunMode(t *testing.T) {
	var successPayloads []string
	var errorPayloads []string

	successServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		successPayloads = append(successPayloads, string(body))
		t.Logf("SUCCESS webhook received (should not happen): %s", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	errorServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		errorPayloads = append(errorPayloads, string(body))
		t.Logf("ERROR webhook received (should not happen): %s", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer errorServer.Close()

	// Create handlers with IsDryRun=true
	successHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: successServer.URL,
		RunID:      "test-dryrun-" + timeBasedID(),
		HTTPClient: successServer.Client(),
		LevelMode:  logging.LevelModeExactInfo,
		IsDryRun:   true, // Dry-run mode
	})
	require.NoError(t, err)

	errorHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: errorServer.URL,
		RunID:      "test-dryrun-" + timeBasedID(),
		HTTPClient: errorServer.Client(),
		LevelMode:  logging.LevelModeWarnAndAbove,
		IsDryRun:   true, // Dry-run mode
	})
	require.NoError(t, err)

	multiHandler, err := logging.NewMultiHandler(successHandler, errorHandler)
	require.NoError(t, err)

	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)
	redactingHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Log both INFO and ERROR messages
	slog.Info("Dry-run success", "slack_notify", true)
	slog.Error("Dry-run error", "slack_notify", true)

	// Wait for async processing
	time.Sleep(500 * time.Millisecond)

	// Verify: Neither webhook should receive notifications in dry-run mode
	assert.Empty(t, successPayloads, "SUCCESS webhook should NOT receive notifications in dry-run")
	assert.Empty(t, errorPayloads, "ERROR webhook should NOT receive notifications in dry-run")

	t.Logf("✓ IT-05: Dry-run mode correctly disables both webhooks")
}

// TestE2E_SlackWebhookSeparation_MessageFormat tests message formatting
func TestE2E_SlackWebhookSeparation_MessageFormat(t *testing.T) {
	var successPayloads []map[string]interface{}

	successServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		if err := json.Unmarshal(body, &payload); err == nil {
			successPayloads = append(successPayloads, payload)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer successServer.Close()

	successHandler, err := logging.NewSlackHandler(logging.SlackHandlerOptions{
		WebhookURL: successServer.URL,
		RunID:      "test-format-" + timeBasedID(),
		HTTPClient: successServer.Client(),
		LevelMode:  logging.LevelModeExactInfo,
		IsDryRun:   false,
	})
	require.NoError(t, err)

	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)
	redactingHandler := redaction.NewRedactingHandler(successHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Create configuration
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: common.Int32Ptr(30),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name: "format-test-group",
				Commands: []runnertypes.CommandSpec{
					{
						Name: "format-cmd",
						Cmd:  "/usr/bin/echo",
						Args: []string{"test"},
					},
				},
			},
		},
	}

	verificationManager, err := verification.NewManagerForDryRun()
	require.NoError(t, err)

	runner, err := NewRunner(
		config,
		WithRunID("test-format-"+timeBasedID()),
		WithVerificationManager(verificationManager),
	)
	require.NoError(t, err)

	ctx := context.Background()
	err = runner.Execute(ctx, nil)
	require.NoError(t, err)

	// Wait for async Slack notifications
	time.Sleep(500 * time.Millisecond)

	// Verify message format
	require.NotEmpty(t, successPayloads, "should receive at least one notification")

	// Check that messages are properly formatted
	for _, payload := range successPayloads {
		if text, ok := payload["text"].(string); ok {
			t.Logf("Message text: %s", text)
			// Basic format verification - messages should contain readable text
			assert.NotEmpty(t, text, "message text should not be empty")
		}
	}

	t.Logf("✓ Message format verification complete")
}
