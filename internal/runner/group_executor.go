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

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/debug"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/variable"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

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
	resourceManager     resource.ResourceManager
	runID               string
	notificationFunc    groupNotificationFunc
	isDryRun            bool
	dryRunDetailLevel   resource.DryRunDetailLevel
	dryRunFormat        resource.OutputFormat
	dryRunShowSensitive bool
	keepTempDirs        bool
	securityLogger      *logging.SecurityLogger
	currentUser         string
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
	resourceManager resource.ResourceManager,
	runID string,
	options ...GroupExecutorOption,
) *DefaultGroupExecutor {
	// Input validation
	if config == nil {
		panic("NewDefaultGroupExecutor: config cannot be nil")
	}
	if resourceManager == nil {
		panic("NewDefaultGroupExecutor: resourceManager cannot be nil")
	}
	if runID == "" {
		panic("NewDefaultGroupExecutor: runID cannot be empty")
	}

	// Apply options
	opts := defaultGroupExecutorOptions()
	for _, opt := range options {
		if opt != nil {
			opt(opts)
		}
	}

	// Extract dry-run settings
	isDryRun := opts.dryRunOptions != nil
	dryRunDetailLevel := resource.DetailLevelSummary
	dryRunFormat := resource.OutputFormatText
	var showSensitive bool

	if isDryRun {
		dryRunDetailLevel = opts.dryRunOptions.DetailLevel
		dryRunFormat = opts.dryRunOptions.OutputFormat
		showSensitive = opts.dryRunOptions.ShowSensitive
	}

	// Create a default security logger if none provided
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
	}
}

// ExecuteGroup executes all commands in a group sequentially
func (ge *DefaultGroupExecutor) ExecuteGroup(ctx context.Context, groupSpec *runnertypes.GroupSpec, runtimeGlobal *runnertypes.RuntimeGlobal) error {
	// Record execution start time for notification
	startTime := time.Now()

	if groupSpec.Description != "" {
		slog.Info("Executing group", "name", groupSpec.Name, "description", groupSpec.Description)
	} else {
		slog.Info("Executing group", "name", groupSpec.Name)
	}

	// 1. Expand group configuration
	runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal)
	if err != nil {
		return fmt.Errorf("failed to expand group[%s]: %w", groupSpec.Name, err)
	}

	// Print debug information in dry-run mode
	if ge.isDryRun {
		ge.outputDryRunDebugInfo(groupSpec, runtimeGroup, runtimeGlobal)
	}

	// Defer notification to ensure it's sent regardless of success or failure
	var executionResult *groupExecutionResult
	defer func() {
		if executionResult != nil && ge.notificationFunc != nil {
			ge.notificationFunc(groupSpec, executionResult, time.Since(startTime))
		}
	}()

	// 2. Determine working directory for the group
	workDir, tempDirMgr, err := ge.resolveGroupWorkDir(runtimeGroup)
	if err != nil {
		return fmt.Errorf("failed to resolve work directory: %w", err)
	}

	// 3. Register cleanup for temporary directories if applicable
	if tempDirMgr != nil && !ge.keepTempDirs {
		defer func() {
			if err := tempDirMgr.Cleanup(); err != nil {
				slog.Warn("Cleanup warning", "error", err)
			}
		}()
	}

	// 4. Set effective working directory for the group
	runtimeGroup.EffectiveWorkDir = workDir

	// 5. Set __runner_workdir variable for use in commands
	// This allows commands to reference the working directory via %{__runner_workdir}
	if runtimeGroup.ExpandedVars == nil {
		runtimeGroup.ExpandedVars = make(map[string]string)
	}
	runtimeGroup.ExpandedVars[variable.WorkDirKey()] = workDir

	// 6. Verify group files before execution
	if err := ge.verifyGroupFiles(groupSpec); err != nil {
		return err
	}

	// 7. Execute commands in the group sequentially
	lastCommand, lastOutput, lastExitCode, errResult, err := ge.executeAllCommands(ctx, groupSpec, runtimeGroup, runtimeGlobal)
	if err != nil {
		// executionResult is set from the returned errResult
		executionResult = errResult
		return err
	}

	// Set success result for notification
	executionResult = &groupExecutionResult{
		status:      GroupExecutionStatusSuccess,
		exitCode:    lastExitCode,
		lastCommand: lastCommand,
		output:      lastOutput,
		errorMsg:    "",
	}

	slog.Info("Group completed successfully", "name", groupSpec.Name)
	return nil
}

