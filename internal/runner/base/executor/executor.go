// Package executor provides the core functionality for executing commands
// in a safe and controlled manner. It includes interfaces and implementations
// for command execution, output handling, and environment management.
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// stagedExecMode is the permission of a staged command copy: owner
// read+execute, group read+execute, others none. The staging directory and
// file remain owned by root/parent (see stageFromFD); only the group is
// chgrp'd to the run-as gid, so the target user can execute the staged copy
// via group permission without being able to chown/chmod it (which owner
// rights would allow) and without exposing it to other users sharing the
// staging parent directory (e.g. /tmp).
const stagedExecMode = 0o550

// nilWriterStderrLimit bounds how many bytes of stderr are retained for runs
// without an OutputWriter, matching the 32 KiB prefix/suffix limit os/exec
// applies to Cmd.Output's stderr.
const nilWriterStderrLimit = 32 << 10

// defaultKillGraceDelay is the production default for DefaultExecutor.killGraceDelay.
const defaultKillGraceDelay = 5 * time.Second

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
	runAsResolver   risktypes.RunAsResolver      // injectable for testing; defaults to risktypes.ResolveRunAsIdent
	fdExecDisabled  bool                         // injectable for testing; forces the staging fallback even on Linux

	// killGraceDelay bounds two distinct waits that share the same value but
	// must not be confused:
	//  1. How long superviseCommand waits for execCmd.Wait() to return after a
	//     kill. Exceeding it means the child could not be reaped and yields
	//     ErrChildNotReaped.
	//  2. How long superviseCommand waits for the output pump to drain after a
	//     kill. Once Stdout/Stderr are *os.File (see output_pump.go), Wait()
	//     does not wait for copy goroutines, so it is a grandchild holding the
	//     pipe's write end -- not a stuck child -- that can stretch (2)
	//     without stretching (1). Exceeding (2) is "could not finish reading
	//     the output", which is not an error: the exit code already came from
	//     Wait().
	killGraceDelay time.Duration

	// waitFn replaces execCmd.Wait() in the wait goroutine when non-nil;
	// injectable for testing. ErrChildNotReaped can only be exercised by a
	// Wait() that never returns, and a real child killed with SIGKILL is
	// always reaped, so this is the only way to hit that path deterministically
	// (giving a grandchild the pipe's write end does not: Wait() does not wait
	// on it -- see killGraceDelay above).
	waitFn func(*exec.Cmd) error
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
		runAsResolver:   risktypes.ResolveRunAsIdent,
		killGraceDelay:  defaultKillGraceDelay,
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

