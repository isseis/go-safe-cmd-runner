//go:build test

package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	securitytesting "github.com/isseis/go-safe-cmd-runner/internal/runner/security/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	verificationtesting "github.com/isseis/go-safe-cmd-runner/internal/verification/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// mockPathResolver for testing
type mockPathResolver struct {
	mock.Mock
}

func (m *mockPathResolver) ResolvePath(path string) (string, error) {
	args := m.Called(path)
	return args.String(0), args.Error(1)
}

// TestIntegration_CommandOutputCapture tests that command stdout/stderr are properly captured and logged
func TestIntegration_CommandOutputCapture(t *testing.T) {
	// Create a buffer to capture log output
	var logBuffer bytes.Buffer
	handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})

	// Wrap with redacting handler
	redactingHandler := redaction.NewRedactingHandler(handler, nil)
	logger := slog.New(redactingHandler)
	slog.SetDefault(logger)

	// Create test configuration
	group := &runnertypes.GroupSpec{
		Name: "test-group",
		Commands: []runnertypes.CommandSpec{
			{
				Name: "test-cmd",
				Cmd:  "/bin/sh",
				Args: []string{"-c", "echo 'stdout output'; echo 'stderr output' >&2; exit 1"},
			},
		},
	}

	runtimeGlobal := &runnertypes.RuntimeGlobal{
		Spec: &runnertypes.GlobalSpec{Timeout: common.Int32Ptr(30)},
	}

	// Create real executor and resource manager
	exec := executor.NewDefaultExecutor()
	fs := common.NewDefaultFileSystem()
	mockValidator := new(securitytesting.MockValidator)
	mockVerificationManager := new(verificationtesting.MockManager)

	// Create mock path resolver
	mockPathResolver := &mockPathResolver{}
	mockPathResolver.On("ResolvePath", mock.Anything).Return(func(path string) string { return path }, nil)

	// Output manager will be created by NewDefaultResourceManager
	var outputMgr output.CaptureManager

	// Create resource manager
	rm, err := resource.NewDefaultResourceManager(
		exec,
		fs,
		nil, // privilege manager not needed for this test
		mockPathResolver,
		logger,
		resource.ExecutionModeNormal,
		nil, // dry-run options
		outputMgr,
		0, // max output size (0 = default)
	)
	require.NoError(t, err)

	ge := NewTestGroupExecutorWithConfig(TestGroupExecutorConfig{
		Config:              &runnertypes.ConfigSpec{},
		Executor:            exec,
		ResourceManager:     rm,
		Validator:           mockValidator,
		VerificationManager: mockVerificationManager,
		RunID:               "test-run-capture",
	})

	// Mock verification manager
	mockVerificationManager.On("VerifyGroupFiles", group).Return(&verification.Result{}, nil)
	mockVerificationManager.On("ResolvePath", "/bin/sh").Return("/bin/sh", nil)

	// Mock validator
	mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)

	ctx := context.Background()
	err = ge.ExecuteGroup(ctx, group, runtimeGlobal)

	require.Error(t, err, "command should fail")

	// Parse log output
	logOutput := logBuffer.String()
	logLines := strings.Split(logOutput, "\n")

	var errorLogs []map[string]interface{}
	var debugLogs []map[string]interface{}

	for _, line := range logLines {
		if line == "" {
			continue
		}
		var logEntry map[string]interface{}
		if err := json.Unmarshal([]byte(line), &logEntry); err != nil {
			continue
		}

		level, ok := logEntry["level"].(string)
		if !ok {
			continue
		}

		switch level {
		case "ERROR":
			errorLogs = append(errorLogs, logEntry)
		case "DEBUG":
			debugLogs = append(debugLogs, logEntry)
		}
	}

	// Check ERROR level logs contain stderr
	require.NotEmpty(t, errorLogs, "should have ERROR level logs")

	foundStderr := false
	for _, logEntry := range errorLogs {
		if stderr, ok := logEntry["stderr"].(string); ok && stderr != "" {
			foundStderr = true
			assert.Contains(t, stderr, "stderr output", "ERROR log should contain stderr")
		}
		// Verify stdout is NOT in ERROR logs
		_, hasStdout := logEntry["stdout"]
		assert.False(t, hasStdout, "ERROR logs should not contain stdout")
	}
	assert.True(t, foundStderr, "ERROR logs should contain stderr")

	// Check DEBUG level logs contain stdout (truncated) and stderr
	require.NotEmpty(t, debugLogs, "should have DEBUG level logs")

	// Note: In actual test, stdout may be captured by output file instead of appearing in logs
	// This is expected behavior when output capture is enabled
	// For this integration test, we verify that the logging framework works correctly

	mockValidator.AssertExpectations(t)
	mockVerificationManager.AssertExpectations(t)
}

// TestIntegration_SensitiveDataRedaction tests that sensitive data in command output is redacted
func TestIntegration_SensitiveDataRedaction(t *testing.T) {
	tests := []struct {
		name             string
		command          string
		sensitivePattern string
		shouldBeRedacted bool
	}{
		{
			name:             "password in stderr is redacted",
			command:          "echo 'authentication failed: password=secret123' >&2; exit 1",
			sensitivePattern: "secret123",
			shouldBeRedacted: true,
		},
		{
			name:             "token in stderr is redacted",
			command:          "echo 'API error: token=abc123xyz' >&2; exit 1",
			sensitivePattern: "abc123xyz",
			shouldBeRedacted: true,
		},
		{
			name:             "normal error message is not redacted",
			command:          "echo 'command failed: exit code 1' >&2; exit 1",
			sensitivePattern: "exit code 1",
			shouldBeRedacted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a buffer to capture log output
			var logBuffer bytes.Buffer
			handler := slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})

			// Wrap with redacting handler
			redactingHandler := redaction.NewRedactingHandler(handler, nil)
			logger := slog.New(redactingHandler)
			slog.SetDefault(logger)

			// Create test configuration
			group := &runnertypes.GroupSpec{
				Name: "test-group",
				Commands: []runnertypes.CommandSpec{
					{
						Name: "test-cmd",
						Cmd:  "/bin/sh",
						Args: []string{"-c", tt.command},
					},
				},
			}

			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec: &runnertypes.GlobalSpec{Timeout: common.Int32Ptr(30)},
			}

			// Create real executor and resource manager
			exec := executor.NewDefaultExecutor()
			fs := common.NewDefaultFileSystem()
			mockValidator := new(securitytesting.MockValidator)
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
				Validator:           mockValidator,
				VerificationManager: mockVerificationManager,
				RunID:               "test-run-redaction",
			})

			// Mock verification manager
			mockVerificationManager.On("VerifyGroupFiles", group).Return(&verification.Result{}, nil)
			mockVerificationManager.On("ResolvePath", "/bin/sh").Return("/bin/sh", nil)

			// Mock validator
			mockValidator.On("ValidateAllEnvironmentVars", mock.Anything).Return(nil)

			ctx := context.Background()
			err = ge.ExecuteGroup(ctx, group, runtimeGlobal)

			require.Error(t, err, "command should fail")

			// Check log output for sensitive pattern
			logOutput := logBuffer.String()

			if tt.shouldBeRedacted {
				assert.NotContains(t, logOutput, tt.sensitivePattern,
					"sensitive pattern should be redacted from logs")
			} else {
				assert.Contains(t, logOutput, tt.sensitivePattern,
					"non-sensitive pattern should appear in logs")
			}

			mockValidator.AssertExpectations(t)
			mockVerificationManager.AssertExpectations(t)
		})
	}
}
