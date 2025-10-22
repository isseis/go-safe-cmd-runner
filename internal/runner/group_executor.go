// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
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
	validator           *security.Validator
	verificationManager *verification.Manager
	resourceManager     resource.ResourceManager
	runID               string
	notificationFunc    groupNotificationFunc
	isDryRun            bool
	keepTempDirs        bool
}

// groupNotificationFunc is a function type for sending group notifications
type groupNotificationFunc func(group *runnertypes.GroupSpec, result *groupExecutionResult, duration time.Duration)

// NewDefaultGroupExecutor creates a new DefaultGroupExecutor
func NewDefaultGroupExecutor(
	executor executor.CommandExecutor,
	config *runnertypes.ConfigSpec,
	validator *security.Validator,
	verificationManager *verification.Manager,
	resourceManager resource.ResourceManager,
	runID string,
	notificationFunc groupNotificationFunc,
	isDryRun bool,
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
	runtimeGroup, err := config.ExpandGroup(groupSpec, runtimeGlobal.ExpandedVars)
	if err != nil {
		return fmt.Errorf("failed to expand group[%s]: %w", groupSpec.Name, err)
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
				slog.Error(fmt.Sprintf("Cleanup warning: %v", err))
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
	runtimeGroup.ExpandedVars["__runner_workdir"] = workDir

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
		runtimeCmd, err := config.ExpandCommand(cmdSpec, runtimeGroup.ExpandedVars, groupSpec.Name)
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
		runtimeCmd.EffectiveWorkDir = ge.resolveCommandWorkDir(runtimeCmd, runtimeGroup)

		// 7.3 Set EffectiveTimeout
		if cmdSpec.Timeout > 0 {
			runtimeCmd.EffectiveTimeout = cmdSpec.Timeout
		} else {
			runtimeCmd.EffectiveTimeout = runtimeGlobal.Timeout()
		}

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
	envVars := executor.BuildProcessEnvironment(runtimeGlobal, runtimeGroup, cmd)

	slog.Debug("Built process environment variables",
		"command", cmd.Name(),
		"group", groupSpec.Name,
		"final_vars_count", len(envVars))

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

// createCommandContext creates a context with timeout for command execution
func (ge *DefaultGroupExecutor) createCommandContext(ctx context.Context, cmd *runnertypes.RuntimeCommand) (context.Context, context.CancelFunc) {
	timeout := time.Duration(cmd.EffectiveTimeout) * time.Second
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

		slog.Info(fmt.Sprintf(
			"Using group workdir for '%s': %s",
			runtimeGroup.Spec.Name, expandedWorkDir,
		))
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
func (ge *DefaultGroupExecutor) resolveCommandWorkDir(
	runtimeCmd *runnertypes.RuntimeCommand,
	runtimeGroup *runnertypes.RuntimeGroup,
) string {
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
			// Log error but fall back to group workdir
			slog.Error(fmt.Sprintf("Failed to expand command workdir, using group workdir: %v", err))
			return runtimeGroup.EffectiveWorkDir
		}
		return expandedWorkDir
	}

	// Priority 2: Group-level EffectiveWorkDir
	// Note: Already determined and set in ExecuteGroup by resolveGroupWorkDir
	//       (either a temporary directory or a fixed directory physical/virtual path)
	return runtimeGroup.EffectiveWorkDir
}
