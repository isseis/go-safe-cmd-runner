// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

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
	dryRunDetailLevel   resource.DetailLevel
	dryRunShowSensitive bool
	keepTempDirs        bool
}

// groupNotificationFunc is a function type for sending group notifications
type groupNotificationFunc func(group *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration)

// NewDefaultGroupExecutor creates a new DefaultGroupExecutor
func NewDefaultGroupExecutor(
	executor executor.CommandExecutor,
	config *runnertypes.ConfigSpec,
	validator security.ValidatorInterface,
	verificationManager verification.ManagerInterface,
	resourceManager resource.ResourceManager,
	runID string,
	notificationFunc groupNotificationFunc,
	isDryRun bool,
	dryRunDetailLevel resource.DetailLevel,
	dryRunShowSensitive bool,
	keepTempDirs bool,
) *DefaultGroupExecutor {
	return &DefaultGroupExecutor{
		executor:            executor,
		config:              config,
		validator:           validator,
		verificationManager: verificationManager,
		resourceManager:     resourceManager,
		runID:               runID,
		notificationFunc:    notificationFunc,
		isDryRun:            isDryRun,
		dryRunDetailLevel:   dryRunDetailLevel,
		dryRunShowSensitive: dryRunShowSensitive,
		keepTempDirs:        keepTempDirs,
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
		_, _ = fmt.Fprintf(os.Stdout, "\n===== Variable Expansion Debug Information =====\n\n")

		// Print from_env inheritance analysis
		debug.PrintFromEnvInheritance(os.Stdout, &ge.config.Global, groupSpec)
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
	if ge.verificationManager != nil {
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
	}

	// 7. Execute commands in the group sequentially
	var lastCommand string
	var lastOutput string
	var lastExitCode int
	for i := range groupSpec.Commands {
		cmdSpec := &groupSpec.Commands[i]
		slog.Info("Executing command", "command", cmdSpec.Name, "index", i+1, "total", len(groupSpec.Commands))

		// 7.1 Expand command configuration
		// Pass global timeout for timeout resolution hierarchy
		runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup, runtimeGlobal, runtimeGlobal.Timeout())
		if err != nil {
			// Set failure result for notification
			executionResult = &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    1,
				lastCommand: cmdSpec.Name,
				output:      lastOutput,
				errorMsg:    fmt.Sprintf("failed to expand command[%s]: %v", cmdSpec.Name, err),
			}
			return fmt.Errorf("failed to expand command[%s]: %w", cmdSpec.Name, err)
		}

		// 7.2 Determine effective working directory for the command
		workDir, err := ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)
		if err != nil {
			// Set failure result for notification
			executionResult = &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    1,
				lastCommand: cmdSpec.Name,
				output:      lastOutput,
				errorMsg:    fmt.Sprintf("failed to resolve command workdir[%s]: %v", cmdSpec.Name, err),
			}
			return fmt.Errorf("failed to resolve command workdir[%s]: %w", cmdSpec.Name, err)
		}
		runtimeCmd.EffectiveWorkDir = workDir

		// Note: EffectiveTimeout is already set by NewRuntimeCommand in ExpandCommand

		lastCommand = cmdSpec.Name

		// 7.4 Execute the command
		newOutput, exitCode, err := ge.executeSingleCommand(ctx, runtimeCmd, groupSpec, runtimeGroup, runtimeGlobal)
		if err != nil {
			// Set failure result for notification
			executionResult = &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    exitCode,
				lastCommand: lastCommand,
				output:      lastOutput,
				errorMsg:    err.Error(),
			}
			return err
		}

		// Update last output if command produced output
		if newOutput != "" {
			lastOutput = newOutput
		}
		lastExitCode = exitCode
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

// executeCommandInGroup executes a command within a specific group context
func (ge *DefaultGroupExecutor) executeCommandInGroup(ctx context.Context, cmd *runnertypes.RuntimeCommand, groupSpec *runnertypes.GroupSpec, runtimeGroup *runnertypes.RuntimeGroup, runtimeGlobal *runnertypes.RuntimeGlobal) (*executor.Result, error) {
	// Resolve environment variables for the command with group context
	envMap := executor.BuildProcessEnvironment(runtimeGlobal, runtimeGroup, cmd)

	slog.Debug("Built process environment variables",
		"command", cmd.Name(),
		"group", groupSpec.Name,
		"final_vars_count", len(envMap))

	// Extract values for validation
	envVars := make(map[string]string, len(envMap))
	for k, v := range envMap {
		envVars[k] = v.Value
	}

	// Validate resolved environment variables
	if err := ge.validator.ValidateAllEnvironmentVars(envVars); err != nil {
		return nil, fmt.Errorf("resolved environment variables security validation failed: %w", err)
	}

	// Print final environment in dry-run mode with full detail level
	if ge.isDryRun && ge.dryRunDetailLevel == resource.DetailLevelFull {
		debug.PrintFinalEnvironment(os.Stdout, envMap, ge.dryRunShowSensitive)
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
		if err := ge.resourceManager.ValidateOutputPath(cmd.Output(), groupSpec.WorkDir); err != nil {
			return nil, fmt.Errorf("output path validation failed: %w", err)
		}
	}

	// Execute the command using ResourceManager
	result, err := ge.resourceManager.ExecuteCommand(ctx, cmd, groupSpec, envVars)
	if err != nil {
		return nil, err
	}

	// Convert ResourceManager result to executor.Result
	return &executor.Result{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
	}, nil
}

// createCommandContext creates a context with timeout for command execution.
// If EffectiveTimeout is 0 or negative, returns a cancellable context without a deadline for unlimited execution.
// Otherwise, creates a context with the specified timeout.
func (ge *DefaultGroupExecutor) createCommandContext(ctx context.Context, cmd *runnertypes.RuntimeCommand) (context.Context, context.CancelFunc) {
	if cmd.EffectiveTimeout <= 0 {
		// Unlimited execution: return a cancellable context without a timeout
		slog.Warn("Command configured with unlimited timeout",
			"command", cmd.Name(),
			"timeout", "unlimited")
		return context.WithCancel(ctx)
	}

	// Limited execution: create a context with timeout
	timeout := time.Duration(cmd.EffectiveTimeout) * time.Second
	slog.Debug("Command timeout configured",
		"command", cmd.Name(),
		"timeout_seconds", cmd.EffectiveTimeout)
	return context.WithTimeout(ctx, timeout)
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
		slog.Error("Command failed", "command", cmd.Name(), "exit_code", 1, "error", err)
		return "", 1, fmt.Errorf("command %s failed: %w", cmd.Name(), err)
	}

	// Display result
	output := ""
	if result.Stdout != "" {
		output = result.Stdout
	}

	// Log command result with all relevant fields
	logArgs := []any{"command", cmd.Name(), "exit_code", result.ExitCode}
	if result.Stdout != "" {
		logArgs = append(logArgs, "stdout", result.Stdout)
	}
	if result.Stderr != "" {
		logArgs = append(logArgs, "stderr", result.Stderr)
	}
	slog.Debug("Command execution result", logArgs...)

	// Check if command succeeded
	if result.ExitCode != 0 {
		slog.Error("Command failed with non-zero exit code", "command", cmd.Name(), "exit_code", result.ExitCode)
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
