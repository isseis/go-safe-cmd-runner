package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

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
// exists to fix.
type killStrategy int

const (
	killUnset killStrategy = iota
	killDirect
	killReelevated
)

// String names the strategy for the log record the kill path writes, so an
// operator can tell a re-elevated kill from a direct one after the fact.
func (k killStrategy) String() string {
	switch k {
	case killDirect:
		return "direct"
	case killReelevated:
		return "reelevated"
	default:
		return "unset"
	}
}

// stagingRequest carries what the start phase needs to build the staged copy
// inside the privilege window: the verified identity to copy from and the
// run-as credential whose gid the copy is chgrp'd to. The resolved path the
// child sees as argv[0] is not here -- prepareCommand has already written it
// into execCmd.Args, and a second copy nothing reads would only invite the two
// to disagree.
type stagingRequest struct {
	identity *risktypes.VerifiedIdentity
	cred     *syscall.Credential
}

// privilegeWindow records one closed privilege window so the caller can fold
// it into the audit metrics. The supervision phase opens windows of its own
// (the kill window) but must not measure them into the metrics itself: the
// metrics belong to executeWithUserGroup, which is the only caller that has
// an audit logger to report them to.
type privilegeWindow struct {
	op       runnertypes.Operation
	duration time.Duration
}

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

	// stage carries what the start phase needs to build the staged copy;
	// nil unless binding == bindingStagedCopy.
	stage *stagingRequest

	// stagingCleanup removes the staged copy's directory; nil until the start
	// phase has built the staged copy.
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

	// killElevation identifies the command in the elevation context the kill
	// window opens with; the zero value unless kill == killReelevated. Its
	// Operation is stamped at the kill site rather than here, since that is
	// where a window's purpose is decided.
	killElevation runnertypes.ElevationContext

	// supervisedInsideStartWindow declares that the supervision phase runs
	// while the start window is still open, which is how executeWithUserGroup
	// calls it until that window is narrowed to startPrepared. The process
	// euid is already 0 there, so a killReelevated kill signals the child
	// directly: asking for a second window would be refused as re-entrant and
	// the child would never be signalled at all. Narrowing the window deletes
	// this field along with the case it describes.
	supervisedInsideStartWindow bool

	// privilegeWindows records the windows the supervision phase opened, for
	// the caller to fold into its audit metrics once they have closed.
	privilegeWindows []privilegeWindow

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

// commandOutcome collects everything superviseCommand learns about the child.
//
// reaped is false when the child did not exit within killGraceDelay of the
// kill. The wait goroutine is then still inside Wait(), which will write
// execCmd.ProcessState, so nothing may read execCmd from that point on -- the
// exit code becomes ExitCodeUnknown and the pid recorded before the kill is
// the only handle left on the child.
type commandOutcome struct {
	waitErr error
	// ctxErr is non-nil only when the run was ended by cancellation.
	ctxErr   error
	killErr  error
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
// It deliberately does not use exec.CommandContext: cancellation is handled
// by superviseCommand's select, which can wrap the kill in a privilege window
// when the child runs under a different uid. CommandContext's watchdog
// goroutine cannot, and its kill would fail with EPERM.
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
		pc.execCmd = exec.Command(childPath, cmd.ExpandedArgs...)
		pc.execCmd.Args[0] = path // present the resolved path as argv[0] to the child
		pc.execCmd.ExtraFiles = []*os.File{extraFile}
		pc.verifiedFD = extraFile
	case identity != nil && identity.FD != nil:
		pc.binding = bindingStagedCopy
		// The staged copy is created by the start phase, so its path does not
		// exist yet. exec.Command would run LookPath over it and record the
		// failure in Cmd.Err, which Start reports before it looks at Path at
		// all -- hence the struct literal. Only Path is left for the start
		// phase to fill in; Args[0] is the resolved path either way.
		pc.execCmd = &exec.Cmd{Args: append([]string{path}, cmd.ExpandedArgs...)}
		pc.stage = &stagingRequest{identity: identity, cred: cred}
	default:
		pc.binding = bindingResolvedPath
		// #nosec G204 - The command and arguments are validated before execution with e.Validate()
		pc.execCmd = exec.Command(path, cmd.ExpandedArgs...)
	}

	applyCredential(pc.execCmd, cred)
	if cred != nil {
		pc.kill = killReelevated
		pc.killElevation = runnertypes.ElevationContext{
			CommandName: cmd.Name(),
			FilePath:    cmd.ExpandedCmd,
			RunAsUser:   cmd.RunAsUser(),
			RunAsGroup:  cmd.RunAsGroup(),
		}
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

	// Last thing before the start phase: an already-cancelled context must not
	// start a process. exec.CommandContext used to refuse this; the check is
	// here now, as the last step of the prepare phase, so a cancelled run
	// starts nothing and leaves no descriptor behind. It opens no privilege
	// window of its own; for run-as execution the start window still wraps
	// this whole phase, so the check moves outside it only once the window is
	// narrowed to startPrepared. A cancellation between this check and Start
	// still starts the child, exactly as before -- the supervision phase's
	// select picks it up and kills it.
	if err := ctx.Err(); err != nil {
		return fail(err)
	}

	return pc, nil
}