// executeAllCommands executes all commands in a group sequentially
// Returns: (lastCommand, lastOutput, lastExitCode, executionResult, error)
// executionResult is non-nil only when an error occurs, representing the error state.
func (ge *DefaultGroupExecutor) executeAllCommands(
	ctx context.Context,
	groupSpec *runnertypes.GroupSpec,
	runtimeGroup *runnertypes.RuntimeGroup,
	runtimeGlobal *runnertypes.RuntimeGlobal,
) (string, string, int, *groupExecutionResult, error) {
	var lastCommand string
	var lastOutput string
	var lastExitCode int

	for i := range groupSpec.Commands {
		cmdSpec := &groupSpec.Commands[i]
		slog.Info("Executing command", "command", cmdSpec.Name, "index", i+1, "total", len(groupSpec.Commands))

		// Expand command configuration
		runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup, runtimeGlobal, runtimeGlobal.Timeout())
		if err != nil {
			// Set failure result for notification
			errResult := &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    1,
				lastCommand: cmdSpec.Name,
				output:      lastOutput,
				errorMsg:    fmt.Sprintf("failed to expand command[%s]: %v", cmdSpec.Name, err),
			}
			return lastCommand, lastOutput, 1, errResult, fmt.Errorf("failed to expand command[%s]: %w", cmdSpec.Name, err)
		}

		// Determine effective working directory for the command
		workDir, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
		if err != nil {
			// Set failure result for notification
			errResult := &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    1,
				lastCommand: cmdSpec.Name,
				output:      lastOutput,
				errorMsg:    fmt.Sprintf("failed to resolve command workdir[%s]: %v", cmdSpec.Name, err),
			}
			return lastCommand, lastOutput, 1, errResult, fmt.Errorf("failed to resolve command workdir[%s]: %w", cmdSpec.Name, err)
		}
		runtimeCmd.EffectiveWorkDir = workDir

		lastCommand = cmdSpec.Name

		// Execute the command
		newOutput, exitCode, err := ge.executeSingleCommand(ctx, runtimeCmd, groupSpec, runtimeGroup, runtimeGlobal)
		if err != nil {
			// Set failure result for notification
			errResult := &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    exitCode,
				lastCommand: lastCommand,
				output:      lastOutput,
				errorMsg:    err.Error(),
			}
			return lastCommand, lastOutput, exitCode, errResult, err
		}

		// Update last output if command produced output
		if newOutput != "" {
			lastOutput = newOutput
		}
		lastExitCode = exitCode
	}

	return lastCommand, lastOutput, lastExitCode, nil, nil
}

// verifyGroupFiles verifies files specified in the group before execution
func (ge *DefaultGroupExecutor) verifyGroupFiles(groupSpec *runnertypes.GroupSpec) error {
	if ge.verificationManager == nil {
		return nil
	}

	result, err := ge.verificationManager.VerifyGroupFiles(groupSpec)
	if err != nil {
		// Return the error directly (it already contains all necessary information)
		return err
	}

	if result.TotalFiles > 0 {
		slog.Info("Group file verification completed",
			"group", groupSpec.Name,
			"verified_files", result.VerifiedFiles,
			"skipped_files", len(result.SkippedFiles),
			"duration_ms", result.Duration.Milliseconds())
	}

	return nil
}

