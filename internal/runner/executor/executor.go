// Package executor provides the core functionality for executing commands
// in a safe and controlled manner. It includes interfaces and implementations
// for command execution, output handling, and environment management.
package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Error definitions
var (
	ErrEmptyCommand                  = errors.New("command cannot be empty")
	ErrDirNotExists                  = errors.New("directory does not exist")
	ErrInvalidPath                   = errors.New("invalid command path")
	ErrNoPrivilegeManager            = errors.New("privileged execution requested but no privilege manager available")
	ErrUserGroupPrivilegeUnsupported = errors.New("user/group privilege changes are not supported")
	ErrPrivilegedCmdSecurity         = errors.New("privileged command failed security validation")
)

// DefaultExecutor is the default implementation of CommandExecutor
type DefaultExecutor struct {
	FS          FileSystem
	PrivMgr     runnertypes.PrivilegeManager // Optional privilege manager for privileged commands
	AuditLogger *audit.Logger                // Optional audit logger for privileged operations
	Logger      *slog.Logger                 // Optional logger for command execution logging
}

// Option is a functional option for configuring DefaultExecutor
type Option func(*DefaultExecutor)

// WithPrivilegeManager sets the privilege manager for the executor
func WithPrivilegeManager(privMgr runnertypes.PrivilegeManager) Option {
	return func(e *DefaultExecutor) {
		e.PrivMgr = privMgr
	}
}

// WithFileSystem sets the file system for the executor
func WithFileSystem(fs FileSystem) Option {
	return func(e *DefaultExecutor) {
		e.FS = fs
	}
}

// WithAuditLogger sets the audit logger for the executor
func WithAuditLogger(auditLogger *audit.Logger) Option {
	return func(e *DefaultExecutor) {
		e.AuditLogger = auditLogger
	}
}

// WithLogger sets the logger for the executor
func WithLogger(logger *slog.Logger) Option {
	return func(e *DefaultExecutor) {
		e.Logger = logger
	}
}

