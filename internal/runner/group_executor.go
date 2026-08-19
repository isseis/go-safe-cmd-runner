// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/variable"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/debuginfo"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// CommandExecutionError adds the group and command names to a command execution
// error while preserving the original error chain.
type CommandExecutionError struct {
	GroupName   string
	CommandName string
	Err         error
}

func (e *CommandExecutionError) Error() string {
	return fmt.Sprintf("command %s in group %s failed: %v", e.CommandName, e.GroupName, e.Err)
}

func (e *CommandExecutionError) Unwrap() error {
	return e.Err
}

// ErrDirPermViolation is returned when the group-level directory permission audit
// detects violations.
var ErrDirPermViolation = errors.New("TOCTOU permission check failed")

// errUnhandledCheckSkipReason is returned when path classification reports a
// reason this package has no case for: an unknown reason says neither "check
// this" nor "leave it out", so refusing to run is the only safe reading.
// Unreachable today, since ClassifyCheckTarget's codomain is the cases handled
// below; it exists for the case that changes.
var errUnhandledCheckSkipReason = errors.New("unhandled permission check skip reason")

// GroupExecutor defines the interface for executing command groups
type GroupExecutor interface {
	// ExecuteGroup executes all commands in a group sequentially
	ExecuteGroup(ctx context.Context, groupSpec *runnertypes.GroupSpec, runtimeGlobal *runnertypes.RuntimeGlobal) error
}

// DefaultGroupExecutor is the default implementation of GroupExecutor
type DefaultGroupExecutor struct {
	executor            executor.CommandExecutor
	config              *runnertypes.ConfigSpec
	validator           security.ValidatorInterface
	verificationManager verification.ManagerInterface
	resourceManager     resource.Manager
	runID               string
	notificationFunc    groupNotificationFunc
	isDryRun            bool
	dryRunDetailLevel   resource.DryRunDetailLevel
	dryRunFormat        resource.OutputFormat
	dryRunShowSensitive bool
	keepTempDirs        bool
	securityLogger      *logging.SecurityLogger
	currentUser         string
	dirPermAuditor      isec.DirectoryPermChecker
}

// groupNotificationFunc is a function type for sending group notifications
type groupNotificationFunc func(group *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration)

// NewDefaultGroupExecutor creates a new DefaultGroupExecutor with the specified
// configuration and optional settings.
func NewDefaultGroupExecutor(
	executor executor.CommandExecutor,
	config *runnertypes.ConfigSpec,
	validator security.ValidatorInterface,
	verificationManager verification.ManagerInterface,
	resourceManager resource.Manager,
	runID string,
	options ...GroupExecutorOption,
) *DefaultGroupExecutor {
	if config == nil {
		panic("NewDefaultGroupExecutor: config cannot be nil")
	}
	if resourceManager == nil {
		panic("NewDefaultGroupExecutor: resourceManager cannot be nil")
	}
	if runID == "" {
		panic("NewDefaultGroupExecutor: runID cannot be empty")
	}

	opts := defaultGroupExecutorOptions()
	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	isDryRun := opts.dryRunOptions != nil
	dryRunDetailLevel := resource.DetailLevelSummary
	dryRunFormat := resource.OutputFormatText
	var showSensitive bool

	if isDryRun {
		dryRunDetailLevel = opts.dryRunOptions.DetailLevel
		dryRunFormat = opts.dryRunOptions.OutputFormat
		showSensitive = opts.dryRunOptions.ShowSensitive
	}

	secLogger := opts.securityLogger
	if secLogger == nil {
		secLogger = logging.NewSecurityLogger()
	}

	return &DefaultGroupExecutor{
		executor:            executor,
		config:              config,
		validator:           validator,
		verificationManager: verificationManager,
		resourceManager:     resourceManager,
		runID:               runID,
		notificationFunc:    opts.notificationFunc,
		isDryRun:            isDryRun,
		dryRunDetailLevel:   dryRunDetailLevel,
		dryRunFormat:        dryRunFormat,
		dryRunShowSensitive: showSensitive,
		keepTempDirs:        opts.keepTempDirs,
		securityLogger:      secLogger,
		currentUser:         opts.currentUser,
		dirPermAuditor:      opts.dirPermAuditor,
	}
}

