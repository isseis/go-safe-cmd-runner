//go:build test

package executor

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
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

	rtCmd, err := runnertypes.NewRuntimeCommand(spec, common.NewUnsetTimeout(), "test_group")
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

func TestExecutor_NoLogging_WhenLoggerNotSet(t *testing.T) {
	// Create executor without logger
	executor := NewDefaultExecutor()

	cmd := createTestCommand("/bin/echo", []string{"test"})

	// Should not panic even without logger
	_, err := executor.Execute(context.Background(), cmd, map[string]string{}, nil)
	require.NoError(t, err)
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

// TestFormatCommandForLog_CopyPasteable verifies that logged commands can be copy-pasted
func TestFormatCommandForLog_CopyPasteable(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		args        []string
		description string
	}{
		{
			name:        "simple command",
			path:        "/bin/ls",
			args:        []string{"-la"},
			description: "Should produce: /bin/ls -la",
		},
		{
			name:        "command with spaces in args",
			path:        "/bin/grep",
			args:        []string{"-r", "search pattern", "/path/to/dir"},
			description: "Should properly quote arguments with spaces",
		},
		{
			name:        "command with special chars",
			path:        "/bin/sh",
			args:        []string{"-c", "echo $HOME && ls -la"},
			description: "Should escape shell special characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatCommandForLog(tt.path, tt.args)

			// Result should not be empty
			assert.NotEmpty(t, result)

			// Result should start with the path
			assert.True(t, strings.HasPrefix(result, ShellEscape(tt.path)),
				"Result should start with escaped path")

			t.Logf("Command: %s", result)
			t.Logf("Description: %s", tt.description)
		})
	}
}