// NewDefaultExecutor creates a new default command executor
func NewDefaultExecutor(opts ...Option) CommandExecutor {
	e := &DefaultExecutor{
		FS:     &osFileSystem{},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), // Default to no-op logger (discards all logs)
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Execute implements the CommandExecutor interface
func (e *DefaultExecutor) Execute(ctx context.Context, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter) (*Result, error) {
	// Note: outputWriter lifecycle is managed by the caller.
	// The caller is responsible for calling Close() when done.
	// This executor will NOT close the outputWriter.

	if cmd.HasUserGroupSpecification() {
		return e.executeWithUserGroup(ctx, cmd, envVars, outputWriter)
	}
	return e.executeNormal(ctx, cmd, envVars, outputWriter)
}

// executeWithUserGroup handles command execution with user/group privilege changes with audit logging and metrics
func (e *DefaultExecutor) executeWithUserGroup(ctx context.Context, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter) (*Result, error) {
	startTime := time.Now()
	var metrics audit.PrivilegeMetrics

	// Pre-execution validation
	if e.PrivMgr == nil {
		e.Logger.Error("No privilege manager available", "error", ErrNoPrivilegeManager)
		return nil, ErrNoPrivilegeManager
	}

	if !e.PrivMgr.IsPrivilegedExecutionSupported() {
		e.Logger.Error("User/group privilege changes are not supported", "error", ErrUserGroupPrivilegeUnsupported)
		return nil, ErrUserGroupPrivilegeUnsupported
	}

	// Validate the command before any privilege changes
	if err := e.Validate(cmd); err != nil {
		e.Logger.Error("Command validation failed", "error", err, "command", cmd.ExpandedCmd)
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	// Additional security validation for privileged commands BEFORE path resolution
	// This ensures the original command in the config file uses absolute paths
	if err := e.validatePrivilegedCommand(cmd); err != nil {
		e.Logger.Error("Privileged command security validation failed", "error", err, "command", cmd.ExpandedCmd)
		return nil, fmt.Errorf("privileged command security validation failed: %w", err)
	}

	if cmd.ExpandedCmd == "" {
		e.Logger.Error("Empty command", "error", ErrEmptyCommand)
		return nil, ErrEmptyCommand
	}

	// Create elevation context for user/group execution
	executionCtx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationUserGroupExecution,
		CommandName: cmd.Name(),
		FilePath:    cmd.ExpandedCmd,
		RunAsUser:   cmd.RunAsUser(),
		RunAsGroup:  cmd.RunAsGroup(),
	}

	var result *Result
	privilegeStart := time.Now()
	err := e.PrivMgr.WithPrivileges(executionCtx, func() error {
		var execErr error
		result, execErr = e.executeCommandWithPath(ctx, cmd.ExpandedCmd, cmd, envVars, outputWriter)
		return execErr
	})
	privilegeDuration := time.Since(privilegeStart)
	metrics.ElevationCount++
	metrics.TotalDuration += privilegeDuration

	if err != nil {
		e.Logger.Error("User/group privilege execution failed", "error", err, "command", cmd.ExpandedCmd, "user", cmd.RunAsUser(), "group", cmd.RunAsGroup())
		return nil, fmt.Errorf("user/group privilege execution failed: %w", err)
	}

	// Audit logging
	if e.AuditLogger != nil {
		executionDuration := time.Since(startTime)
		auditResult := &audit.ExecutionResult{
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
		}
		e.AuditLogger.LogUserGroupExecution(ctx, cmd, auditResult, executionDuration, metrics)
	}

	return result, nil
}

// executeNormal handles normal (non-privileged) command execution
func (e *DefaultExecutor) executeNormal(ctx context.Context, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter) (*Result, error) {
	// Validate the command before execution
	if err := e.Validate(cmd); err != nil {
		e.Logger.Error("Command validation failed", "error", err, "command", cmd.ExpandedCmd)
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	if cmd.ExpandedCmd == "" {
		e.Logger.Error("Empty command", "error", ErrEmptyCommand)
		return nil, ErrEmptyCommand
	}

	// Resolve the command path
	path, lookErr := exec.LookPath(cmd.ExpandedCmd)
	if lookErr != nil {
		e.Logger.Error("Failed to find command", "error", lookErr, "command", cmd.ExpandedCmd)
		return nil, fmt.Errorf("failed to find command %q: %w", cmd.ExpandedCmd, lookErr)
	}

	return e.executeCommandWithPath(ctx, path, cmd, envVars, outputWriter)
}

// executeCommandWithPath executes a command with the given resolved path
func (e *DefaultExecutor) executeCommandWithPath(ctx context.Context, path string, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter) (*Result, error) {
	// Log the command being executed at DEBUG level
	cmdLine := FormatCommandForLog(path, cmd.ExpandedArgs)
	e.Logger.Debug("Executing command", "command", cmdLine)

	// Create the command with the resolved path
	// #nosec G204 - The command and arguments are validated before execution with e.Validate()
	execCmd := exec.CommandContext(ctx, path, cmd.ExpandedArgs...)

	// Set up working directory
	if cmd.EffectiveWorkDir != "" {
		execCmd.Dir = cmd.EffectiveWorkDir
	}

	// Set up environment variables
	// Only use the filtered environment variables provided in envVars
	// This ensures allowlist filtering is properly enforced
	execCmd.Env = make([]string, 0, len(envVars))
	for k, v := range envVars {
		execCmd.Env = append(execCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up output capture
	var stdout, stderr []byte
	var cmdErr error

	if outputWriter != nil {
		// Create buffered wrappers that both capture output and write to OutputWriter
		stdoutWrapper := &outputWrapper{writer: outputWriter, stream: StdoutStream}
		stderrWrapper := &outputWrapper{writer: outputWriter, stream: StderrStream}

		execCmd.Stdout = stdoutWrapper
		execCmd.Stderr = stderrWrapper

		// Run the command
		e.Logger.Debug("Starting command execution", "command", cmdLine, "working_dir", execCmd.Dir, "env_count", len(execCmd.Env))
		cmdErr = execCmd.Run()
		e.Logger.Debug("Command execution completed", "command", cmdLine, "error", cmdErr)

		// Get the captured output
		stdout = stdoutWrapper.GetBuffer()
		stderr = stderrWrapper.GetBuffer()
	} else {
		// Otherwise, capture output in memory
		e.Logger.Debug("Starting command execution (memory capture)", "command", cmdLine, "working_dir", execCmd.Dir, "env_count", len(execCmd.Env))
		stdout, cmdErr = execCmd.Output()
		e.Logger.Debug("Command execution completed (memory capture)", "command", cmdLine, "error", cmdErr)
		if exitErr, ok := cmdErr.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
		}
	}

	// Prepare the result
	result := &Result{
		Stdout: string(stdout),
		Stderr: string(stderr),
	}
	if execCmd.ProcessState != nil {
		result.ExitCode = execCmd.ProcessState.ExitCode()
	} else {
		result.ExitCode = ExitCodeUnknown // Use constant for unknown exit code
	}

	if cmdErr != nil {
		e.Logger.Error("Command execution failed",
			"error", cmdErr,
			"command", cmdLine,
			"exit_code", result.ExitCode,
			"stderr", string(stderr))
		return result, fmt.Errorf("command execution failed: %w", cmdErr)
	}

	return result, nil
}

// Validate implements the CommandExecutor interface
func (e *DefaultExecutor) Validate(cmd *runnertypes.RuntimeCommand) error {
	if cmd.ExpandedCmd == "" {
		return ErrEmptyCommand
	}

	// Validate command path to prevent command injection and ensure proper format
	if !filepath.IsLocal(cmd.ExpandedCmd) && !filepath.IsAbs(cmd.ExpandedCmd) {
		return fmt.Errorf("%w: command path must be local or absolute: %s", ErrInvalidPath, cmd.ExpandedCmd)
	}
	if filepath.Clean(cmd.ExpandedCmd) != cmd.ExpandedCmd {
		return fmt.Errorf("%w: command path contains relative path components ('.' or '..'): %s", ErrInvalidPath, cmd.ExpandedCmd)
	}

	// Check if working directory exists and is accessible
	if cmd.EffectiveWorkDir != "" {
		exists, err := e.FS.FileExists(cmd.EffectiveWorkDir)
		if err != nil {
			return fmt.Errorf("failed to check directory %s: %w", cmd.EffectiveWorkDir, err)
		}
		if !exists {
			return fmt.Errorf("working directory %q does not exist: %w", cmd.EffectiveWorkDir, ErrDirNotExists)
		}
	}

	return nil
}

// osFileSystem implements FileSystem using the standard os package
type osFileSystem struct{}

func (fs *osFileSystem) CreateTempDir(dir, prefix string) (string, error) {
	return os.MkdirTemp(dir, prefix)
}

func (fs *osFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

func (fs *osFileSystem) FileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// outputWrapper is an io.Writer that both captures output in a buffer
// and writes to an OutputWriter with a specific stream name
type outputWrapper struct {
	writer OutputWriter
	stream OutputStream
	buffer bytes.Buffer
	mu     sync.Mutex
}

func (w *outputWrapper) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write to buffer for capturing
	w.buffer.Write(p)

	// Also write to the OutputWriter
	if w.writer != nil {
		if err := w.writer.Write(w.stream, p); err != nil {
			return 0, err
		}
	}

	return len(p), nil
}

func (w *outputWrapper) GetBuffer() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.Bytes()
}

// validatePrivilegedCommand performs additional security checks specifically for privileged commands
// This adds an extra layer of security validation beyond the basic validation
func (e *DefaultExecutor) validatePrivilegedCommand(cmd *runnertypes.RuntimeCommand) error {
	if cmd.ExpandedCmd == "" {
		return ErrEmptyCommand
	}

	// Enforce absolute paths for privileged commands
	if !filepath.IsAbs(cmd.ExpandedCmd) {
		return fmt.Errorf("%w: privileged commands must use absolute paths: %s", ErrPrivilegedCmdSecurity, cmd.ExpandedCmd)
	}

	// Ensure working directory is also absolute for privileged commands
	if cmd.EffectiveWorkDir != "" && !filepath.IsAbs(cmd.EffectiveWorkDir) {
		return fmt.Errorf("%w: privileged commands must use absolute working directory paths: %s", ErrPrivilegedCmdSecurity, cmd.EffectiveWorkDir)
	}

	// Additional validation could include:
	// 1. Check for suspicious or potentially dangerous arguments
	// 2. Allowlist checking for permitted privileged commands
	// 3. Check if command is in system directories like /bin, /usr/bin, etc.
	// 4. Verify that the command binary has proper permissions
	return nil
}
