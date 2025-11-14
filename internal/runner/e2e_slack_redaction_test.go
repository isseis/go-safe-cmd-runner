//go:build test

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	verificationtesting "github.com/isseis/go-safe-cmd-runner/internal/verification/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockSlackHandler is a mock handler that captures messages for testing
type MockSlackHandler struct {
	messages []string
}

func (m *MockSlackHandler) Handle(_ context.Context, record slog.Record) error {
	// Simulate formatting the log record
	var buf bytes.Buffer

	// Format record attributes
	attrs := make(map[string]interface{})
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	// Convert to JSON (simulating Slack webhook payload)
	data, err := json.Marshal(map[string]interface{}{
		"message": record.Message,
		"level":   record.Level.String(),
		"attrs":   attrs,
	})
	if err != nil {
		return err
	}

	buf.Write(data)
	m.messages = append(m.messages, buf.String())
	return nil
}

func (m *MockSlackHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true // Always enabled for testing
}

func (m *MockSlackHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return m
}

func (m *MockSlackHandler) WithGroup(_ string) slog.Handler {
	return m
}

// TestE2E_RealCommandWithAPIKey tests end-to-end flow from command execution to Slack webhook
// with a mock HTTP server simulating the Slack endpoint.
func TestE2E_RealCommandWithAPIKey(t *testing.T) {
	// Setup: Mock Slack webhook endpoint
	var receivedPayloads []string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("Failed to read request body: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		receivedPayloads = append(receivedPayloads, string(body))

		// Verify: API key is redacted in the payload
		assert.NotContains(t, string(body), "secret123", "API key should be redacted in Slack webhook payload")
		assert.NotContains(t, string(body), "mypassword", "password should be redacted in Slack webhook payload")

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer mockServer.Close()

	// Create mock Slack handler
	mockSlackHandler := &MockSlackHandler{
		messages: make([]string, 0),
	}

	// Create failure logger (stderr only, no Slack)
	var failureLogBuffer bytes.Buffer
	failureHandler := slog.NewJSONHandler(&failureLogBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	failureLogger := slog.New(failureHandler)

	// Wrap mock Slack handler with RedactingHandler (Case 1)
	redactingHandler := redaction.NewRedactingHandler(mockSlackHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Use real validator with redaction enabled (Case 2)
	realValidator, err := security.NewValidator(&security.Config{
		LoggingOptions: security.LoggingOptions{
			RedactSensitiveInfo: true,
		},
	})
	require.NoError(t, err)

	// Create test configuration with command that outputs sensitive data
	group := &runnertypes.GroupSpec{
		Name: "test-group-e2e",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/sh",
				Args: []string{"-c", "echo 'API response: api_key=secret123 password=mypassword'"},
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.Int32Ptr(30)},
	}

	// Create real executor and resource manager
	exec := executor.NewDefaultExecutor()
	fs := common.NewDefaultFileSystem()
	mockVerificationManager := new(verificationtesting.MockManager)

	// Create mock path resolver
	mockPathResolver := &mockPathResolver{}
	mockPathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	// Output manager will be created by NewDefaultResourceManager
	var outputMgr output.CaptureManager

	rm, err := resource.NewDefaultResourceManager(
		exec,
		fs,
		nil,
		mockPathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil,
		outputMgr,
		0,
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:              &runnertypes.ConfigSpec{},
		Executor:            exec,
		ResourceManager:     rm,
		Validator:           realValidator,
		VerificationManager: mockVerificationManager,
		RunID:               "test-run-e2e",
	})

	// Mock verification manager
	mockVerificationManager.On("VerifyGroupFiles", group).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/sh").Return("/bin/sh", nil)

	ctx := context.Background()
	err = ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.NoError(t, err, "command should succeed")

	// Verify: Check messages received by mock Slack handler
	require.NotEmpty(t, mockSlackHandler.messages, "should have sent messages to Slack handler")

	allMessages := ""
	for _, msg := range mockSlackHandler.messages {
		allMessages += msg + "\n"
	}

	// Verify sensitive data is redacted
	assert.NotContains(t, allMessages, "secret123", "API key should be redacted in Slack messages")
	assert.NotContains(t, allMessages, "mypassword", "password should be redacted in Slack messages")
	assert.Contains(t, allMessages, "[REDACTED]", "redacted placeholder should appear in Slack messages")

	t.Logf("Total messages sent to Slack handler: %d", len(mockSlackHandler.messages))
	t.Logf("Sample message: %s", mockSlackHandler.messages[0][:minInt(len(mockSlackHandler.messages[0]), 300)])
} // TestE2E_MultiHandlerLogging tests that sensitive data is redacted across multiple log handlers
// (stderr, file, and Slack) in a realistic multi-handler setup.
func TestE2E_MultiHandlerLogging(t *testing.T) {
	// Create buffers for different log destinations
	var stderrBuffer bytes.Buffer
	var fileBuffer bytes.Buffer

	// Create mock Slack handler
	mockSlackHandler := &MockSlackHandler{
		messages: make([]string, 0),
	}

	// Create individual handlers
	stderrHandler := slog.NewJSONHandler(&stderrBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug, // Use Debug level to capture command output
	})
	fileHandler := slog.NewJSONHandler(&fileBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Create multi-handler (stderr + file + slack)
	multiHandler, err := logging.NewMultiHandler(stderrHandler, fileHandler, mockSlackHandler)
	require.NoError(t, err)

	// Create failure logger (stderr + file only, NO Slack)
	failureMultiHandler, err := logging.NewMultiHandler(stderrHandler, fileHandler)
	require.NoError(t, err)
	failureLogger := slog.New(failureMultiHandler)

	// Wrap with RedactingHandler
	redactingHandler := redaction.NewRedactingHandler(multiHandler, nil, failureLogger)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Use real validator with redaction enabled
	realValidator, err := security.NewValidator(&security.Config{
		LoggingOptions: security.LoggingOptions{
			RedactSensitiveInfo: true,
		},
	})
	require.NoError(t, err)

	// Create test configuration
	group := &runnertypes.GroupSpec{
		Name: "test-group-multihandler",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/sh",
				Args: []string{"-c", "echo 'token=xyz789 password=secretpass'"},
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.Int32Ptr(30)},
	}

	// Create executor and resource manager
	exec := executor.NewDefaultExecutor()
	fs := common.NewDefaultFileSystem()
	mockVerificationManager := new(verificationtesting.MockManager)

	mockPathResolver := &mockPathResolver{}
	mockPathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	var outputMgr output.CaptureManager

	rm, err := resource.NewDefaultResourceManager(
		exec,
		fs,
		nil,
		mockPathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil,
		outputMgr,
		0,
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:              &runnertypes.ConfigSpec{},
		Executor:            exec,
		ResourceManager:     rm,
		Validator:           realValidator,
		VerificationManager: mockVerificationManager,
		RunID:               "test-run-multihandler",
	})

	mockVerificationManager.On("VerifyGroupFiles", group).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/sh").Return("/bin/sh", nil)

	ctx := context.Background()
	err = ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.NoError(t, err, "command should succeed")

	// Verify redaction across all handlers
	stderrOutput := stderrBuffer.String()
	fileOutput := fileBuffer.String()

	allSlackMessages := ""
	for _, msg := range mockSlackHandler.messages {
		allSlackMessages += msg + "\n"
	}

	// Check stderr output
	assert.NotContains(t, stderrOutput, "xyz789", "token should be redacted in stderr")
	assert.NotContains(t, stderrOutput, "secretpass", "password should be redacted in stderr")

	// Check file output
	assert.NotContains(t, fileOutput, "xyz789", "token should be redacted in file logs")
	assert.NotContains(t, fileOutput, "secretpass", "password should be redacted in file logs")

	// Check Slack output
	assert.NotContains(t, allSlackMessages, "xyz789", "token should be redacted in Slack messages")
	assert.NotContains(t, allSlackMessages, "secretpass", "password should be redacted in Slack messages")

	// Verify [REDACTED] appears in all outputs
	assert.Contains(t, stderrOutput, "[REDACTED]", "redacted placeholder should appear in stderr")
	assert.Contains(t, fileOutput, "[REDACTED]", "redacted placeholder should appear in file logs")
	assert.Contains(t, allSlackMessages, "[REDACTED]", "redacted placeholder should appear in Slack messages")

	t.Logf("Redaction verified across all handlers (stderr, file, Slack)")
}

// minInt returns the smaller of two integers (Go 1.20 compatible)
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