// executeWithUserGroup handles command execution with user/group privilege changes with audit logging and metrics.
// It resolves the run-as identity and sets SysProcAttr.Credential on the child
// process so the kernel sets uid/gid/supplementary groups atomically at execve
// time. On resolution failure the command is not executed and an error is
// returned (fail-closed).
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

	// Resolve the run-as identity before privilege escalation. Delegates to
	// ResolveRunAsIdentStrict which handles nil-resolver fallback, error
	// wrapping, and Groups==nil detection in one call.
	resolvedIdent, err := risktypes.ResolveRunAsIdentStrict(e.runAsResolver, risktypes.OriginalExecutionIdentity(), cmd.RunAsUser(), cmd.RunAsGroup())
	if err != nil {
		e.Logger.Error("Failed to resolve run-as identity",
			"error", err,
			"user", cmd.RunAsUser(),
			"group", cmd.RunAsGroup())
		return nil, err
	}

	// Build the Credential for the child process. NoSetGroups: false ensures
	// supplementary groups are reset to the target user's groups.
	cred := &syscall.Credential{
		Uid:         resolvedIdent.UID,
		Gid:         resolvedIdent.GID,
		Groups:      resolvedIdent.Groups,
		NoSetGroups: false,
	}

	// Create elevation context for user/group execution
	executionCtx := runnertypes.ElevationContext{
		Operation:   runnertypes.OperationUserGroupExecution,
		CommandName: cmd.Name(),
		FilePath:    cmd.ExpandedCmd,
		RunAsUser:   cmd.RunAsUser(),
		RunAsGroup:  cmd.RunAsGroup(),
	}

	// pc is populated inside the WithPrivileges closure below. This phase
	// still has WithPrivileges wrap prepareCommand, startPrepared and
	// superviseCommand together (narrowing the window to startPrepared alone
	// is deferred to the cancellation work), so it is the only place the
	// non-fatal warnings collected on pc can be logged: logging from inside
	// the closure would run at euid 0.
	var pc *preparedCommand
	var result *Result
	privilegeStart := time.Now()
	e.Logger.Debug("Calling WithPrivileges for user/group execution", "command", cmd.Name(), "user", cmd.RunAsUser(), "group", cmd.RunAsGroup())
	err = e.PrivMgr.WithPrivileges(executionCtx, func() error {
		var prepErr error
		pc, prepErr = e.prepareCommand(ctx, plan, cmd.ExpandedCmd, cmd, envVars, outputWriter, cred)
		if prepErr != nil {
			return prepErr
		}
		var runErr error
		result, runErr = e.runCommand(ctx, pc)
		return runErr
	})
	privilegeDuration := time.Since(privilegeStart)
	metrics.ElevationCount++
	metrics.TotalDuration += privilegeDuration
	metrics.ByOperation = map[runnertypes.Operation]time.Duration{
		runnertypes.OperationUserGroupExecution: privilegeDuration,
	}

	e.logDeferredWarnings(pc)

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

	pc, err := e.prepareCommand(ctx, plan, cmd.ExpandedCmd, cmd, envVars, outputWriter, nil)
	if err != nil {
		// prepareCommand returns a non-nil pc on failure too, carrying
		// whatever its own release could not do cleanly.
		e.logDeferredWarnings(pc)
		return nil, err
	}
	result, err := e.runCommand(ctx, pc)
	e.logDeferredWarnings(pc)
	return result, err
}

// logDeferredWarnings logs pc's non-fatal resource-release failures, if any:
// the staging warnings and release()'s failures on devNull, verifiedFD and
// the output pump. Pre-refactor, each of these was logged individually at its
// own close site; decomposing execution into prepare/start/supervise moved
// all of them behind preparedCommand.release(), which may run inside the
// privilege window, so they are recorded as values there instead and logged
// here. Callers must invoke logDeferredWarnings only once the privilege
// window (if any) that produced pc has closed: nothing inside a window may
// log, since a slog handler is free to open a file and would do so at euid 0.
// pc may be nil when the caller never reached prepareCommand, in which case
// there is nothing to log.
func (e *DefaultExecutor) logDeferredWarnings(pc *preparedCommand) {
	if pc == nil {
		return
	}
	if pc.stagingWarn != nil {
		e.Logger.Warn("Failed to close staging source descriptor", "error", pc.stagingWarn)
	}
	if pc.stagingCleanupErr != nil {
		e.Logger.Warn("Failed to remove staging directory", "error", pc.stagingCleanupErr)
	}
	if pc.devNullCloseErr != nil {
		e.Logger.Warn("Failed to close null device", "error", pc.devNullCloseErr)
	}
	if pc.verifiedFDCloseErr != nil {
		e.Logger.Warn("Failed to close duplicated verified fd", "error", pc.verifiedFDCloseErr)
	}
	if pc.pumpReleaseErr != nil {
		e.Logger.Warn("Failed to release output pump", "error", pc.pumpReleaseErr)
	}
}

