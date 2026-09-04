package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// ErrExecBindingUnset is returned when a preparedCommand reaches the start
// path without having declared how the executed inode is bound. It is
// unreachable while prepareCommand is the only constructor; the guard sits on
// a privilege boundary, so it fails closed rather than relying on that
// staying true.
var ErrExecBindingUnset = errors.New("execution binding not declared")

// ErrPreparedCommandSpent is returned when a preparedCommand that
// prepareCommand already released -- the one it returns alongside an error --
// reaches the start path. Unlike ErrExecBindingUnset this input is one the
// package can actually produce, so the guard is what enforces "must not be
// started".
var ErrPreparedCommandSpent = errors.New("prepared command already released")

// ErrKillStrategyUnset is returned when a preparedCommand reaches the kill
// path without having declared what a cancellation-triggered kill requires.
// Like ErrExecBindingUnset it is unreachable while prepareCommand is the only
// constructor; the guard sits on a privilege boundary, so it fails closed
// rather than relying on that staying true.
var ErrKillStrategyUnset = errors.New("kill strategy not declared")

// ErrKillAfterCancel is returned when the child could not be killed after the
// context was cancelled, so the run may leave a process behind.
var ErrKillAfterCancel = errors.New("failed to kill command after cancellation")

// ErrChildNotReaped is returned when the child did not exit within
// killGraceDelay after the kill, which usually means a grandchild inherited
// the pipe. The run returns rather than blocking; the pid is logged.
var ErrChildNotReaped = errors.New("command did not exit after kill")

// execBinding declares how the executed inode is bound. The zero value is
// bindingUnset, which startPrepared rejects, so a preparedCommand whose
// binding was never declared cannot be started.
type execBinding int

const (
	bindingUnset execBinding = iota
	bindingVerifiedFD
	bindingStagedCopy
	bindingResolvedPath
)

// killStrategy declares what a cancellation-triggered kill requires. Like
// execBinding its zero value is an explicit unset: defaulting to "no
// re-elevation" would silently reproduce the EPERM-on-kill failure this task
// exists to fix. Not yet consumed by any kill path; that arrives with the
// cancellation work.
type killStrategy int

const (
	killUnset killStrategy = iota
	killDirect
	killReelevated
)

// preparedCommand holds what prepareCommand builds before the privilege
// window opens. release() frees every descriptor prepareCommand acquired, on
// every path -- callers must call it exactly once, whether the command ran or
// prepareCommand's caller gave up before starting it.
type preparedCommand struct {
	execCmd *exec.Cmd
	binding execBinding
	pump    *outputPump
	kill    killStrategy

	// verifiedFD is the duplicated verified descriptor used for fd-bound
	// execution; nil unless binding == bindingVerifiedFD.
	verifiedFD *os.File

	// stagingCleanup removes the staged copy's directory; nil unless
	// binding == bindingStagedCopy.
	stagingCleanup func() error

	// stagingWarn carries a non-fatal staging failure (closing the fd staging
	// read from) out of the privilege window. Staging succeeded when this is
	// set, so it is not returned as an error.
	stagingWarn error

	devNull *os.File

	// release()'s failures, recorded for the caller to log; see release().
	stagingCleanupErr  error
	devNullCloseErr    error
	verifiedFDCloseErr error
	pumpReleaseErr     error

	cmdLine         string // pre-formatted for logging; see FormatCommandForLog
	hasOutputWriter bool   // whether Execute's caller supplied an OutputWriter

	// spent marks a preparedCommand that prepareCommand released and handed
	// back alongside an error. It carries only recorded warnings; its pump
	// and descriptors are gone, so starting it would nil-deref.
	spent bool
}

// release closes every descriptor prepareCommand acquired: the null-device
// stdin, the duplicated verified descriptor (if any), the staged copy's
// directory (if any), and the output pump. Safe to call multiple times --
// each resource's guarding field is cleared as it is released, so a second
// call neither re-releases nor overwrites the recorded error.
//
// Every failure is recorded on pc as well as joined into the returned error.
// release may itself run inside the privilege window, where nothing may log
// (a slog handler is free to open a file, and would do so at euid 0), so a
// caller that discards the return value -- superviseCommand does -- still
// leaves the failures where logDeferredWarnings can report them once the
// window has closed.
func (pc *preparedCommand) release() error {
	var errs []error
	if pc.devNull != nil {
		pc.devNullCloseErr = closeUnlessClosed(pc.devNull)
		errs = append(errs, pc.devNullCloseErr)
		pc.devNull = nil
	}
	if pc.verifiedFD != nil {
		pc.verifiedFDCloseErr = closeUnlessClosed(pc.verifiedFD)
		errs = append(errs, pc.verifiedFDCloseErr)
		pc.verifiedFD = nil
	}
	if pc.stagingCleanup != nil {
		pc.stagingCleanupErr = pc.stagingCleanup()
		errs = append(errs, pc.stagingCleanupErr)
		pc.stagingCleanup = nil
	}
	if pc.pump != nil {
		pc.pumpReleaseErr = pc.pump.release()
		errs = append(errs, pc.pumpReleaseErr)
		pc.pump = nil
	}
	return errors.Join(errs...)
}