// ExecuteGroup executes all commands in a group sequentially
func (ge *DefaultGroupExecutor) ExecuteGroup(ctx context.Context, groupSpec *runnertypes.GroupSpec, runtimeGlobal *runnertypes.RuntimeGlobal) error {
	startTime := time.Now()

	if groupSpec.Description != "" {
		slog.Info("Executing group", slog.String("name", groupSpec.Name), slog.String("description", groupSpec.Description))
	} else {
		slog.Info("Executing group", slog.String("name", groupSpec.Name))
	}

	runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)
	if err != nil {
		return fmt.Errorf("failed to expand group[%s]: %w", groupSpec.Name, err)
	}

	if ge.isDryRun {
		ge.outputDryRunDebugInfo(groupSpec, runtimeGroup, runtimeGlobal)
	}

	// Deferred so the notification is sent on failure as well as success.
	var executionResult *groupExecutionResult
	defer func() {
		if executionResult != nil && ge.notificationFunc != nil {
			ge.notificationFunc(groupSpec, executionResult, time.Since(startTime))
		}
	}()

	workDir, tempDirMgr, err := ge.resolveGroupWorkDir(runtimeGroup)
	if err != nil {
		return fmt.Errorf("failed to resolve work directory: %w", err)
	}

	if tempDirMgr != nil && !ge.keepTempDirs {
		defer func() {
			if err := tempDirMgr.Cleanup(); err != nil {
				slog.Warn("Cleanup warning", slog.Any("error", err))
			}
		}()
	}

	runtimeGroup.EffectiveWorkDir = workDir

	// Exposed as a variable so commands can reference it as %{__runner_workdir}.
	if runtimeGroup.ExpandedVars == nil {
		runtimeGroup.ExpandedVars = make(map[string]string)
	}
	runtimeGroup.ExpandedVars[variable.WorkDirKey()] = workDir

	if err := ge.preExpandCommands(groupSpec, runtimeGroup, runtimeGlobal); err != nil {
		return fmt.Errorf("failed to pre-expand commands for group[%s]: %w", groupSpec.Name, err)
	}

	// Runs after expansion: group-level verify_files and command paths may hold
	// %{GROUP_VAR} references that the startup check could not resolve.
	if err := ge.auditGroupDirPermissions(runtimeGroup); err != nil {
		return err
	}

	if err := ge.verifyGroupFiles(runtimeGroup, runtimeGlobal); err != nil {
		return err
	}

	commandResults, errResult, err := ge.executeAllCommands(ctx, groupSpec, runtimeGroup, runtimeGlobal)
	if err != nil {
		executionResult = errResult
		return err
	}

	executionResult = &groupExecutionResult{
		status:   GroupExecutionStatusSuccess,
		commands: commandResults,
		errorMsg: "",
	}

	slog.Info("Group completed successfully", slog.String("name", groupSpec.Name))
	return nil
}

