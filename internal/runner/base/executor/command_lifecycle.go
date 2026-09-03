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

// execBinding declares how the executed inode is bound. The zero value is
// bindingUnset, which every switch over this type rejects, so a
// preparedCommand whose binding was never declared cannot be started.
type execBinding int

const (
	bindingUnset        execBinding = iota
	bindingVerifiedFD               // /proc/self/fd/<n> (Linux)
	bindingStagedCopy               // private copy made from the verified fd
	bindingResolvedPath             // already-resolved path, no verified fd
)

// killStrategy declares what a cancellation-triggered kill requires. Like
// execBinding it has an explicit unset zero value: defaulting to "no
// re-elevation" would silently reproduce the EPERM-on-kill failure this task
// exists to fix.
type killStrategy int

const (
	killUnset      killStrategy = iota
	killDirect                  // no credential was applied; kill as-is
	killReelevated              // run-as execution; kill inside a privilege window
)

// ErrExecBindingUnset is returned when a preparedCommand reaches the start
// path without having declared how the executed inode is bound. It is
// unreachable while prepareCommand is the only constructor; it exists because
// the switch it guards sits on a privilege boundary, where failing closed on
// an impossible input is cheaper than reasoning about whether it stays
// impossible.
var ErrExecBindingUnset = errors.New("execution binding not declared")

// commandOutcome collects everything superviseCommand learns about the child.
// The wait goroutine fills waitErr and sends the outcome; the remaining
// fields are filled by the cancel and kill paths added in a later phase, so
// the wait goroutine already hands results through the final shape.
type commandOutcome struct {
	waitErr  error
	ctxErr   error  //nolint:unused // non-nil only when the run was ended by cancellation
	killErr  error  //nolint:unused // the kill's failure, if any
	stdout   []byte //nolint:unused // the pump's captured stdout
	stderr   []byte //nolint:unused // the pump's captured stderr
	writeErr error  //nolint:unused // the pump's write error, stdout side preferred
	reaped   bool   //nolint:unused // false when the child did not exit within killGraceDelay
}

// preparedCommand holds what prepareCommand built without privilege.
// release() frees every descriptor prepareCommand acquired, on every path.
type preparedCommand struct {
	execCmd *exec.Cmd
	binding execBinding
	pump    *outputPump
	kill    killStrategy
	// outputWriter is the writer the pump forwards to; nil means the run's
	// output is collected into the Result instead.
	outputWriter OutputWriter
	verifiedFD   *os.File // duplicated verified fd; nil unless bindingVerifiedFD
	devNull      *os.File
	// stageCleanup removes the staged copy and its directory; nil unless
	// binding == bindingStagedCopy. It must run only after the child is
	// gone: a shebang interpreter opens the script path after execve, so
	// deleting the copy at start would race that open.
	stageCleanup func() error
	// stagingWarn carries a non-fatal staging failure (a close of the
	// duplicated source descriptor) out of the privilege window so the
	// caller can log it after the window closes. Nothing inside a window
	// may log: a slog handler is free to open a file, and it would do so at
	// euid 0. Staging succeeded when this is set, so it is not returned as
	// an error.
	stagingWarn error
	// stagingCleanupErr carries the staged-copy removal error out of the
	// privilege window for the same reason as stagingWarn: the removal runs
	// after the child exits, which is inside a privilege window, and the
	// error is not a failure of the run itself.
	stagingCleanupErr error
}

// release frees every descriptor the prepare phase acquired: the output
// pump's pipe ends, the duplicated verified fd, the null device, and the
// staged copy. Fields are cleared as they are released, so a second call is
// a no-op.
func (pc *preparedCommand) release() error {
	var errs []error
	if pc.pump != nil {
		errs = append(errs, pc.pump.release())
		pc.pump = nil
	}
	if pc.verifiedFD != nil {
		errs = append(errs, pc.verifiedFD.Close())
		pc.verifiedFD = nil
	}
	if pc.devNull != nil {
		errs = append(errs, pc.devNull.Close())
		pc.devNull = nil
	}
	if pc.stageCleanup != nil {
		errs = append(errs, pc.stageCleanup())
		pc.stageCleanup = nil
	}
	return errors.Join(errs...)
}

