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
// unreachable while prepareCommand is the only constructor; it exists
// because the switch it guards sits on a privilege boundary, where failing
// closed on an impossible input is cheaper than reasoning about whether it
// stays impossible.
var ErrExecBindingUnset = errors.New("execution binding not declared")

// execBinding declares how the executed inode is bound. The zero value is
// bindingUnset, which the switch in startPrepared rejects, so a
// preparedCommand whose binding was never declared cannot be started.
type execBinding int

const (
	bindingUnset execBinding = iota
	bindingVerifiedFD
	bindingStagedCopy
	bindingResolvedPath
)

// killStrategy declares what a cancellation-triggered kill requires. Like
// execBinding it has an explicit unset zero value: defaulting to "no
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
	// binding == bindingStagedCopy. release() records its result in
	// stagingCleanupErr rather than logging it, since release() itself may
	// run inside the privilege window.
	stagingCleanup    func() error
	stagingCleanupErr error

	// stagingWarn carries a non-fatal staging failure (closing the fd staging
	// read from) out of the privilege window so the caller can log it once
	// the window has closed. Nothing inside a window may log: a slog handler
	// is free to open a file, and it would do so at euid 0. Staging succeeded
	// when this is set, so it is not returned as an error.
	stagingWarn error

	devNull *os.File

	// devNullCloseErr and verifiedFDCloseErr record release()'s close
	// failures on devNull and verifiedFD, the same way stagingCleanupErr
	// does for the staged copy: release() itself may run inside the
	// privilege window, so it records rather than logs, and the caller logs
	// once the window has closed.
	devNullCloseErr    error
	verifiedFDCloseErr error

	cmdLine         string // pre-formatted for logging; see FormatCommandForLog
	hasOutputWriter bool   // whether Execute's caller supplied an OutputWriter
}

// release closes every descriptor prepareCommand acquired: the null-device
// stdin, the duplicated verified descriptor (if any), the staged copy's
// directory (if any), and the output pump. Safe to call multiple times.
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
		errs = append(errs, pc.pump.release())
		pc.pump = nil
	}
	return errors.Join(errs...)
}

// commandOutcome collects everything superviseCommand learns about the
// child. reaped is always true in this phase; it becomes meaningful once the
// kill path can give up on collecting the child (see the cancellation work,
// which also adds the fields for a cancellation-triggered kill's outcome:
// the ctx error and the kill error).
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

	switch {
	case identity != nil && identity.FD != nil && !e.fdExecDisabled && fdExecSupported():
		pc.binding = bindingVerifiedFD
		childPath, extraFile, err := fdExecExtraFile(identity)
		if err != nil {
			return nil, err
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
			return nil, err
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

	// Set up stdin to null device for security and stability:
	// 1. Security: Prevents child processes from reading unexpected input from stdin
	// 2. Stability: Prevents errors in commands that try to allocate a pseudo-TTY when stdin is nil
	//    (e.g., docker-compose exec can fail with "exit status 255" if stdin is not configured)
	// 3. Best practice: Batch processing tools should explicitly control stdin rather than inheriting it
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		_ = pc.release()
		return nil, fmt.Errorf("failed to open null device for stdin: %w", err)
	}
	pc.devNull = devNull
	pc.execCmd.Stdin = devNull

	if cmd.EffectiveWorkDir != "" {
		pc.execCmd.Dir = cmd.EffectiveWorkDir
	}

	// Only use the filtered environment variables provided in envVars; this
	// ensures allowlist filtering is properly enforced.
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
		_ = pc.release()
		return nil, err
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
// returned error alongside whatever the run itself produced, but it does not
// change which error superviseCommand reports first (see the priority order
// below).
func (e *DefaultExecutor) superviseCommand(_ context.Context, pc *preparedCommand, startupErr error) (*Result, error) {
	defer func() { _ = pc.release() }()

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- pc.execCmd.Wait()
	}()

	// Read the child's output into the wrappers. Must not start before the
	// privilege window has closed; that is not yet guaranteed in this phase,
	// since WithPrivileges still wraps prepareCommand, startPrepared and
	// superviseCommand together for run-as execution (see executor.go).
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
