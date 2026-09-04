//go:build test

package executor

import (
	"context"
	"log/slog"
	"os/exec"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/require"
)

// createTestCommandOption configures optional fields on the CommandSpec that
// createTestCommand builds, for tests that need more than a bare cmd/args pair.
type createTestCommandOption func(*runnertypes.CommandSpec)

// withRunAs sets the run-as user and group on the command spec.
func withRunAs(user, group string) createTestCommandOption {
	return func(spec *runnertypes.CommandSpec) {
		spec.RunAsUser = user
		spec.RunAsGroup = group
	}
}

// withOutputSizeLimit sets the command-specific output size limit in bytes.
func withOutputSizeLimit(limit int64) createTestCommandOption {
	return func(spec *runnertypes.CommandSpec) {
		spec.OutputSizeLimit = &limit
	}
}

// createTestCommand creates a RuntimeCommand for testing purposes
func createTestCommand(cmd string, args []string, opts ...createTestCommandOption) *runnertypes.RuntimeCommand {
	spec := &runnertypes.CommandSpec{
		Name: "test_cmd",
		Cmd:  cmd,
		Args: args,
	}
	for _, opt := range opts {
		opt(spec)
	}

	rtCmd, err := runnertypes.NewRuntimeCommand(spec, common.NewUnsetTimeout(), commontestutil.NewUnsetOutputSizeLimit(), "test_group")
	if err != nil {
		panic(err)
	}

	rtCmd.ExpandedCmd = cmd
	rtCmd.ExpandedArgs = args

	return rtCmd
}

func TestExecutor_DebugLogging(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()

	executor := NewDefaultExecutor(WithLogger(logger))

	cmd := createTestCommand("/bin/echo", []string{"hello", "world with spaces"})

	_, err := executor.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.NoError(t, err)

	// The command attribute carries the shell-quoted line, ready for copy-paste.
	rec.RequireRecord(t, slog.LevelDebug, "Executing command").
		AssertAttrs(t, map[string]any{"command": "/bin/echo hello 'world with spaces'"})
}

func TestExecutor_ErrorLogging_CommandNotFound(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()

	executor := NewDefaultExecutor(WithLogger(logger))

	cmd := createTestCommand("/nonexistent/command", []string{})

	_, err := executor.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.Error(t, err)

	// The command path is absolute and non-existent, so it fails at execution time
	rec.RequireRecord(t, slog.LevelError, "Command execution failed").
		AssertAttrs(t, map[string]any{"command": "/nonexistent/command"})
}

func TestExecutor_ErrorLogging_CommandExecutionFailure(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()

	executor := NewDefaultExecutor(WithLogger(logger))

	// Use "false" command which always exits with non-zero status
	falsePath, err := exec.LookPath("false")
	require.NoError(t, err, "false command not found in PATH")
	cmd := createTestCommand(falsePath, []string{})

	_, err = executor.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.Error(t, err)

	rec.RequireRecord(t, slog.LevelError, "Command execution failed").
		AssertAttrs(t, map[string]any{
			"command":   falsePath,
			"exit_code": 1,
		})
}

func TestExecutor_ErrorLogging_ValidationFailure(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()

	executor := NewDefaultExecutor(WithLogger(logger))

	// Use a command with invalid path (contains ..)
	cmd := createTestCommand("../invalid/path", []string{})

	_, err := executor.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.Error(t, err)

	rec.RequireRecord(t, slog.LevelError, "Command validation failed").
		AssertAttrs(t, map[string]any{"command": "../invalid/path"})
}

func TestExecutor_DefaultLogger_UsesSlogDefault(t *testing.T) {
	// Create executor without explicitly setting logger
	// The default behavior is to use slog.Default()
	executor := NewDefaultExecutor()

	cmd := createTestCommand("/bin/echo", []string{"test"})

	// Should execute successfully with default slog logger
	_, err := executor.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.NoError(t, err)

	// Note: Both NewDefaultExecutor() and runner.NewRunner() use slog.Default(),
	// ensuring consistent logging behavior across the application.
}

func TestExecutor_ShellEscapingInLogs(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()

	executor := NewDefaultExecutor(WithLogger(logger))

	// Test command with special characters that need escaping
	cmd := createTestCommand("/bin/echo", []string{
		"simple",
		"with spaces",
		"with'quote",
		"with$variable",
		"with;semicolon",
	})

	_, err := executor.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.NoError(t, err)

	// Verify the command is properly escaped for copy-paste. Asserting on the
	// attribute value rather than the rendered line keeps the expectation in
	// shell-quoting terms, without the handler's own backslash escaping on top.
	rec.RequireRecord(t, slog.LevelDebug, "Executing command").
		AssertAttrs(t, map[string]any{
			"command": `/bin/echo simple 'with spaces' 'with'\''quote' 'with$variable' 'with;semicolon'`,
		})
}

func TestExecutor_ErrorLogging_WithStderr(t *testing.T) {
	logger, rec := tu.NewRecordingLogger()

	executor := NewDefaultExecutor(WithLogger(logger))

	// Use a command that writes to stderr and fails
	// sh -c "echo 'error message' >&2; exit 1"
	cmd := createTestCommand("/bin/sh", []string{"-c", "echo 'error message' >&2; exit 1"})

	_, err := executor.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.Error(t, err)

	// The stderr the command wrote is carried in its own attribute.
	rec.RequireRecord(t, slog.LevelError, "Command execution failed").
		AssertAttrs(t, map[string]any{"stderr": "error message\n"})
}