// outputDryRunDebugInfo outputs debug information in dry-run mode
func (ge *DefaultGroupExecutor) outputDryRunDebugInfo(groupSpec *runnertypes.GroupSpec, runtimeGroup *runnertypes.RuntimeGroup, runtimeGlobal *runnertypes.RuntimeGlobal) {
	// Collect inheritance analysis data
	analysis := debug.CollectInheritanceAnalysis(
		runtimeGlobal,
		runtimeGroup,
		ge.dryRunDetailLevel,
	)

	// Format based on output format
	if ge.dryRunFormat == resource.OutputFormatJSON {
		// Record to ResourceManager for JSON output
		debugInfo := &resource.DebugInfo{
			InheritanceAnalysis: analysis,
		}
		err := ge.resourceManager.RecordGroupAnalysis(groupSpec.Name, debugInfo)
		if err != nil {
			// Log error but continue execution
			slog.Warn("Failed to record group analysis", "error", err, "group", groupSpec.Name)
		}
	} else {
		// Text format: output immediately
		_, _ = fmt.Fprintf(os.Stdout, "\n===== Variable Expansion Debug Information =====\n\n")
		output := debug.FormatInheritanceAnalysisText(analysis, groupSpec.Name)
		if output != "" {
			_, _ = fmt.Fprint(os.Stdout, output)
		}
	}
}

// executeCommandInGroup executes a command within a specific group context
//
// Two-Phase Debug Info Update Pattern (Dry-run mode):
// This function uses a two-phase approach to populate debug information:
//
//  1. Phase 1 (ExecuteCommand): Records core command analysis and returns a token
//  2. Phase 2 (UpdateCommandDebugInfo): Adds optional debug info using the token
//
// Why this pattern is necessary:
//   - ExecuteCommand accepts env as map[string]string (values only)
//   - Debug info needs map[string]executor.EnvVar (values + origin metadata)
//   - envMap (with metadata) is only available here in the caller context
//   - ResourceManager interface stays simple and works for both normal/dry-run modes
//   - Separation of concerns: core analysis vs optional debug details
func (ge *DefaultGroupExecutor) executeCommandInGroup(ctx context.Context, cmd *runnertypes.RuntimeCommand, groupSpec *runnertypes.GroupSpec, runtimeGroup *runnertypes.RuntimeGroup, runtimeGlobal *runnertypes.RuntimeGlobal) (*executor.Result, error) {
	// Resolve environment variables for the command with group context
	// envMap contains executor.EnvVar with both value and origin metadata
	envMap := executor.BuildProcessEnvironment(runtimeGlobal, runtimeGroup, cmd)

	slog.Debug("Built process environment variables",
		"command", cmd.Name(),
		"group", groupSpec.Name,
		"final_vars_count", len(envMap))

	// Extract values for validation and ExecuteCommand
	// Note: Origin metadata is stripped here, which is why Phase 2 update is needed
	envVars := make(map[string]string, len(envMap))
	for k, v := range envMap {
		envVars[k] = v.Value
	}

	// Validate resolved environment variables
	if err := ge.validator.ValidateAllEnvironmentVars(envVars); err != nil {
		return nil, fmt.Errorf("resolved environment variables security validation failed: %w", err)
	}

	// Resolve and validate command path if verification manager is available
	if ge.verificationManager != nil {
		resolvedPath, err := ge.verificationManager.ResolvePath(cmd.ExpandedCmd)
		if err != nil {
			return nil, fmt.Errorf("command path resolution failed: %w", err)
		}

		// Update the expanded command path (don't modify original)
		cmd.ExpandedCmd = resolvedPath
	}

	// Note: EffectiveWorkDir should be set earlier in ExecuteGroup()
	// If still empty at this point, the command will use the process's current working directory

	// Validate output path before command execution if output capture is requested
	if cmd.Output() != "" {
		if err := ge.resourceManager.ValidateOutputPath(cmd.Output(), cmd.EffectiveWorkDir); err != nil {
			return nil, fmt.Errorf("output path validation failed: %w", err)
		}
	}

	// Phase 1: Execute the command using ResourceManager
	// ExecuteCommand records core analysis and returns a token for later updates
	token, resourceResult, err := ge.resourceManager.ExecuteCommand(ctx, cmd, groupSpec, envVars)

	// Convert ResourceManager result to executor.Result (even if err is non-nil)
	// This preserves exit code information for error handling
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

	// Phase 2: Update final environment debug info in dry-run mode (after command execution)
	// Uses the token to update the ResourceAnalysis with environment origin metadata
	if ge.isDryRun {
		// Collect final environment data
		finalEnv := debug.CollectFinalEnvironment(
			envMap,
			ge.dryRunDetailLevel,
			ge.dryRunShowSensitive,
		)

		if finalEnv != nil {
			if ge.dryRunFormat == resource.OutputFormatJSON {
				// Update the command's ResourceAnalysis with debug info using token
				debugInfo := &resource.DebugInfo{
					FinalEnvironment: finalEnv,
				}
				err := ge.resourceManager.UpdateCommandDebugInfo(token, debugInfo)
				if err != nil {
					slog.Warn("Failed to update command debug info", "error", err, "command", cmd.Name())
				}
			} else {
				// Text format: output immediately
				output := debug.FormatFinalEnvironmentText(finalEnv)
				if output != "" {
					_, _ = fmt.Fprint(os.Stdout, output)
				}
			}
		}
	}

	// Return the converted executor.Result
	return execResult, nil
}

