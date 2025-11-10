//go:build test

package executor

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestCommand creates a RuntimeCommand for testing purposes
func createTestCommand(cmd string, args []string) *runnertypes.RuntimeCommand {
	spec := &runnertypes.CommandSpec{
		Name: "test_cmd",
		Cmd:  cmd,
		Args: args,
	}

	rtCmd, err := runnertypes.NewRuntimeCommand(spec, common.NewUnsetTimeout(), nil, "test_group")
	if err != nil {
		panic(err)
	}

	rtCmd.ExpandedCmd = cmd
	rtCmd.ExpandedArgs = args

	return rtCmd
}

func TestExecutor_DebugLogging(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	executor := NewDefaultExecutor(WithLogger(logger))

	cmd := createTestCommand("/bin/echo", []string{"hello", "world with spaces"})

	_, err := executor.Execute(context.Background(), cmd, map[string]string{}, nil)
	require.NoError(t, err)

	// Check that the debug log contains the command
	logOutput := buf.String()
	assert.Contains(t, logOutput, "Executing command")
	assert.Contains(t, logOutput, "/bin/echo hello 'world with spaces'")
}

func TestExecutor_ErrorLogging_CommandNotFound(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	executor := NewDefaultExecutor(WithLogger(logger))

	cmd := createTestCommand("/nonexistent/command", []string{})

	_, err := executor.Execute(context.Background(), cmd, map[string]string{}, nil)
	require.Error(t, err)

	// Check that the error log contains the failure reason
	logOutput := buf.String()
	assert.Contains(t, logOutput, "Failed to find command")
	assert.Contains(t, logOutput, "/nonexistent/command")
}

func TestExecutor_ErrorLogging_CommandExecutionFailure(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	executor := NewDefaultExecutor(WithLogger(logger))

	// Use /bin/false which always exits with non-zero status
	cmd := createTestCommand("/bin/false", []string{})

	_, err := executor.Execute(context.Background(), cmd, map[string]string{}, nil)
	require.Error(t, err)

	// Check that the error log contains the failure information
	logOutput := buf.String()
	assert.Contains(t, logOutput, "Command execution failed")
	assert.Contains(t, logOutput, "/bin/false")
	assert.Contains(t, logOutput, "exit_code=1")
}

func TestExecutor_ErrorLogging_ValidationFailure(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	executor := NewDefaultExecutor(WithLogger(logger))

	// Use a command with invalid path (contains ..)
	cmd := createTestCommand("../invalid/path", []string{})

	_, err := executor.Execute(context.Background(), cmd, map[string]string{}, nil)
	require.Error(t, err)

	// Check that the error log contains the validation failure
	logOutput := buf.String()
	assert.Contains(t, logOutput, "Command validation failed")
}

func TestExecutor_DefaultLogger_UsesSlogDefault(t *testing.T) {
	// Create executor without explicitly setting logger
	// The default behavior is to use slog.Default()
	executor := NewDefaultExecutor()

	cmd := createTestCommand("/bin/echo", []string{"test"})

	// Should execute successfully with default slog logger
	_, err := executor.Execute(context.Background(), cmd, map[string]string{}, nil)
	require.NoError(t, err)

	// Note: Both NewDefaultExecutor() and runner.NewRunner() use slog.Default(),
	// ensuring consistent logging behavior across the application.
}

func TestExecutor_ShellEscapingInLogs(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	executor := NewDefaultExecutor(WithLogger(logger))

	// Test command with special characters that need escaping
	cmd := createTestCommand("/bin/echo", []string{
		"simple",
		"with spaces",
		"with'quote",
		"with$variable",
		"with;semicolon",
	})

	_, err := executor.Execute(context.Background(), cmd, map[string]string{}, nil)
	require.NoError(t, err)

	logOutput := buf.String()

	// Verify the command is properly escaped for copy-paste
	// Note: In the log output, backslashes are escaped (doubled)
	assert.Contains(t, logOutput, "simple")
	assert.Contains(t, logOutput, "'with spaces'")
	// In log output, \ becomes \\ (escaped), so 'with'\''quote' appears as 'with'\\''quote'
	assert.Contains(t, logOutput, "'with'\\\\''quote'")
	assert.Contains(t, logOutput, "'with$variable'")
	assert.Contains(t, logOutput, "'with;semicolon'")
}

func TestExecutor_ErrorLogging_WithStderr(t *testing.T) {
	// Create a buffer to capture log output
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	executor := NewDefaultExecutor(WithLogger(logger))

	// Use a command that writes to stderr and fails
	// sh -c "echo 'error message' >&2; exit 1"
	cmd := createTestCommand("/bin/sh", []string{"-c", "echo 'error message' >&2; exit 1"})

	_, err := executor.Execute(context.Background(), cmd, map[string]string{}, nil)
	require.Error(t, err)

	// Check that the error log includes stderr output
	logOutput := buf.String()
	assert.Contains(t, logOutput, "Command execution failed")
	assert.Contains(t, logOutput, "stderr")
	// Note: stderr content appears in the log structured output
}
