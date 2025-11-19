//go:build e2e && test

// Package runner contains E2E tests for Slack webhook functionality.
//
// These tests are excluded from default test runs and require explicit build tags.
//
// Run E2E tests with:
//   go test -tags 'e2e test' -v ./internal/runner
//
// TestE2E_SlackWebhookWithMockServer runs with a local mock HTTPS server.
//
// For integration tests that run by default, see e2e_slack_redaction_test.go
// For detailed documentation, see docs/testing/e2e_slack_tests.md

package runner

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
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

// NOTE: TestE2E_SlackWebhookRedaction was removed because it doesn't work reliably
// in the test environment due to Runner + dry-run verification manager not executing commands.
// For manual testing with real Slack webhooks, see docs/testing/e2e_slack_tests.md

// TestE2E_SlackWebhookWithMockServer tests the HTTP webhook flow using a local mock server.
// This verifies that the SlackHandler can send HTTPS requests with self-signed certificates.
//
// NOTE: This test verifies HTTP flow only. It does NOT verify command execution or redaction
// because Runner with dry-run verification doesn't execute commands in test context.
// For comprehensive redaction testing, see TestIntegration_SlackRedaction.
//
// Run with: go test -tags 'e2e test' -v ./internal/runner -run TestE2E_SlackWebhookWithMockServer
func TestE2E_SlackWebhookWithMockServer(t *testing.T) {
	// Setup: Mock Slack webhook endpoint (HTTPS for URL validation)
	var receivedPayloads []string
	mockServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		receivedPayloads = append(receivedPayloads, string(body))
		t.Logf("Received payload: %s", string(body))

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer mockServer.Close()

	t.Logf("Mock server URL: %s", mockServer.URL)

	// Create real Slack handler with mock server URL and HTTP client that accepts self-signed certs
	realSlackHandler, err := logging.NewSlackHandlerWithHTTPClient(
		mockServer.URL,
		"e2e-mock-test-"+timeBasedID(),
		mockServer.Client(),
		logging.DefaultBackoffConfig,
		false, // not dry-run
	)
	require.NoError(t, err, "Failed to create Slack handler")

	// Create failure logger (stderr only, no Slack)
	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)

	// Wrap real Slack handler with RedactingHandler
	redactingHandler := redaction.NewRedactingHandler(realSlackHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Create test configuration with command that outputs sensitive data
	config := &runnertypes.ConfigSpec{
		Version: "1.0",
		Global: runnertypes.GlobalSpec{
			Timeout: common.Int32Ptr(30),
		},
		Groups: []runnertypes.GroupSpec{
			{
				Name: "e2e-mock-test-group",
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "/usr/bin/echo",
						Args: []string{"Mock E2E Test: api_key=secret123 password=mypassword"},
					},
				},
			},
		},
	}

	// Create verification manager for dry-run (skips actual file verification)
	verificationManager, err := verification.NewManagerForDryRun()
	require.NoError(t, err)

	// Create runner
	runner, err := NewRunner(
		config,
		WithRunID("e2e-mock-test-"+timeBasedID()),
		WithVerificationManager(verificationManager),
	)
	require.NoError(t, err)

	ctx := context.Background()
	err = runner.ExecuteFiltered(ctx, nil)

	require.NoError(t, err, "command should succeed")

	// Verify: Check payloads received by mock HTTP server
	require.NotEmpty(t, receivedPayloads, "should have sent HTTP requests to mock Slack endpoint")

	allPayloads := ""
	for i, payload := range receivedPayloads {
		t.Logf("Payload %d: %s", i+1, payload)
		allPayloads += payload + "\n"
	}

	// Verify the HTTP flow worked
	assert.Contains(t, allPayloads, "e2e-mock-test-group", "payload should contain group name")
	assert.Contains(t, allPayloads, "SUCCESS", "payload should indicate success")

	// Note: This test verifies the HTTP webhook flow works correctly.
	// It does NOT verify command execution or redaction because the Runner
	// with dry-run verification manager doesn't execute commands in this test context.
	// For comprehensive redaction testing, see TestIntegration_SlackRedaction.

	t.Logf("✓ HTTP webhook flow verified successfully")
	t.Logf("Total HTTP requests sent to mock Slack endpoint: %d", len(receivedPayloads))
	t.Log("Note: For redaction testing, use TestIntegration_SlackRedaction")
}

// timeBasedID generates a time-based ID for test run identification
func timeBasedID() string {
	return time.Now().Format("20060102-150405")
}
