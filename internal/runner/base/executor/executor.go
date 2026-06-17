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
	"syscall"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// stagedExecMode is the permission of a staged command copy: owner read+execute
// only, with write withheld so the verified copy cannot be modified before exec.
const stagedExecMode = 0o500

// ErrPrivilegeLeak is returned when effective UID/GID do not match real UID/GID after execution.
var ErrPrivilegeLeak = errors.New("privilege leak detected")

// Error definitions
var (
	ErrEmptyCommand                  = errors.New("command cannot be empty")
	ErrDirNotExists                  = errors.New("directory does not exist")
	ErrInvalidPath                   = errors.New("invalid command path")
	ErrPathNotAbsolute               = errors.New("command path must be absolute")
	ErrNoPrivilegeManager            = errors.New("privileged execution requested but no privilege manager available")
	ErrUserGroupPrivilegeUnsupported = errors.New("user/group privilege changes are not supported")
	ErrPrivilegedCmdSecurity         = errors.New("privileged command failed security validation")
	ErrNoVerifiedFD                  = errors.New("no verified file descriptor available for fd-bound execution")
	ErrFdExecUnsupported             = errors.New("fd-bound execution is not supported on this platform")
)

// DefaultExecutor is the default implementation of CommandExecutor
type DefaultExecutor struct {
	FS              FileSystem
	PrivMgr         runnertypes.PrivilegeManager // Optional privilege manager for privileged commands
	AuditLogger     *audit.Logger                // Optional audit logger for privileged operations
	Logger          *slog.Logger                 // Optional logger for command execution logging
	osExit          func(code int)               // injectable for testing; defaults to os.Exit
	identityChecker func() error                 // injectable for testing; defaults to defaultIdentityChecker
	fdExecDisabled  bool                         // injectable for testing; forces the staging fallback even on Linux
}

// Option is a functional option for configuring DefaultExecutor
type Option func(*DefaultExecutor)

// WithPrivilegeManager sets the privilege manager for the executor
func WithPrivilegeManager(privMgr runnertypes.PrivilegeManager) Option {
	return func(e *DefaultExecutor) {
		e.PrivMgr = privMgr
	}
}

// WithAuditLogger sets the audit logger for the executor
func WithAuditLogger(auditLogger *audit.Logger) Option {
	return func(e *DefaultExecutor) {
		e.AuditLogger = auditLogger
	}
}