// commandOutcome collects everything superviseCommand learns about the
// child. reaped is always true in this phase; it becomes meaningful once the
// kill path can give up on collecting the child (the cancellation work).
type commandOutcome struct {
	waitErr  error
	stdout   []byte
	stderr   []byte
	writeErr error
	reaped   bool
}

// prepareCommand builds a preparedCommand without privilege: it selects how
// the executed inode is bound, applies the run-as credential, opens the null
// device used for stdin, sets the working directory and environment, and
// creates the output pump. cred is nil for normal execution.
//
// This phase still uses exec.CommandContext for all three binding paths;
// switching to exec.Command (so cancellation is handled by the caller
// instead of a watchdog goroutine) is deferred to the cancellation work,
// together with moving the staged-copy creation into the start phase.
//
// On failure prepareCommand releases everything it acquired and returns a
// non-nil preparedCommand alongside the error, so the caller can hand it to
// logDeferredWarnings once the privilege window has closed. That pc is spent:
// it carries only the recorded warnings and must not be started.
func (e *DefaultExecutor) prepareCommand(ctx context.Context, plan *risktypes.VerifiedCommandPlan, path string, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter, cred *syscall.Credential) (*preparedCommand, error) {
	cmdLine := FormatCommandForLog(path, cmd.ExpandedArgs)
	e.Logger.Debug("Executing command",
		"command", cmdLine,
		"path", path,
		"work_dir", cmd.EffectiveWorkDir,
		"work_dir_len", len(cmd.EffectiveWorkDir))

	var identity *risktypes.VerifiedIdentity
	if plan != nil {
		identity = plan.Identity
	}

	pc := &preparedCommand{
		cmdLine:         cmdLine,
		hasOutputWriter: outputWriter != nil,
	}

	fail := func(err error) (*preparedCommand, error) {
		_ = pc.release()
		pc.spent = true
		return pc, err
	}

	switch {
	case identity != nil && identity.FD != nil && !e.fdExecDisabled && fdExecSupported():
		pc.binding = bindingVerifiedFD
		childPath, extraFile, err := fdExecExtraFile(identity)
		if err != nil {
			return fail(err)
		}
		// #nosec G204 - childPath is /proc/self/fd/<n> bound to the verified inode.
		pc.execCmd = exec.CommandContext(ctx, childPath, cmd.ExpandedArgs...)
		pc.execCmd.Args[0] = path // present the resolved path as argv[0] to the child
		pc.execCmd.ExtraFiles = []*os.File{extraFile}
		pc.verifiedFD = extraFile
	case identity != nil && identity.FD != nil:
		pc.binding = bindingStagedCopy
		stagedPath, cleanupFn, warn, err := e.stageFromFD(identity, cred)
		if err != nil {
			// stageFromFD already removed its staging directory, but it can
			// still have failed to close the descriptor it staged from; carry
			// that warning out even though staging itself failed.
			pc.stagingWarn = warn
			return fail(err)
		}
		// #nosec G204 - stagedPath is a private copy of the verified inode.
		pc.execCmd = exec.CommandContext(ctx, stagedPath, cmd.ExpandedArgs...)
		pc.execCmd.Args[0] = path
		pc.stagingCleanup = cleanupFn
		pc.stagingWarn = warn
	default:
		pc.binding = bindingResolvedPath
		// #nosec G204 - The command and arguments are validated before execution with e.Validate()
		pc.execCmd = exec.CommandContext(ctx, path, cmd.ExpandedArgs...)
	}

	applyCredential(pc.execCmd, cred)
	if cred != nil {
		pc.kill = killReelevated
	} else {
		pc.kill = killDirect
	}

	// Bind stdin to the null device rather than inheriting it, so the child
	// cannot read unexpected input. A nil stdin is not equivalent: commands
	// that try to allocate a pseudo-TTY fail on it (docker-compose exec exits
	// 255).
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return fail(fmt.Errorf("failed to open null device for stdin: %w", err))
	}
	pc.devNull = devNull
	pc.execCmd.Stdin = devNull

	if cmd.EffectiveWorkDir != "" {
		pc.execCmd.Dir = cmd.EffectiveWorkDir
	}

	// Use only envVars, never the parent environment: that is what enforces
	// allowlist filtering.
	pc.execCmd.Env = make([]string, 0, len(envVars))
	for k, v := range envVars {
		pc.execCmd.Env = append(pc.execCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Without an OutputWriter, stderr is bounded to the same prefix/suffix
	// limit os/exec applies to Cmd.Output's stderr.
	stderrLimit := 0
	if outputWriter == nil {
		stderrLimit = nilWriterStderrLimit
	}
	pump, err := newOutputPump(outputWriter, stderrLimit)
	if err != nil {
		return fail(err)
	}
	pc.pump = pump
	stdoutFile, stderrFile := pump.childFiles()
	pc.execCmd.Stdout = stdoutFile
	pc.execCmd.Stderr = stderrFile

	return pc, nil
}

// startPrepared starts the child process. started is true once Start has
// succeeded; the caller must proceed to supervise the child (reap it and
// collect its output) whenever started is true, regardless of err, since a
// running child must not be abandoned.
func (e *DefaultExecutor) startPrepared(pc *preparedCommand) (started bool, err error) {
	if pc.spent {
		return false, ErrPreparedCommandSpent
	}
	switch pc.binding {
	case bindingVerifiedFD, bindingStagedCopy, bindingResolvedPath:
	default:
		return false, ErrExecBindingUnset
	}

	if err := pc.execCmd.Start(); err != nil {
		return false, err
	}
	return true, nil
}

// runCommand starts pc's child and, once started, supervises it to
// completion. On failure to start it releases every resource prepareCommand
// acquired and returns a placeholder Result with ExitCodeUnknown, matching
// what execCmd.Start failing has always reported.
//
// The pipe write ends are released here -- immediately after Start returns,
// regardless of its outcome -- because the read ends never reach EOF
// otherwise, which would block the pump's wait until its deadline.
//
// Three log records still run inside the privilege window for run-as
// execution: prepareCommand's "Executing command" debug record and the
// "Command execution failed" records here and in superviseCommand. They leave
// the window when it is narrowed to startPrepared.
func (e *DefaultExecutor) runCommand(ctx context.Context, pc *preparedCommand) (*Result, error) {
	started, startErr := e.startPrepared(pc)
	closeErr := pc.pump.releaseChildEnds()

	if !started {
		releaseErr := pc.release()
		combinedErr := errors.Join(startErr, closeErr, releaseErr)
		result := &Result{ExitCode: ExitCodeUnknown}
		e.Logger.Error("Command execution failed",
			"error", combinedErr,
			"command", pc.cmdLine,
			"exit_code", result.ExitCode)
		return result, fmt.Errorf("command execution failed: %w", combinedErr)
	}

	return e.superviseCommand(ctx, pc, closeErr)
}

// superviseCommand reaps the child and reads its output, then builds the
// Result. startupErr carries a non-fatal failure from the start phase (here,
// only a failure to release the pipe write ends); it is joined into the
// returned error but does not change which error is reported first.
func (e *DefaultExecutor) superviseCommand(_ context.Context, pc *preparedCommand, startupErr error) (*Result, error) {
	// Discarded rather than logged: superviseCommand still runs inside the
	// privilege window for run-as execution. release records each failure on
	// pc for logDeferredWarnings.
	defer func() { _ = pc.release() }()

	waitCh := make(chan error, 1)
	go func() {
		// waitFn, when injected, stands in for Wait() here -- the only way to
		// reach ErrChildNotReaped deterministically (see DefaultExecutor.waitFn).
		if e.waitFn != nil {
			waitCh <- e.waitFn(pc.execCmd)
			return
		}
		waitCh <- pc.execCmd.Wait()
	}()

	// Must not start before the privilege window has closed; not yet
	// guaranteed in this phase, since WithPrivileges still wraps
	// prepareCommand, startPrepared and superviseCommand together for run-as
	// execution (see executor.go).
	pc.pump.start()

	outcome := commandOutcome{reaped: true}
	outcome.waitErr = <-waitCh
	outcome.stdout, outcome.stderr, outcome.writeErr, _ = pc.pump.wait(0)

	result := &Result{Stdout: string(outcome.stdout)}
	if pc.hasOutputWriter {
		result.Stderr = string(outcome.stderr)
	} else if _, isExitError := errors.AsType[*exec.ExitError](outcome.waitErr); isExitError {
		// Match Cmd.Output: without an OutputWriter, stderr is reported
		// only when the command exited abnormally.
		result.Stderr = string(outcome.stderr)
	}
	if pc.execCmd.ProcessState != nil {
		result.ExitCode = pc.execCmd.ProcessState.ExitCode()
	} else {
		result.ExitCode = ExitCodeUnknown
	}

	cmdErr := outcome.waitErr
	// A write error (e.g. output size limit exceeded) outranks the broken
	// pipe error the child exits with once the reader closed the pipe: it
	// is the real cause of the failure.
	if outcome.writeErr != nil {
		cmdErr = outcome.writeErr
	}
	// A write end that could not be released leaks a descriptor, so it is
	// reported alongside the run's own outcome.
	if startupErr != nil {
		cmdErr = errors.Join(cmdErr, startupErr)
	}

	if cmdErr != nil {
		e.Logger.Error("Command execution failed",
			"error", cmdErr,
			"command", pc.cmdLine,
			"exit_code", result.ExitCode,
			"stderr", string(outcome.stderr))
		return result, fmt.Errorf("command execution failed: %w", cmdErr)
	}

	return result, nil
}