// executeAllCommands executes all commands in a group sequentially.
// executionResult is non-nil only when an error occurs, representing the error state.
func (ge *DefaultGroupExecutor) executeAllCommands(
	ctx context.Context,
	groupSpec *runnertypes.GroupSpec,
	runtimeGroup *runnertypes.RuntimeGroup,
	runtimeGlobal *runnertypes.RuntimeGlobal,
) (common.CommandResults, *groupExecutionResult, error) {
	commandResults := make(common.CommandResults, 0, len(runtimeGroup.Commands))

	for i, runtimeCmd := range runtimeGroup.Commands {
		slog.Info("Executing command", slog.String("command", runtimeCmd.Spec.Name), slog.Int("index", i+1), slog.Int("total", len(runtimeGroup.Commands)))

		stdout, stderr, exitCode, err := ge.executeSingleCommand(ctx, runtimeCmd, groupSpec, runtimeGroup, runtimeGlobal)

		// First layer of the output redaction defense-in-depth: the stored result
		// reaches logs and Slack, and the second layer (RedactingHandler) only sees
		// LogValuer types during logging.
		// See docs/tasks/0055_command_output_redaction_for_slack.
		sanitizedStdout := ge.validator.SanitizeOutputForLogging(stdout)
		sanitizedStderr := ge.validator.SanitizeOutputForLogging(stderr)

		cmdResult := common.CommandResult{
			CommandResultFields: common.CommandResultFields{
				Name:     runtimeCmd.Spec.Name,
				ExitCode: exitCode,
				Output:   sanitizedStdout,
				Stderr:   sanitizedStderr,
			},
		}
		commandResults = append(commandResults, cmdResult)

		if err != nil {
			errResult := &groupExecutionResult{
				status:   GroupExecutionStatusError,
				commands: commandResults,
				errorMsg: err.Error(),
			}
			return commandResults, errResult, err
		}
	}

	return commandResults, nil, nil
}

// preExpandCommands expands every command of the group into
// runtimeGroup.Commands, resolving each one's working directory as it goes.
//
// Expanding up front rather than per command at execution time gives
// verification the same variables execution will see (including command-level
// vars and env_import), and surfaces expansion and workdir errors before the
// first command runs.
func (ge *DefaultGroupExecutor) preExpandCommands(
	groupSpec *runnertypes.GroupSpec,
	runtimeGroup *runnertypes.RuntimeGroup,
	runtimeGlobal *runnertypes.RuntimeGlobal,
) error {
	runtimeGroup.Commands = make([]*runnertypes.RuntimeCommand, 0, len(groupSpec.Commands))

	globalOutputSizeLimit := common.NewOutputSizeLimitFromPtr(runtimeGlobal.Spec.OutputSizeLimit)

	for i := range groupSpec.Commands {
		cmdSpec := &groupSpec.Commands[i]

		runtimeCmd, err := config.ExpandCommand(
			cmdSpec,
			ge.config.CommandTemplates,
			runtimeGroup,
			runtimeGlobal,
			runtimeGlobal.Timeout(),
			globalOutputSizeLimit,
		)
		if err != nil {
			return fmt.Errorf("command[%s] (index %d): %w", cmdSpec.Name, i, err)
		}

		workDir, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
		if err != nil {
			return fmt.Errorf("command[%s] (index %d): failed to resolve workdir: %w", cmdSpec.Name, i, err)
		}
		runtimeCmd.EffectiveWorkDir = workDir

		runtimeGroup.Commands = append(runtimeGroup.Commands, runtimeCmd)
	}

	return nil
}