// NewDefaultExecutor creates a new default command executor
// By default, it uses slog.Default() for logging, ensuring all execution logs
// are visible through the application's default logger.
func NewDefaultExecutor(opts ...Option) CommandExecutor {
	e := &DefaultExecutor{
		FS:              &osFileSystem{},
		Logger:          slog.Default(),
		osExit:          os.Exit,
		identityChecker: defaultIdentityChecker,
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Execute implements the CommandExecutor interface.
//
// plan is the verified command plan produced by the risk evaluator. When it
// carries a verified file descriptor (plan.Identity.FD), the executor binds the
// executed inode to that descriptor (fd-bound execution, the inode the evaluator
// verified) instead of re-resolving the path, closing the TOCTOU window between
// verification and exec. The plan's descriptors are owned by the caller, which
// must Close the plan; the executor only duplicates/copies from them.
//
// Scope: this binds the executed inode only. argv and env are still taken from
// cmd.ExpandedArgs and envVars (the plan's ResolvedArgv/ResolvedEnv are not yet
// consumed); binding those, and the inner artifacts of indirect-execution
// wrappers, is deferred (see architecture section 5.2).
func (e *DefaultExecutor) Execute(ctx context.Context, plan *risktypes.VerifiedCommandPlan, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter) (*Result, error) {
	// Note: outputWriter lifecycle is managed by the caller.
	// The caller is responsible for calling Close() when done.
	// This executor will NOT close the outputWriter.

	var result *Result
	var err error
	if cmd.HasUserGroupSpecification() {
		result, err = e.executeWithUserGroup(ctx, plan, cmd, envVars, outputWriter)
	} else {
		result, err = e.executeNormal(ctx, plan, cmd, envVars, outputWriter)
	}

	// Security invariant: EUID must equal UID and EGID must equal GID after every execution.
	// This acts as a defense-in-depth check independent of the privilege manager's own
	// restoration logic. If a bug causes privilege escalation to leak into the next command,
	// we detect it here and terminate immediately rather than continue with wrong identity.
	if checkErr := e.identityChecker(); checkErr != nil {
		e.Logger.Error("CRITICAL SECURITY FAILURE: privilege leak detected after command execution",
			"error", checkErr,
			"command", cmd.Name(),
			"pid", os.Getpid())
		fmt.Fprintf(os.Stderr, "FATAL: %v\n", checkErr)
		e.osExit(1)
	}

	return result, err
}

// executeWithUserGroup handles command execution with user/group privilege changes with audit logging and metrics
func (e *DefaultExecutor) executeWithUserGroup(ctx context.Context, plan *risktypes.VerifiedCommandPlan, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter) (*Result, error) {
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
	e.Logger.Debug("Calling WithPrivileges for user/group execution", "command", cmd.Name(), "user", cmd.RunAsUser(), "group", cmd.RunAsGroup())
	err := e.PrivMgr.WithPrivileges(executionCtx, func() error {
		var execErr error
		result, execErr = e.executeCommandWithPath(ctx, plan, cmd.ExpandedCmd, cmd, envVars, outputWriter)
		return execErr
	})
	privilegeDuration := time.Since(privilegeStart)
	metrics.ElevationCount++
	metrics.TotalDuration += privilegeDuration

	if err != nil {
		e.Logger.Error("User/group privilege execution failed", "error", err, "command", cmd.ExpandedCmd, "user", cmd.RunAsUser(), "group", cmd.RunAsGroup())
		return result, fmt.Errorf("user/group privilege execution failed: %w", err)
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
func (e *DefaultExecutor) executeNormal(ctx context.Context, plan *risktypes.VerifiedCommandPlan, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter) (*Result, error) {
	// Validate the command before execution
	if err := e.Validate(cmd); err != nil {
		e.Logger.Error("Command validation failed", "error", err, "command", cmd.ExpandedCmd)
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	if cmd.ExpandedCmd == "" {
		e.Logger.Error("Empty command", "error", ErrEmptyCommand)
		return nil, ErrEmptyCommand
	}

	// cmd.ExpandedCmd should already be an absolute, symlink-resolved path
	// (resolved by verification.PathResolver.ResolvePath() in group_executor).
	// No need for exec.LookPath() here as the path is already resolved.
	if !filepath.IsAbs(cmd.ExpandedCmd) {
		e.Logger.Error("Command path is not absolute", "command", cmd.ExpandedCmd)
		return nil, fmt.Errorf("%w: %s", ErrPathNotAbsolute, cmd.ExpandedCmd)
	}

	return e.executeCommandWithPath(ctx, plan, cmd.ExpandedCmd, cmd, envVars, outputWriter)
}

// executeCommandWithPath executes a command with the given resolved path.
//
// When plan carries a verified file descriptor, execution is bound to that
// descriptor (fd-bound exec on Linux, or read-only staging copied from the
// descriptor as a fallback) so the executed inode is exactly the one the
// evaluator verified. Without a verified descriptor the already-resolved path is
// executed directly (no re-resolution); the evaluator's identity gate denies
// unverified binaries before they reach an allowed plan, so this branch does not
// weaken the production guarantee.
func (e *DefaultExecutor) executeCommandWithPath(ctx context.Context, plan *risktypes.VerifiedCommandPlan, path string, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter) (*Result, error) {
	// Log the command being executed at DEBUG level
	cmdLine := FormatCommandForLog(path, cmd.ExpandedArgs)
	e.Logger.Debug("Executing command",
		"command", cmdLine,
		"path", path,
		"work_dir", cmd.EffectiveWorkDir,
		"work_dir_len", len(cmd.EffectiveWorkDir))

	// Bind execution to the verified descriptor when one is available.
	// #nosec G204 - The command and arguments are validated before execution with e.Validate()
	execCmd, cleanup, err := e.prepareExecCommand(ctx, plan, path, cmd.ExpandedArgs)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Set up stdin to null device for security and stability:
	// 1. Security: Prevents child processes from reading unexpected input from stdin
	// 2. Stability: Prevents errors in commands that try to allocate a pseudo-TTY when stdin is nil
	//    (e.g., docker-compose exec can fail with "exit status 255" if stdin is not configured)
	// 3. Best practice: Batch processing tools should explicitly control stdin rather than inheriting it
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return nil, fmt.Errorf("failed to open null device for stdin: %w", err)
	}
	defer func() {
		if closeErr := devNull.Close(); closeErr != nil {
			e.Logger.Warn("Failed to close null device", "error", closeErr)
		}
	}()
	execCmd.Stdin = devNull

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
		cmdErr = execCmd.Run()

		// Get the captured output
		stdout = stdoutWrapper.GetBuffer()
		stderr = stderrWrapper.GetBuffer()

		// Check if there was a write error (e.g., size limit exceeded)
		// If so, prefer that error over the broken pipe error from the command
		// This is important because when the writer returns an error, the command
		// receives SIGPIPE and exits with "signal: broken pipe", which masks the
		// real cause of the failure (e.g., output size limit exceeded)
		if writeErr := stdoutWrapper.GetWriteError(); writeErr != nil {
			cmdErr = writeErr
		} else if writeErr := stderrWrapper.GetWriteError(); writeErr != nil {
			cmdErr = writeErr
		}
	} else {
		// Otherwise, capture output in memory
		stdout, cmdErr = execCmd.Output()
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

// prepareExecCommand builds the *exec.Cmd to run, binding it to the verified
// file descriptor when the plan supplies one. It returns a cleanup function the
// caller must defer; cleanup closes any duplicated descriptor and removes any
// staged copy, so descriptors are not leaked even when exec.Cmd.Start fails.
//
// Selection order when a verified descriptor is present:
//  1. fd-bound exec via /proc/self/fd (Linux) — the kernel resolves the
//     descriptor to the verified inode regardless of later path swaps.
//  2. read-only staging — copy the verified inode out of the held descriptor to
//     a private file and exec that copy (non-Linux, or when fd exec is disabled
//     for tests).
//
// Without a verified descriptor it execs the already-resolved path directly (no
// re-resolution).
func (e *DefaultExecutor) prepareExecCommand(ctx context.Context, plan *risktypes.VerifiedCommandPlan, path string, args []string) (*exec.Cmd, func(), error) {
	noop := func() {}

	var identity *risktypes.VerifiedIdentity
	if plan != nil {
		identity = plan.Identity
	}

	if identity != nil && identity.FD != nil {
		if !e.fdExecDisabled && fdExecSupported() {
			childPath, extraFile, err := fdExecExtraFile(identity)
			if err != nil {
				return nil, nil, err
			}
			// #nosec G204 - childPath is /proc/self/fd/<n> bound to the verified inode.
			execCmd := exec.CommandContext(ctx, childPath, args...)
			execCmd.Args[0] = path // present the resolved path as argv[0] to the child
			execCmd.ExtraFiles = []*os.File{extraFile}
			return execCmd, func() {
				if closeErr := extraFile.Close(); closeErr != nil {
					e.Logger.Warn("Failed to close duplicated verified fd", "error", closeErr)
				}
			}, nil
		}

		stagedPath, stagingCleanup, err := e.stageFromFD(identity)
		if err != nil {
			return nil, nil, err
		}
		// #nosec G204 - stagedPath is a private copy of the verified inode.
		execCmd := exec.CommandContext(ctx, stagedPath, args...)
		execCmd.Args[0] = path
		return execCmd, stagingCleanup, nil
	}

	// #nosec G204 - The command and arguments are validated before execution with e.Validate()
	return exec.CommandContext(ctx, path, args...), noop, nil
}

// stageFromFD copies the verified inode out of the held descriptor into a private
// read-only file and returns its path plus a cleanup function. The bytes are read
// from the verified descriptor (not re-opened from the path), so a swapped path
// cannot substitute different content. The staged copy lives in a 0700 temp
// directory and is created 0500 (read+execute, no write).
func (e *DefaultExecutor) stageFromFD(identity *risktypes.VerifiedIdentity) (string, func(), error) {
	dir, err := os.MkdirTemp("", "scr-stage-")
	if err != nil {
		return "", nil, fmt.Errorf("failed to create staging directory: %w", err)
	}
	cleanup := func() {
		if rmErr := os.RemoveAll(dir); rmErr != nil {
			e.Logger.Warn("Failed to remove staging directory", "error", rmErr, "dir", dir)
		}
	}

	// Duplicate the verified descriptor so this function owns a separate closable
	// handle (the original stays owned by VerifiedFD). The duplicate shares the
	// same open file description -- and therefore the same file offset -- as the
	// original, so the copy below reads via ReadAt (pread) over a SectionReader,
	// which never moves that shared offset.
	dup, err := syscall.Dup(identity.FD.Fd())
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to duplicate verified fd for staging: %w", err)
	}
	src := os.NewFile(uintptr(dup), identity.ResolvedPath) // #nosec G115 -- dup is a valid non-negative fd from syscall.Dup; int->uintptr cannot overflow
	if src == nil {
		_ = syscall.Close(dup)
		cleanup()
		return "", nil, ErrNoVerifiedFD
	}
	defer func() {
		if closeErr := src.Close(); closeErr != nil {
			e.Logger.Warn("Failed to close staging source fd", "error", closeErr)
		}
	}()
	info, err := src.Stat()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to stat verified fd for staging: %w", err)
	}

	// Preserve the original basename: multi-call binaries (e.g. busybox/coreutils)
	// select their applet from the executable name, and tools may inspect
	// /proc/self/exe, so the staged copy must keep the verified command's name.
	name := filepath.Base(identity.ResolvedPath)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "command"
	}
	stagedPath := filepath.Join(dir, name)
	// #nosec G304 G302 - stagedPath is a freshly created file (O_EXCL) inside our
	// own 0700 MkdirTemp directory; the basename derives from the already-verified
	// resolved path, not untrusted input. The execute bit (0500) is required to
	// exec the staged copy and write is intentionally withheld.
	dst, err := os.OpenFile(stagedPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, stagedExecMode)
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to create staged command file: %w", err)
	}
	if _, err := io.Copy(dst, io.NewSectionReader(src, 0, info.Size())); err != nil {
		_ = dst.Close()
		cleanup()
		return "", nil, fmt.Errorf("failed to stage verified command: %w", err)
	}
	if err := dst.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("failed to close staged command file: %w", err)
	}

	return stagedPath, cleanup, nil
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
			return fmt.Errorf("%w: %s", ErrDirNotExists, cmd.EffectiveWorkDir)
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
	writer   OutputWriter
	stream   OutputStream
	buffer   bytes.Buffer
	writeErr error // Stores the first write error encountered
	mu       sync.Mutex
}

func (w *outputWrapper) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write to buffer for capturing
	w.buffer.Write(p)

	// Also write to the OutputWriter
	if w.writer != nil {
		if err := w.writer.Write(w.stream, p); err != nil {
			// Store the first error encountered for later retrieval
			if w.writeErr == nil {
				w.writeErr = err
			}
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

func (w *outputWrapper) GetWriteError() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.writeErr
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