// applyCredential sets execCmd.SysProcAttr.Credential to cred when cred is
// non-nil, so the kernel sets uid/gid/supplementary groups atomically at
// execve time. Extracted as its own function so the wiring is unit-testable
// without actually running a command.
func applyCredential(execCmd *exec.Cmd, cred *syscall.Credential) {
	if cred == nil {
		return
	}
	if execCmd.SysProcAttr == nil {
		execCmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	execCmd.SysProcAttr.Credential = cred
}

// stagingDirMode is the permission of the staging directory: owner
// read+write+execute, group execute only (to traverse into it), others none.
// The directory stays owned by root/parent; only the group is chgrp'd to the
// run-as gid (see stageFromFD), so the target user can traverse it via group
// permission but cannot replace/rename the directory contents (which owner
// write rights would allow), closing the TOCTOU directory-replacement gap.
const stagingDirMode = 0o710

// stageFromFD copies the verified inode out of the held descriptor into a private
// read-only file and returns its path plus a cleanup function. The bytes are read
// from the verified descriptor (not re-opened from the path), so a swapped path
// cannot substitute different content.
//
// stageFromFD is called from inside the privilege window (prepareCommand
// still runs there in this phase; see command_lifecycle.go), so it must not
// log: a slog handler is free to open a file, and it would do so at euid 0.
// Its two failure modes that are not returned as err are carried out instead:
//   - the cleanup function returns the os.RemoveAll error rather than logging
//     it, so the caller can log once the window has closed. It also writes a
//     single WARNING line directly to os.Stderr (bypassing the redaction
//     handler, so it must never carry secret-bearing values) because a
//     restore failure elsewhere can end the process via emergencyShutdown
//     before the caller's window-closed logging point is reached.
//   - a failure to close the duplicated source descriptor is returned as
//     warn, which the caller carries out of the window on preparedCommand
//     (see stagingWarn) rather than logging here.
//
// When cred is non-nil (run-as execution), both the staging directory and the
// staged file are chgrp'd (not chowned) to cred's gid, leaving the owner as
// root/parent. This lets the target user -- running as a different,
// unprivileged uid -- traverse and execute them via group permission, without
// granting that user chown/chmod rights over the staged copy (which owner
// rights would allow) and without exposing the copy to any other user sharing
// the staging parent directory (e.g. /tmp). When cred is nil (normal
// execution), no chgrp occurs and the staging directory and staged file are
// owned by the current, typically non-root, invoking user.
func (e *DefaultExecutor) stageFromFD(identity *risktypes.VerifiedIdentity, cred *syscall.Credential) (stagedPath string, cleanupFn func() error, warn error, err error) {
	dir, err := os.MkdirTemp("", "scr-stage-")
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to create staging directory: %w", err)
	}
	cleanup := func() error {
		rmErr := os.RemoveAll(dir)
		if rmErr != nil {
			// The write goes to the already-open stderr descriptor and
			// bypasses the redaction handler, so it carries only the
			// staging directory path and the error -- never secret
			// values. The WriteString form is deliberate: the static
			// window guard added in a later phase allows
			// (*os.File).WriteString by name (02_architecture.md §7.2),
			// and a fmt.Fprintf call would force that allowlist to admit
			// writes to arbitrary writers instead.
			//
			//nolint:errcheck,gosec,staticcheck // there is no further fallback if the write itself fails
			os.Stderr.WriteString(fmt.Sprintf("WARNING: failed to remove staging directory %s: %v\n", dir, rmErr))
		}
		return rmErr
	}
	// On any error return below, the staging directory (and whatever was
	// written into it so far) must not leak: this defer runs cleanup
	// whenever the named return err is non-nil, so each error path below
	// only needs to `return ..., err` without repeating cleanup() itself.
	defer func() {
		if err != nil {
			_ = cleanup()
		}
	}()

	// Chgrp (uid -1 leaves owner unchanged, keeping root/parent as owner) the
	// staging directory to the run-as gid so that user (running as a
	// different, unprivileged uid) can traverse it via group permission; the
	// parent process here still runs privileged (root), so Chown is permitted.
	if cred != nil {
		// #nosec G115 -- cred.Gid is uint32 parsed via strconv.ParseUint(s, 10, 32), so it fits in int (64-bit on all supported platforms).
		if err := os.Chown(dir, -1, int(cred.Gid)); err != nil {
			return "", nil, nil, fmt.Errorf("failed to chgrp staging directory to gid=%d: %w", cred.Gid, err)
		}
		if err := os.Chmod(dir, stagingDirMode); err != nil {
			return "", nil, nil, fmt.Errorf("failed to chmod staging directory: %w", err)
		}
	}

	// Duplicate the verified descriptor so this function owns a separate closable
	// handle (the original stays owned by VerifiedFD). The duplicate shares the
	// same open file description -- and therefore the same file offset -- as the
	// original, so the copy below reads via ReadAt (pread) over a SectionReader,
	// which never moves that shared offset.
	dup, err := syscall.Dup(identity.FD.Fd())
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to duplicate verified fd for staging: %w", err)
	}
	src := os.NewFile(uintptr(dup), identity.ResolvedPath) // #nosec G115 -- dup is a valid non-negative fd from syscall.Dup; int->uintptr cannot overflow
	if src == nil {
		_ = syscall.Close(dup)
		return "", nil, nil, ErrNoVerifiedFD
	}
	defer func() {
		if closeErr := src.Close(); closeErr != nil {
			warn = closeErr
		}
	}()
	info, err := src.Stat()
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to stat verified fd for staging: %w", err)
	}

	// Preserve the original basename: multi-call binaries (e.g. busybox/coreutils)
	// select their applet from the executable name, and tools may inspect
	// /proc/self/exe, so the staged copy must keep the verified command's name.
	name := filepath.Base(identity.ResolvedPath)
	if name == "." || name == string(filepath.Separator) || name == "" {
		name = "command"
	}
	path := filepath.Join(dir, name)
	// #nosec G304 G302 - path is a freshly created file (O_EXCL) inside our
	// own 0700 MkdirTemp directory; the basename derives from the already-verified
	// resolved path, not untrusted input. The execute bit (0500) is required to
	// exec the staged copy and write is intentionally withheld.
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, stagedExecMode)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to create staged command file: %w", err)
	}
	if _, err := io.Copy(dst, io.NewSectionReader(src, 0, info.Size())); err != nil {
		_ = dst.Close()
		return "", nil, nil, fmt.Errorf("failed to stage verified command: %w", err)
	}
	if err := dst.Close(); err != nil {
		return "", nil, nil, fmt.Errorf("failed to close staged command file: %w", err)
	}

	// os.OpenFile's mode is subject to the process umask, so in hardened
	// environments (e.g. umask 0027/0077) the group execute bit requested via
	// stagedExecMode could be silently stripped. Explicitly chmod the staged
	// file so its permissions are deterministic regardless of umask, mirroring
	// how the staging directory is already explicitly chmod'd above.
	if err := os.Chmod(path, stagedExecMode); err != nil {
		return "", nil, nil, fmt.Errorf("failed to chmod staged command file: %w", err)
	}

	// Chgrp (uid -1 leaves owner as root/parent) the staged file to the run-as
	// gid so that user can execute it via group permission despite not owning
	// it, preventing the target user from chown/chmod'ing the staged copy.
	if cred != nil {
		// #nosec G115 -- cred.Gid is uint32 parsed via strconv.ParseUint(s, 10, 32), so it fits in int (64-bit on all supported platforms).
		if err := os.Chown(path, -1, int(cred.Gid)); err != nil {
			return "", nil, nil, fmt.Errorf("failed to chgrp staged command file to gid=%d: %w", cred.Gid, err)
		}
	}

	return path, cleanup, warn, nil
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
// and writes to an OutputWriter with a specific stream name.
//
// Each wrapper is written by exactly one goroutine: the output pump starts
// one reader goroutine per stream, and Execute gives stdout and stderr
// separate wrapper instances. buffer and writeErr are read only after that
// reader goroutine's done channel has yielded a value, which happens after
// the goroutine has stopped writing -- the pump's wait enforces this order.
// The shared OutputWriter is the part reached from both goroutines, and
// OutputWriter implementations are required to be thread-safe (see the
// interface contract in interface.go).
type outputWrapper struct {
	writer   OutputWriter
	stream   OutputStream
	buffer   *boundedBuffer
	writeErr error // Stores the first write error encountered
}

// newOutputWrapper builds the wrapper for one stream. limit bounds how many
// bytes the wrapper retains in memory; 0 means unbounded.
func newOutputWrapper(writer OutputWriter, stream OutputStream, limit int) *outputWrapper {
	return &outputWrapper{
		writer: writer,
		stream: stream,
		buffer: newBoundedBuffer(limit),
	}
}

func (w *outputWrapper) Write(p []byte) (n int, err error) {
	// Write to buffer for capturing. boundedBuffer.Write never fails.
	_, _ = w.buffer.Write(p)

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
	return w.buffer.Bytes()
}

func (w *outputWrapper) GetWriteError() error {
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