// auditGroupDirPermissions audits directory permissions using the fully-expanded
// group paths. It is a no-op when dirPermAuditor is nil.
// Returns an error if any violation is detected.
func (ge *DefaultGroupExecutor) auditGroupDirPermissions(runtimeGroup *runnertypes.RuntimeGroup) error {
	if ge.dirPermAuditor == nil {
		return nil
	}

	// Every path here has been through preExpandCommands, so PathExpanded is a
	// fact rather than a guess, unlike at startup where raw templates also arrive.
	// Declaring it matters: a "%{" surviving expansion is a literal an escape
	// produced, and dropping such a path would silently narrow a check that exists
	// to fail closed -- silently, because this call site keeps no exclusion counts.
	candidates := make([]string, 0, len(runtimeGroup.ExpandedVerifyFiles)+len(runtimeGroup.Commands))
	candidates = append(candidates, runtimeGroup.ExpandedVerifyFiles...)
	for _, cmd := range runtimeGroup.Commands {
		candidates = append(candidates, cmd.Cmd())
	}

	filePaths := make([]string, 0, len(candidates))
	for _, p := range candidates {
		switch reason := isec.ClassifyCheckTarget(p, isec.PathExpanded); reason {
		case isec.CheckSkipNone:
			filePaths = append(filePaths, p)
		case isec.CheckSkipRelative:
			// Not anchored to this process's working directory, so there is no
			// tree to check.
		default:
			// See errUnhandledCheckSkipReason.
			return fmt.Errorf("%w: %d for path %s", errUnhandledCheckSkipReason, reason, p)
		}
	}

	// Resolution logs a WARN for a path it cannot resolve while still handing back
	// something checkable, so no path leaves the check without a trace.
	resolved, _ := isec.ResolveAllForCheck(filePaths, slog.Default())

	// hashDir is already checked at startup; pass none to skip re-traversal.
	dirs := isec.CollectPermissionCheckDirs(resolved, nil)
	violations := isec.AuditDirectoryPermissions(ge.dirPermAuditor, dirs, slog.Default()).Violations
	if len(violations) > 0 {
		return fmt.Errorf("%w for group[%s]: %d directory violation(s) detected; review directory permissions",
			ErrDirPermViolation, runnertypes.ExtractGroupName(runtimeGroup), len(violations))
	}
	return nil
}

// verifyGroupFiles verifies files specified in the group before execution.
// After successful verification it copies the computed content hashes into each
// RuntimeCommand so that downstream ELF analysis can skip re-hashing the binary.
func (ge *DefaultGroupExecutor) verifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup, runtimeGlobal *runnertypes.RuntimeGlobal) error {
	if ge.verificationManager == nil {
		return nil
	}

	input := &verification.GroupVerificationInput{
		Name:                runnertypes.ExtractGroupName(runtimeGroup),
		ExpandedVerifyFiles: runtimeGroup.ExpandedVerifyFiles,
		Commands:            make([]verification.CommandEntry, 0, len(runtimeGroup.Commands)),
	}
	for _, cmd := range runtimeGroup.Commands {
		input.Commands = append(input.Commands, verification.CommandEntry{ExpandedCmd: cmd.ExpandedCmd})
	}

	result, err := ge.verificationManager.VerifyGroupFiles(input)
	if err != nil {
		return err
	}

	groupName := runnertypes.ExtractGroupName(runtimeGroup)

	if result.TotalFiles > 0 {
		slog.Info("Group file verification completed",
			"group", groupName,
			"verified_files", result.VerifiedFiles,
			"duration_ms", result.Duration.Milliseconds())
	}

	// One pass rather than three: the path is resolved once per command, and the
	// three concerns below cannot drift apart in how they handle errors.
	for _, cmd := range runtimeGroup.Commands {
		resolvedPath, resolveErr := ge.verificationManager.ResolvePath(cmd.ExpandedCmd)
		if resolveErr != nil {
			return fmt.Errorf("command path resolution failed for %q: %w", cmd.ExpandedCmd, resolveErr)
		}

		// Pinned once, here: the risk evaluator binds this exact path and inode for
		// fd-bound execution, so re-resolving before exec would reopen a TOCTOU
		// window between verification and execution.
		cmd.ExpandedCmd = resolvedPath

		if hash, ok := result.ContentHashes[resolvedPath]; ok {
			cmd.ExpandedCmdContentHash = hash
		}

		if dlErr := ge.verificationManager.VerifyCommandDynLibDeps(resolvedPath); dlErr != nil {
			slog.Error("Dynamic library verification failed",
				"group", groupName,
				"command", resolvedPath,
				"error", dlErr)
			return dlErr
		}

		finalEnv := executor.EnvVarValues(executor.BuildProcessEnvironment(runtimeGlobal, runtimeGroup, cmd))
		if siErr := ge.verificationManager.VerifyCommandShebangInterpreter(resolvedPath, finalEnv); siErr != nil {
			slog.Error("Shebang interpreter verification failed",
				"group", groupName,
				"command", resolvedPath,
				"error", siErr)
			return siErr
		}
	}

	return nil
}