// startPrepared builds the staged copy (staging fallback only) and starts the
// child process. started is true once Start has succeeded; the caller must
// proceed to supervise the child (reap it and collect its output) whenever
// started is true, regardless of err, since a running child must not be
// abandoned.
//
// The staged copy is created here rather than in the prepare phase because
// this is the function the privilege window wraps: a copy made outside the
// window is owned by the invoking user, who is the attacker in this project's
// threat model and could swap its contents between creation and exec.
func (e *DefaultExecutor) startPrepared(pc *preparedCommand) (started bool, err error) {
	if pc.spent {
		return false, ErrPreparedCommandSpent
	}
	switch pc.binding {
	case bindingStagedCopy:
		if err := e.stagePrepared(pc); err != nil {
			return false, err
		}
	case bindingVerifiedFD, bindingResolvedPath:
	default:
		return false, ErrExecBindingUnset
	}

	if err := pc.execCmd.Start(); err != nil {
		return false, err
	}
	return true, nil
}

// stagePrepared copies the verified inode into a private file and points
// execCmd at it. The warning stageFromFD cannot log from inside the window is
// carried on pc either way, including when staging itself failed.
func (e *DefaultExecutor) stagePrepared(pc *preparedCommand) error {
	stagedPath, cleanupFn, warn, err := e.stageFromFD(pc.stage.identity, pc.stage.cred)
	pc.stagingWarn = warn
	if err != nil {
		// stageFromFD has already removed its staging directory.
		return err
	}
	pc.stagingCleanup = cleanupFn
	// #nosec G204 - stagedPath is a private copy of the verified inode.
	pc.execCmd.Path = stagedPath
	return nil
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
// Several log records still run inside the privilege window for run-as
// execution: prepareCommand's "Executing command" debug record, the
// "Command execution failed" records here and in superviseCommand, and
// superviseCommand's kill and drain records. They leave the window when it is
// narrowed to startPrepared.
//
// For the same reason a run-as child is killed directly rather than through
// the kill window in this phase: the start window is still open around this
// call, so the process is already at euid 0 and a second window would be
// refused as re-entrant. See preparedCommand.supervisedInsideStartWindow.
func (e *DefaultExecutor) runCommand(ctx context.Context, pc *preparedCommand) (*Result, error) {
	// Checked before the pump is touched, not left to startPrepared's own
	// guard: a spent preparedCommand has already released its pump, so the
	// releaseChildEnds below would nil-deref before that guard's error could
	// be reported.
	if pc.spent {
		return e.reportStartFailure(pc, ErrPreparedCommandSpent)
	}

	started, startErr := e.startPrepared(pc)
	closeErr := pc.pump.releaseChildEnds()

	if !started {
		return e.reportStartFailure(pc, errors.Join(startErr, closeErr))
	}

	return e.superviseCommand(ctx, pc, closeErr)
}

// reportStartFailure releases everything the prepare phase acquired and builds
// the placeholder Result a run that never started has always reported.
func (e *DefaultExecutor) reportStartFailure(pc *preparedCommand, startErr error) (*Result, error) {
	combinedErr := errors.Join(startErr, pc.release())
	result := &Result{ExitCode: ExitCodeUnknown}
	e.Logger.Error("Command execution failed",
		"error", combinedErr,
		"command", pc.cmdLine,
		"exit_code", result.ExitCode)
	return result, fmt.Errorf("command execution failed: %w", combinedErr)
}

// superviseCommand reaps the child, reads its output and builds the Result,
// killing the child first if the run was cancelled.
//
// Three semantics inherited from exec.CommandContext and Cmd.Output are kept
// deliberately, and are worth naming because nothing else enforces them now
// that os/exec no longer handles cancellation:
//   - An already-cancelled context starts no process at all. prepareCommand
//     checks for that as its last step, so such a run reaches neither Start
//     nor the kill path. (It still runs inside the start window for run-as
//     execution in this phase; the check leaves the window when the window is
//     narrowed to startPrepared.)
//   - Process.Kill returning os.ErrProcessDone is not a failure: the child
//     exited between the cancellation being observed and the signal being
//     sent.
//   - Both waits after a kill are bounded by killGraceDelay. os/exec's
//     WaitDelay defaults to no limit, which would let a child that cannot be
//     reaped, or a grandchild holding the pipe's write end, stretch the run
//     past its timeout indefinitely -- the timeout guarantee would be lost
//     silently.
//
// startupErr carries a failure from the start phase -- today only a failure to
// release the pipe write ends, since runCommand still runs inside the privilege
// window and never sees the elevation's own error; an elevation failure after
// Start succeeded reaches here too once the window is narrowed to
// startPrepared. It forces the kill path -- a started child must not be
// abandoned -- and is joined into the returned error.
//
// Its two log records, like runCommand's, still run inside the privilege
// window for run-as execution; they leave it when the window is narrowed to
// startPrepared.
func (e *DefaultExecutor) superviseCommand(ctx context.Context, pc *preparedCommand, startupErr error) (*Result, error) {
	// Discarded rather than logged: superviseCommand still runs inside the
	// privilege window for run-as execution. release records each failure on
	// pc for logDeferredWarnings.
	defer func() { _ = pc.release() }()

	// Recorded while the child is known to be live. Once the reap gives up,
	// the wait goroutine is still inside Wait() writing execCmd.ProcessState,
	// so execCmd must not be read from that point on; these two are the only
	// handles on the child that stay valid.
	proc := pc.execCmd.Process
	pid := proc.Pid

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

	var outcome commandOutcome
	if startupErr == nil {
		select {
		case <-ctx.Done():
			outcome.ctxErr = ctx.Err()
		case outcome.waitErr = <-waitCh:
			outcome.reaped = true
		}
	}

	killed := !outcome.reaped
	if killed {
		outcome.killErr = e.killChild(pc, proc, pid)
		if outcome.killErr == nil {
			// Only on success: a failed kill leaves the child running, which
			// the Error record below reports. Claiming "killed" here as well
			// would have an operator triaging a stuck child read two records
			// that contradict each other.
			e.Logger.Info(killedMessage(outcome.ctxErr),
				"command", pc.cmdLine,
				"pid", pid,
				"kill_strategy", pc.kill.String())
		}

		timer := time.NewTimer(e.effectiveKillGraceDelay())
		select {
		case outcome.waitErr = <-waitCh:
			outcome.reaped = true
		case <-timer.C:
		}
		timer.Stop()
	}

	// Draining is bounded only after a kill: a run that ended on its own has
	// no reason to abandon output, and an unbounded wait is what os/exec did.
	drainDeadline := time.Duration(0)
	if killed {
		drainDeadline = e.effectiveKillGraceDelay()
	}
	if !outcome.reaped {
		// The child that could not be reaped is the one holding the pipe's
		// write end, so no further output can arrive. Closing the read ends
		// lets the drain finish now rather than running out a second full
		// killGraceDelay for nothing.
		_ = pc.pump.closeReadEnds()
	}
	var drainTimedOut bool
	outcome.stdout, outcome.stderr, outcome.writeErr, drainTimedOut = pc.pump.wait(drainDeadline)

	// A child that could not be reaped may still be running under another uid,
	// so it is reported; the deferred release closes the pipe read ends the
	// pump gave up on. Draining timing out on its own is a different event --
	// Wait() returned, so the exit status is known -- and is only recorded.
	var notReapedErr error
	switch {
	case !outcome.reaped:
		notReapedErr = fmt.Errorf("%w: pid=%d", ErrChildNotReaped, pid)
		e.Logger.Error("Command did not exit after kill",
			"error", notReapedErr,
			"command", pc.cmdLine,
			"pid", pid)
	case drainTimedOut:
		e.Logger.Warn("Could not finish reading command output after kill",
			"command", pc.cmdLine,
			"pid", pid)
	}

	result := &Result{Stdout: string(outcome.stdout)}
	if pc.hasOutputWriter {
		result.Stderr = string(outcome.stderr)
	} else if _, isExitError := errors.AsType[*exec.ExitError](outcome.waitErr); isExitError {
		// Match Cmd.Output: without an OutputWriter, stderr is reported
		// only when the command exited abnormally.
		result.Stderr = string(outcome.stderr)
	}
	if outcome.reaped && pc.execCmd.ProcessState != nil {
		result.ExitCode = pc.execCmd.ProcessState.ExitCode()
	} else {
		result.ExitCode = ExitCodeUnknown
	}

	cmdErr := errors.Join(rankedError(outcome), outcome.killErr, notReapedErr, startupErr)
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

// killedMessage names why the child was killed, so the record distinguishes a
// run the caller cancelled from one the start phase could not finish setting
// up.
func killedMessage(ctxErr error) string {
	if ctxErr != nil {
		return "Killed command after cancellation"
	}
	return "Killed command after start-phase failure"
}

// rankedError picks the single error that names the run's cause, in the order
// the design fixes:
//
//  1. A write error (typically the output size limit) outranks everything: it
//     is what made the reader close the pipe, and the broken-pipe exit the
//     child then reports is a consequence, not the cause.
//  2. A cancellation joins ctx.Err() with Wait()'s error, so callers can reach
//     both. os/exec drops the context error here, which is why a timed-out
//     command currently reports only "signal: killed" and cannot be told apart
//     from one an operator killed by hand.
//  3. Otherwise Wait()'s error stands on its own.
//
// ErrKillAfterCancel and ErrChildNotReaped are deliberately outside this
// ranking: the caller joins them in unconditionally, because "a process may
// still be running under another uid" must not be hidden behind whichever
// error happened to rank first.
func rankedError(outcome commandOutcome) error {
	switch {
	case outcome.writeErr != nil:
		return outcome.writeErr
	case outcome.ctxErr != nil:
		return errors.Join(outcome.ctxErr, outcome.waitErr)
	default:
		return outcome.waitErr
	}
}

// killChild stops the child after a cancellation, wrapping exactly one call --
// Process.Kill -- in the kill window when the strategy calls for it. proc and
// pid are the values recorded while the child was known to be live.
//
// A run-as child runs under a different real uid, and by the time the start
// window has closed the parent is back at the invoking user's effective uid,
// which the kernel will not let signal it. killReelevated says so; killDirect
// says the child shares the parent's uid and needs no window. Which one
// applies was declared by prepareCommand, not inferred from cred here.
func (e *DefaultExecutor) killChild(pc *preparedCommand, proc *os.Process, pid int) error {
	switch pc.kill {
	case killDirect:
		return killOutcome(proc.Kill(), pid)
	case killReelevated:
		if pc.supervisedInsideStartWindow {
			// Already at euid 0; see supervisedInsideStartWindow.
			return killOutcome(proc.Kill(), pid)
		}
		if e.PrivMgr == nil {
			return fmt.Errorf("%w: pid=%d: %w", ErrKillAfterCancel, pid, ErrNoPrivilegeManager)
		}
		elevationCtx := pc.killElevation
		elevationCtx.Operation = runnertypes.OperationKillAfterCancel

		// opened is set from inside the window, so it says a window opened
		// rather than guessing from the error which of WithPrivileges'
		// failures (re-entrancy, an unsupported operation, seteuid) happened
		// before it could. Recording a refused elevation would put a phantom
		// escalation in the audit log, which is the record the elevation
		// criteria are checked against.
		opened := false
		start := time.Now()
		err := e.PrivMgr.WithPrivileges(elevationCtx, func() error {
			opened = true
			return proc.Kill()
		})
		if opened {
			pc.privilegeWindows = append(pc.privilegeWindows, privilegeWindow{
				op:       runnertypes.OperationKillAfterCancel,
				duration: time.Since(start),
			})
		}
		return killOutcome(err, pid)
	default:
		return fmt.Errorf("%w: pid=%d", ErrKillStrategyUnset, pid)
	}
}

// killOutcome maps a kill attempt to what the run reports. A child that had
// already exited is not a failure; anything else leaves a process behind and
// is.
func killOutcome(err error, pid int) error {
	if err == nil || errors.Is(err, os.ErrProcessDone) {
		return nil
	}
	return fmt.Errorf("%w: pid=%d: %w", ErrKillAfterCancel, pid, err)
}