// prepareCommand is the prepare phase: it builds everything the run needs
// without privilege. It selects how the executed inode is bound, wires the
// credential, the null-device stdin, the working directory, the environment,
// and the output pump, and declares the binding and the kill strategy.
//
// When plan carries a verified file descriptor, execution is bound to that
// descriptor (fd-bound exec on Linux, or read-only staging copied from the
// descriptor as a fallback) so the executed inode is exactly the one the
// evaluator verified. Without a verified descriptor the already-resolved path
// is executed directly (no re-resolution); the evaluator's identity gate
// denies unverified binaries before they reach an allowed plan, so this
// branch does not weaken the production guarantee.
//
// cred is the kernel-level credential to pass to the child process via
// SysProcAttr.Credential (used for run-as execution). When nil, no credential
// override is applied (normal execution).
func (e *DefaultExecutor) prepareCommand(ctx context.Context, plan *risktypes.VerifiedCommandPlan, path string, cmd *runnertypes.RuntimeCommand, envVars map[string]string, outputWriter OutputWriter, cred *syscall.Credential) (*preparedCommand, error) {
	// Log the command being executed at DEBUG level
	cmdLine := FormatCommandForLog(path, cmd.ExpandedArgs)
	e.Logger.Debug("Executing command",
		"command", cmdLine,
		"path", path,
		"work_dir", cmd.EffectiveWorkDir,
		"work_dir_len", len(cmd.EffectiveWorkDir))

	pc := &preparedCommand{outputWriter: outputWriter}

	// The prepare phase failed: release what it acquired so far. A release
	// failure is reported the way the pre-lifecycle code reported it (a
	// warning), not joined into the error that identifies the prepare
	// failure.
	fail := func(err error) (*preparedCommand, error) {
		if releaseErr := pc.release(); releaseErr != nil {
			e.Logger.Warn("Failed to release command resources", "error", releaseErr)
		}
		return nil, err
	}

	var identity *risktypes.VerifiedIdentity
	if plan != nil {
		identity = plan.Identity
	}

	if identity != nil && identity.FD != nil {
		if !e.fdExecDisabled && fdExecSupported() {
			childPath, extraFile, err := fdExecExtraFile(identity)
			if err != nil {
				return nil, err
			}
			pc.binding = bindingVerifiedFD
			pc.verifiedFD = extraFile
			// #nosec G204 - childPath is /proc/self/fd/<n> bound to the verified inode.
			execCmd := exec.CommandContext(ctx, childPath, cmd.ExpandedArgs...)
			execCmd.Args[0] = path // present the resolved path as argv[0] to the child
			execCmd.ExtraFiles = []*os.File{extraFile}
			pc.execCmd = execCmd
		} else {
			stagedPath, cleanup, err := e.stageFromFD(pc, identity, cred)
			if err != nil {
				return nil, err // stageFromFD already removed its staging directory
			}
			pc.binding = bindingStagedCopy
			pc.stageCleanup = cleanup
			// #nosec G204 - stagedPath is a private copy of the verified inode.
			execCmd := exec.CommandContext(ctx, stagedPath, cmd.ExpandedArgs...)
			execCmd.Args[0] = path
			pc.execCmd = execCmd
		}
	} else {
		pc.binding = bindingResolvedPath
		// #nosec G204 - The command and arguments are validated before execution with e.Validate()
		pc.execCmd = exec.CommandContext(ctx, path, cmd.ExpandedArgs...)
	}

	// Set SysProcAttr.Credential for run-as execution. When cred is non-nil,
	// the kernel sets uid/gid/supplementary groups atomically at execve time.
	// For normal execution (cred == nil), no credential is set.
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
		return fail(fmt.Errorf("failed to open null device for stdin: %w", err))
	}
	pc.devNull = devNull
	pc.execCmd.Stdin = devNull

	// Set up working directory
	if cmd.EffectiveWorkDir != "" {
		pc.execCmd.Dir = cmd.EffectiveWorkDir
	}

	// Set up environment variables
	// Only use the filtered environment variables provided in envVars
	// This ensures allowlist filtering is properly enforced
	pc.execCmd.Env = make([]string, 0, len(envVars))
	for k, v := range envVars {
		pc.execCmd.Env = append(pc.execCmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up output capture. The child writes into *os.File pipe ends, so
	// os/exec starts no copy goroutine of its own; the pump reads the other
	// ends into the wrappers. Without an OutputWriter, stderr is bounded to
	// the same prefix/suffix limit os/exec applies to Cmd.Output's stderr.
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

	// Fail closed on a binding that was never declared: the start path
	// refuses to run a command whose inode binding is unknown.
	switch pc.binding {
	case bindingVerifiedFD, bindingStagedCopy, bindingResolvedPath:
	default:
		return fail(ErrExecBindingUnset)
	}

	return pc, nil
}

// startPrepared is the start phase: it starts the child process. In this
// phase its body is the Start() call itself; the staged copy is still made in
// the prepare phase, so execCmd is fully built when this runs.
//
// Invariant: started is true only when the child is actually running. A
// running child must always be supervised (killed and reaped) rather than
// abandoned, so callers branch on started, not on err.
func (e *DefaultExecutor) startPrepared(pc *preparedCommand) (started bool, err error) {
	if startErr := pc.execCmd.Start(); startErr != nil {
		return false, startErr
	}
	return true, nil
}

// startAndSupervise runs the start and supervise phases of a prepared
// command: it starts the child, releases the pipe write ends on every path,
// and either reports the start failure or hands the running child to
// superviseCommand.
//
// The pipe write ends are released on every path, success or failure, and
// before supervision starts: left open, the read ends never reach EOF and
// the pump's wait blocks on them.
func (e *DefaultExecutor) startAndSupervise(ctx context.Context, pc *preparedCommand) (*Result, error) {
	started, startErr := e.startPrepared(pc)
	closeErr := pc.pump.releaseChildEnds()

	switch {
	case !started:
		// The child never ran: release the read ends as well.
		releaseErr := pc.release()
		result := &Result{ExitCode: ExitCodeUnknown}
		cmdLine := FormatCommandForLog(pc.execCmd.Args[0], pc.execCmd.Args[1:])
		e.Logger.Error("Command execution failed",
			"error", startErr,
			"command", cmdLine,
			"exit_code", result.ExitCode)
		return result, fmt.Errorf("command execution failed: %w", errors.Join(startErr, closeErr, releaseErr))
	case startErr != nil || closeErr != nil:
		// The child is running: supervise it so it is stopped and reaped
		// before the run reports, with the start-phase failure attached.
		return e.superviseCommand(ctx, pc, errors.Join(startErr, closeErr))
	default:
		return e.superviseCommand(ctx, pc, nil)
	}
}

// superviseCommand is the supervise phase: it waits for the child to finish,
// collects its output, releases what the prepare phase acquired, and
// assembles the Result. The child is already running when it is called.
//
// startupErr carries a start-phase failure that did not stop the child (a
// pipe write end that could not be released). It is joined into the returned
// error: the child ran, so the run's outcome is the child's, and the
// unreleased descriptor is reported alongside it rather than hidden.
//
//nolint:revive,unparam // ctx is not used until the cancel path is added; the signature is the one the later phase builds on
func (e *DefaultExecutor) superviseCommand(ctx context.Context, pc *preparedCommand, startupErr error) (*Result, error) {
	// Reap the child in its own goroutine. exec.CommandContext still kills
	// it on context cancellation.
	waitCh := make(chan commandOutcome, 1)
	go func() {
		waitCh <- commandOutcome{waitErr: pc.execCmd.Wait()}
	}()

	// Read the child's output into the wrappers.
	pc.pump.start()

	outcome := <-waitCh
	stdout, stderr, writeErr, _ := pc.pump.wait(0)

	// The child is gone; the staged copy must outlive it (a shebang
	// interpreter opens the script path after execve), so it is removed
	// here. Its removal error is not a failure of the run: it is carried on
	// pc for logging after the privilege window closes.
	if pc.stageCleanup != nil {
		if rmErr := pc.stageCleanup(); rmErr != nil {
			pc.stagingCleanupErr = rmErr
		}
		pc.stageCleanup = nil
	}
	if releaseErr := pc.release(); releaseErr != nil {
		e.Logger.Warn("Failed to release command resources", "error", releaseErr)
	}

	// Prepare the result
	result := &Result{
		Stdout: string(stdout),
	}
	if pc.outputWriter != nil {
		result.Stderr = string(stderr)
	} else if _, isExitError := errors.AsType[*exec.ExitError](outcome.waitErr); isExitError {
		// Match Cmd.Output: without an OutputWriter, stderr is reported
		// only when the command exited abnormally.
		result.Stderr = string(stderr)
	}
	if pc.execCmd.ProcessState != nil {
		result.ExitCode = pc.execCmd.ProcessState.ExitCode()
	} else {
		result.ExitCode = ExitCodeUnknown // Use constant for unknown exit code
	}

	cmdErr := outcome.waitErr
	// A write error (e.g. output size limit exceeded) outranks the broken
	// pipe error the child exits with once the reader closed the pipe: it
	// is the real cause of the failure.
	if writeErr != nil {
		cmdErr = writeErr
	}
	// A descriptor that could not be released leaks, so it is reported
	// alongside the run's own outcome.
	cmdErr = errors.Join(cmdErr, startupErr)

	if cmdErr != nil {
		cmdLine := FormatCommandForLog(pc.execCmd.Args[0], pc.execCmd.Args[1:])
		e.Logger.Error("Command execution failed",
			"error", cmdErr,
			"command", cmdLine,
			"exit_code", result.ExitCode,
			"stderr", string(stderr))
		return result, fmt.Errorf("command execution failed: %w", cmdErr)
	}

	return result, nil
}

// logStagingWarnings records the non-fatal staging failures carried on pc
// (stagingWarn and the staged-copy removal error). It must run after the
// privilege window has closed: nothing inside a window may log, because a
// slog handler is free to open a file, and it would do so at euid 0.
func (e *DefaultExecutor) logStagingWarnings(pc *preparedCommand) {
	if pc == nil {
		return
	}
	if pc.stagingWarn != nil {
		e.Logger.Warn("Failed to close staging source fd", "error", pc.stagingWarn)
	}
	if pc.stagingCleanupErr != nil {
		e.Logger.Warn("Failed to remove staging directory", "error", pc.stagingCleanupErr)
	}
}