// outputDryRunDebugInfo outputs debug information in dry-run mode
func (ge *DefaultGroupExecutor) outputDryRunDebugInfo(groupSpec *runnertypes.GroupSpec, runtimeGroup *runnertypes.RuntimeGroup, runtimeGlobal *runnertypes.RuntimeGlobal) {
	analysis := debuginfo.CollectInheritanceAnalysis(
		runtimeGlobal,
		runtimeGroup,
		ge.dryRunDetailLevel,
	)

	if ge.dryRunFormat == resource.OutputFormatJSON {
		// JSON output is assembled by the ResourceManager, so it is recorded rather
		// than printed here.
		debugInfo := &resource.DebugInfo{
			InheritanceAnalysis: analysis,
		}
		err := ge.resourceManager.RecordGroupAnalysis(groupSpec.Name, debugInfo)
		if err != nil {
			// Debug info is not worth aborting the run for.
			slog.Warn("Failed to record group analysis", slog.Any("error", err), slog.String("group", groupSpec.Name))
		}
	} else {
		fmt.Fprintf(os.Stdout, "\n===== Variable Expansion Debug Information =====\n\n") //nolint:errcheck
		output := debuginfo.FormatInheritanceAnalysisText(analysis, groupSpec.Name)
		if output != "" {
			fmt.Fprint(os.Stdout, output) //nolint:errcheck
		}
	}
}

// executeCommandInGroup executes a command within a specific group context.
//
// In dry-run mode the debug information is filled in in two phases: ExecuteCommand
// records the core analysis and returns a token, then UpdateCommandDebugInfo adds
// the environment details under that token. The split keeps the ResourceManager
// interface taking plain values, while the origin metadata the debug output needs
// exists only here in the caller.
func (ge *DefaultGroupExecutor) executeCommandInGroup(ctx context.Context, cmd *runnertypes.RuntimeCommand, groupSpec *runnertypes.GroupSpec, runtimeGroup *runnertypes.RuntimeGroup, runtimeGlobal *runnertypes.RuntimeGlobal) (*executor.Result, error) {
	// envMap carries origin metadata alongside each value; only the debug output
	// below needs it.
	envMap := executor.BuildProcessEnvironment(runtimeGlobal, runtimeGroup, cmd)

	slog.Debug("Built process environment variables",
		"command", cmd.Name(),
		"group", groupSpec.Name,
		"final_vars_count", len(envMap))

	envVars := executor.EnvVarValues(envMap)

	if err := ge.validator.ValidateAllEnvironmentVars(envVars); err != nil {
		return nil, fmt.Errorf("resolved environment variables security validation failed: %w", err)
	}

	// cmd.ExpandedCmd is deliberately not re-resolved here; see verifyGroupFiles.
	if err := ge.validator.ValidateCommandAllowed(cmd.ExpandedCmd, runtimeGroup.ExpandedCmdAllowed); err != nil {
		return nil, err
	}

	if cmd.Output() != "" {
		if err := ge.resourceManager.ValidateOutputPath(cmd.Output(), cmd.EffectiveWorkDir); err != nil {
			return nil, fmt.Errorf("output path validation failed: %w", err)
		}
	}

	// Phase 1 (see the doc comment).
	token, resourceResult, err := ge.resourceManager.ExecuteCommand(ctx, cmd, groupSpec, envVars)

	// Converted even when err is non-nil, so the caller keeps the exit code.
	var execResult *executor.Result
	if resourceResult != nil {
		execResult = &executor.Result{
			ExitCode: resourceResult.ExitCode,
			Stdout:   resourceResult.Stdout,
			Stderr:   resourceResult.Stderr,
		}
	}

	if err != nil {
		return execResult, err
	}

	// Phase 2 (see the doc comment).
	if ge.isDryRun {
		finalEnv := debuginfo.CollectFinalEnvironment(
			envMap,
			ge.dryRunDetailLevel,
			ge.dryRunShowSensitive,
		)

		if finalEnv != nil {
			if ge.dryRunFormat == resource.OutputFormatJSON {
				debugInfo := &resource.DebugInfo{
					FinalEnvironment: finalEnv,
				}
				err := ge.resourceManager.UpdateCommandDebugInfo(token, debugInfo)
				if err != nil {
					slog.Warn("Failed to update command debug info", slog.Any("error", err), slog.String("command", cmd.Name()))
				}
			} else {
				output := debuginfo.FormatFinalEnvironmentText(finalEnv)
				if output != "" {
					_, _ = fmt.Fprint(os.Stdout, output)
				}
			}
		}
	}

	return execResult, nil
}