// createCommandContext creates a context with timeout for command execution.
// If EffectiveTimeout is 0 or negative, returns a cancellable context without a deadline for unlimited execution.
// Otherwise, creates a context with the specified timeout.
func (ge *DefaultGroupExecutor) createCommandContext(ctx context.Context, cmd *runnertypes.RuntimeCommand) (context.Context, context.CancelFunc) {
	// Defensive check: panic if EffectiveTimeout is negative (should never happen due to config validation)
	if cmd.EffectiveTimeout < 0 {
		panic(fmt.Sprintf("program error: negative timeout %d for command %s",
			cmd.EffectiveTimeout, cmd.Name()))
	}

	if cmd.EffectiveTimeout <= 0 {
		// Unlimited execution: return a cancellable context without a timeout
		// Log security event with current user
		ge.securityLogger.LogUnlimitedExecution(cmd.Name(), ge.currentUser)
		return context.WithCancel(ctx)
	}

	// Limited execution: create a context with timeout
	timeout := time.Duration(cmd.EffectiveTimeout) * time.Second
	slog.Debug("Command timeout configured",
		"command", cmd.Name(),
		"timeout_seconds", cmd.EffectiveTimeout)
	return context.WithTimeout(ctx, timeout)
}

// maxStdoutLengthForDebugLog is the maximum length of stdout to include in debug logs
const maxStdoutLengthForDebugLog = 500

// buildCommandDebugLogArgs builds log arguments for command output logging
// Returns a slice of log arguments including command name, exit code, stdout (truncated), and stderr
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

// truncateStdout truncates stdout to the maximum length for debug logging
// If the stdout is longer than maxStdoutLengthForDebugLog, it will be truncated with "... (truncated)" suffix
func truncateStdout(stdout string) string {
	if len(stdout) <= maxStdoutLengthForDebugLog {
		return stdout
	}
	return stdout[:maxStdoutLengthForDebugLog] + "... (truncated)"
}