// createCommandContext creates the context a command runs under. An
// EffectiveTimeout of 0 means unlimited execution, so the context carries no
// deadline.
func (ge *DefaultGroupExecutor) createCommandContext(ctx context.Context, cmd *runnertypes.RuntimeCommand) (context.Context, context.CancelFunc) {
	// Config validation rejects a negative timeout, so reaching here is a bug.
	if cmd.EffectiveTimeout < 0 {
		panic(fmt.Sprintf("program error: negative timeout %d for command %s",
			cmd.EffectiveTimeout, cmd.Name()))
	}

	if cmd.EffectiveTimeout <= 0 {
		ge.securityLogger.LogUnlimitedExecution(cmd.Name(), ge.currentUser)
		return context.WithCancel(ctx)
	}

	timeout := time.Duration(cmd.EffectiveTimeout) * time.Second
	slog.Debug("Command timeout configured",
		"command", cmd.Name(),
		"timeout_seconds", cmd.EffectiveTimeout)
	return context.WithTimeout(ctx, timeout)
}

// maxStdoutLengthForDebugLog is the maximum length of stdout to include in debug logs
const maxStdoutLengthForDebugLog = 500

// buildCommandDebugLogArgs builds the log arguments for a command's result:
// command name, exit code, truncated stdout, and stderr.
func buildCommandDebugLogArgs(cmdName string, result *executor.Result) []any {
	logArgs := []any{"command", cmdName}
	if result != nil {
		logArgs = append(logArgs, "exit_code", result.ExitCode)
		if result.Stdout != "" {
			logArgs = append(logArgs, "stdout", truncateStdout(result.Stdout))
		}
		if result.Stderr != "" {
			logArgs = append(logArgs, "stderr", result.Stderr)
		}
	}
	return logArgs
}

// truncateStdout truncates stdout to maxStdoutLengthForDebugLog, marking the cut
// with a "... (truncated)" suffix.
func truncateStdout(stdout string) string {
	if len(stdout) <= maxStdoutLengthForDebugLog {
		return stdout
	}
	return stdout[:maxStdoutLengthForDebugLog] + "... (truncated)"
}

// executeSingleCommand executes a single command under its own timeout context
// and returns its stdout, stderr, exit code, and any error.
func (ge *DefaultGroupExecutor) executeSingleCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, groupSpec *runnertypes.GroupSpec, runtimeGroup *runnertypes.RuntimeGroup, runtimeGlobal *runnertypes.RuntimeGlobal) (string, string, int, error) {
	cmdCtx, cancel := ge.createCommandContext(ctx, cmd)
	defer cancel()

	result, err := ge.executeCommandInGroup(cmdCtx, cmd, groupSpec, runtimeGroup, runtimeGlobal)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			ge.securityLogger.LogTimeoutExceeded(cmd.Name(), cmd.EffectiveTimeout, 0) // PID not available at this level
		}
		exitCode := executor.ExitCodeUnknown
		stderr := ""
		if result != nil {
			exitCode = result.ExitCode
			stderr = result.Stderr
		}
		// stdout is excluded to keep the error log bounded.
		errorLogArgs := []any{"command", cmd.Name(), "exit_code", exitCode, "error", err}
		if stderr != "" {
			errorLogArgs = append(errorLogArgs, "stderr", stderr)
		}
		slog.Error("Command failed", errorLogArgs...)

		return "", stderr, exitCode, &CommandExecutionError{
			GroupName:   groupSpec.Name,
			CommandName: cmd.Name(),
			Err:         err,
		}
	}

	output := ""
	if result.Stdout != "" {
		output = result.Stdout
	}

	logArgs := buildCommandDebugLogArgs(cmd.Name(), result)
	slog.Debug("Command execution result", logArgs...)

	if result.ExitCode != 0 {
		// stdout is excluded to keep the error log bounded.
		errorLogArgs := []any{"command", cmd.Name(), "exit_code", result.ExitCode}
		if result.Stderr != "" {
			errorLogArgs = append(errorLogArgs, "stderr", result.Stderr)
		}
		slog.Error("Command failed with non-zero exit code", errorLogArgs...)

		return output, result.Stderr, result.ExitCode, &CommandExecutionError{
			GroupName:   groupSpec.Name,
			CommandName: cmd.Name(),
			Err:         fmt.Errorf("%w: command %s failed with exit code %d", ErrCommandFailed, cmd.Name(), result.ExitCode),
		}
	}

	return output, result.Stderr, 0, nil
}

// resolveGroupWorkDir determines the working directory for a group.
// For fixed directories, tempDirManager is nil; for temporary directories, it is non-nil (used for cleanup).
func (ge *DefaultGroupExecutor) resolveGroupWorkDir(
	runtimeGroup *runnertypes.RuntimeGroup,
) (string, executor.TempDirManager, error) {
	if runtimeGroup.Spec.WorkDir != "" {
		// __runner_workdir is not defined yet: it is derived from what this
		// function returns.
		level := fmt.Sprintf("group[%s]", runtimeGroup.Spec.Name)
		expandedWorkDir, err := config.ExpandWorkDir(
			runtimeGroup.Spec.WorkDir,
			runtimeGroup.ExpandedVars,
			level,
		)
		if err != nil {
			return "", nil, err
		}

		slog.Info("Using group workdir",
			"group", runtimeGroup.Spec.Name,
			"workdir", expandedWorkDir)
		return expandedWorkDir, nil, nil
	}

	tempDirMgr := executor.NewTempDirManager(runtimeGroup.Spec.Name, ge.isDryRun)

	// In dry-run mode this is a virtual path; nothing is created on disk.
	tempDir, err := tempDirMgr.Create()
	if err != nil {
		return "", nil, err
	}

	return tempDir, tempDirMgr, nil
}

// resolveCommandWorkDir determines the working directory for a command:
// Command.WorkDir takes precedence over RuntimeGroup.EffectiveWorkDir. An empty
// string is a valid result and means the current directory.
func (ge *DefaultGroupExecutor) resolveCommandWorkDir(
	runtimeCmd *runnertypes.RuntimeCommand,
	runtimeGroup *runnertypes.RuntimeGroup,
) (string, error) {
	// A non-nil WorkDir wins even when it is the empty string, which is how a
	// command asks to opt out of the group's directory.
	if runtimeCmd.Spec.WorkDir != nil {
		level := fmt.Sprintf("command[%s]", runtimeCmd.Spec.Name)
		expandedWorkDir, err := config.ExpandWorkDir(
			*runtimeCmd.Spec.WorkDir,
			runtimeCmd.ExpandedVars,
			level,
		)
		if err != nil {
			return "", err
		}

		return expandedWorkDir, nil
	}

	return runtimeGroup.EffectiveWorkDir, nil
}