// executeSingleCommand executes a single command with proper context management
// Returns the output string, exit code, and any error encountered
func (ge *DefaultGroupExecutor) executeSingleCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, groupSpec *runnertypes.GroupSpec, runtimeGroup *runnertypes.RuntimeGroup, runtimeGlobal *runnertypes.RuntimeGlobal) (string, int, error) {
	// Create command context with timeout
	cmdCtx, cancel := ge.createCommandContext(ctx, cmd)
	defer cancel()

	// Execute the command with group context
	result, err := ge.executeCommandInGroup(cmdCtx, cmd, groupSpec, runtimeGroup, runtimeGlobal)
	if err != nil {
		// Check if the error is due to context deadline exceeded (timeout)
		if errors.Is(err, context.DeadlineExceeded) {
			// Log timeout exceeded event
			ge.securityLogger.LogTimeoutExceeded(cmd.Name(), cmd.EffectiveTimeout, 0) // PID not available at this level
		}
		// Use actual exit code from result if available, otherwise use ExitCodeUnknown
		exitCode := executor.ExitCodeUnknown
		if result != nil {
			exitCode = result.ExitCode
		}
		// Log command failure with stderr at ERROR level (stdout is excluded to avoid excessive logging)
		errorLogArgs := []any{"command", cmd.Name(), "exit_code", exitCode, "error", err}
		if result != nil && result.Stderr != "" {
			errorLogArgs = append(errorLogArgs, "stderr", result.Stderr)
		}
		slog.Error("Command failed", errorLogArgs...)
		return "", exitCode, fmt.Errorf("command %s failed: %w", cmd.Name(), err)
	}

	// Display result
	output := ""
	if result.Stdout != "" {
		output = result.Stdout
	}

	// Log command result with all relevant fields
	logArgs := buildCommandDebugLogArgs(cmd.Name(), result)
	slog.Debug("Command execution result", logArgs...)

	// Check if command succeeded
	if result.ExitCode != 0 {
		// Log command failure with stderr at ERROR level (stdout is excluded to avoid excessive logging)
		errorLogArgs := []any{"command", cmd.Name(), "exit_code", result.ExitCode}
		if result.Stderr != "" {
			errorLogArgs = append(errorLogArgs, "stderr", result.Stderr)
		}
		slog.Error("Command failed with non-zero exit code", errorLogArgs...)
		return output, result.ExitCode, fmt.Errorf("%w: command %s failed with exit code %d", ErrCommandFailed, cmd.Name(), result.ExitCode)
	}

	return output, 0, nil
}

// resolveGroupWorkDir determines the working directory for a group.
// Returns: (workdir, tempDirManager, error)
//   - For fixed directories: tempDirManager is nil
//   - For temporary directories: tempDirManager is non-nil (used for cleanup)
func (ge *DefaultGroupExecutor) resolveGroupWorkDir(
	runtimeGroup *runnertypes.RuntimeGroup,
) (string, executor.TempDirManager, error) {
	// Check if group-level WorkDir is specified
	if runtimeGroup.Spec.WorkDir != "" {
		// Expand variables (note: __runner_workdir is not yet defined at this point)
		level := fmt.Sprintf("group[%s]", runtimeGroup.Spec.Name)
		expandedWorkDir, err := config.ExpandString(
			runtimeGroup.Spec.WorkDir,
			runtimeGroup.ExpandedVars, // __runner_workdir is not included
			level,
			"workdir",
		)
		if err != nil {
			return "", nil, fmt.Errorf("failed to expand group workdir: %w", err)
		}

		slog.Info("Using group workdir",
			"group", runtimeGroup.Spec.Name,
			"workdir", expandedWorkDir)
		return expandedWorkDir, nil, nil
	}

	// Create temporary directory manager
	tempDirMgr := executor.NewTempDirManager(runtimeGroup.Spec.Name, ge.isDryRun)

	// Create temporary directory
	// In dry-run mode, a virtual path is returned
	tempDir, err := tempDirMgr.Create()
	if err != nil {
		return "", nil, err
	}

	return tempDir, tempDirMgr, nil
}

// resolveCommandWorkDir determines the working directory for a command.
// Priority: Command.WorkDir > RuntimeGroup.EffectiveWorkDir
// Returns (workdir, error). Returns error if variable expansion fails.
func (ge *DefaultGroupExecutor) resolveCommandWorkDir(
	runtimeCmd *runnertypes.RuntimeCommand,
	runtimeGroup *runnertypes.RuntimeGroup,
) (string, error) {
	// Priority 1: Command-level WorkDir (from spec)
	if runtimeCmd.Spec.WorkDir != "" {
		// Expand variables in command workdir
		level := fmt.Sprintf("command[%s]", runtimeCmd.Spec.Name)
		expandedWorkDir, err := config.ExpandString(
			runtimeCmd.Spec.WorkDir,
			runtimeCmd.ExpandedVars, // Use command's expanded vars
			level,
			"workdir",
		)
		if err != nil {
			return "", fmt.Errorf("failed to expand command workdir: %w", err)
		}
		return expandedWorkDir, nil
	}

	// Priority 2: Group-level EffectiveWorkDir
	// Note: Already determined and set in ExecuteGroup by resolveGroupWorkDir
	//       (either a temporary directory or a fixed directory physical/virtual path)
	return runtimeGroup.EffectiveWorkDir, nil
}
