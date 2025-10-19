// Package runner provides the core functionality for running commands
// in a safe and controlled manner with group-based execution and dependency management.
package runner

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// GroupExecutor defines the interface for executing command groups
type GroupExecutor interface {
	// ExecuteGroup executes all commands in a group sequentially
	ExecuteGroup(ctx context.Context, group runnertypes.CommandGroup) error
}

// DefaultGroupExecutor is the default implementation of GroupExecutor
type DefaultGroupExecutor struct {
	executor            executor.CommandExecutor
	config              *runnertypes.Config
	validator           *security.Validator
	verificationManager *verification.Manager
	resourceManager     resource.ResourceManager
	runID               string
	notificationFunc    groupNotificationFunc
}

// groupNotificationFunc is a function type for sending group notifications
type groupNotificationFunc func(group runnertypes.CommandGroup, result *groupExecutionResult, duration time.Duration)

// NewDefaultGroupExecutor creates a new DefaultGroupExecutor
func NewDefaultGroupExecutor(
	executor executor.CommandExecutor,
	config *runnertypes.Config,
	validator *security.Validator,
	verificationManager *verification.Manager,
	resourceManager resource.ResourceManager,
	runID string,
	notificationFunc groupNotificationFunc,
) *DefaultGroupExecutor {
	return &DefaultGroupExecutor{
		executor:            executor,
		config:              config,
		validator:           validator,
		verificationManager: verificationManager,
		resourceManager:     resourceManager,
		runID:               runID,
		notificationFunc:    notificationFunc,
	}
}

// ExecuteGroup executes all commands in a group sequentially
func (ge *DefaultGroupExecutor) ExecuteGroup(ctx context.Context, group runnertypes.CommandGroup) error {
	// Record execution start time for notification
	startTime := time.Now()

	if group.Description != "" {
		slog.Info("Executing group", "name", group.Name, "description", group.Description)
	} else {
		slog.Info("Executing group", "name", group.Name)
	}

	// Track temporary directories for cleanup
	groupTempDirs := make([]string, 0)

	// Explicit cleanup function to ensure resources are released as soon as
	// group execution is finished (or on early return). Previously cleanup
	// was deferred until function return which delayed releasing resources.
	cleanupGroupTempDirs := func() {
		for _, tempDirPath := range groupTempDirs {
			if err := ge.resourceManager.CleanupTempDir(tempDirPath); err != nil {
				slog.Warn("Failed to cleanup temp directory", "path", tempDirPath, "error", err)
			}
		}
	}

	// Defer notification to ensure it's sent regardless of success or failure
	var executionResult *groupExecutionResult
	defer func() {
		if executionResult != nil && ge.notificationFunc != nil {
			ge.notificationFunc(group, executionResult, time.Since(startTime))
		}
	}()

	// Process the group without template
	processedGroup := group

	// Process new fields (TempDir, Cleanup, WorkDir)
	var tempDirPath string
	if processedGroup.TempDir {
		// Create temporary directory for this group using ResourceManager
		var err error
		tempDirPath, err = ge.resourceManager.CreateTempDir(processedGroup.Name)
		if err != nil {
			return fmt.Errorf("failed to create temp directory for group %s: %w", processedGroup.Name, err)
		}
		groupTempDirs = append(groupTempDirs, tempDirPath)
	}

	// Determine and set the effective working directory for each command
	for i := range processedGroup.Commands {
		// Priority for working directory:
		// 1. Command's Dir (if set) - highest priority
		// 2. TempDir (if enabled)
		// 3. Group's WorkDir
		// 4. Global WorkDir (handled later in executeCommandInGroup)
		switch {
		case processedGroup.Commands[i].Dir != "":
			// Command has explicit Dir - use it as-is
			processedGroup.Commands[i].EffectiveWorkdir = processedGroup.Commands[i].Dir
		case tempDirPath != "":
			// Use auto-generated temp directory
			processedGroup.Commands[i].EffectiveWorkdir = tempDirPath
		case processedGroup.WorkDir != "":
			// Use group's WorkDir
			processedGroup.Commands[i].EffectiveWorkdir = processedGroup.WorkDir
		}
	}

	// Verify group files before execution
	if ge.verificationManager != nil {
		result, err := ge.verificationManager.VerifyGroupFiles(&processedGroup)
		if err != nil {
			// Ensure temp dirs are cleaned up before returning an error
			cleanupGroupTempDirs()
			// Return the error directly (it already contains all necessary information)
			return err
		}

		if result.TotalFiles > 0 {
			slog.Info("Group file verification completed",
				"group", processedGroup.Name,
				"verified_files", result.VerifiedFiles,
				"skipped_files", len(result.SkippedFiles),
				"duration_ms", result.Duration.Milliseconds())
		}
	}

	// Execute commands in the group sequentially
	var lastCommand string
	var lastOutput string
	var lastExitCode int
	for i, cmd := range processedGroup.Commands {
		slog.Info("Executing command", "command", cmd.Name, "index", i+1, "total", len(processedGroup.Commands))

		// Process the command
		processedCmd := cmd
		lastCommand = processedCmd.Name

		// Execute the command
		newOutput, exitCode, err := ge.executeSingleCommand(ctx, &processedCmd, &processedGroup)
		if err != nil {
			// Set failure result for notification
			executionResult = &groupExecutionResult{
				status:      GroupExecutionStatusError,
				exitCode:    exitCode,
				lastCommand: lastCommand,
				output:      lastOutput,
				errorMsg:    err.Error(),
			}
			// Clean up temp dirs immediately before returning to avoid
			// holding onto filesystem resources longer than necessary.
			cleanupGroupTempDirs()
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

	// Clean up temporary directories now that the group completed
	cleanupGroupTempDirs()

	slog.Info("Group completed successfully", "name", processedGroup.Name)
	return nil
}

// executeCommandInGroup executes a command within a specific group context
func (ge *DefaultGroupExecutor) executeCommandInGroup(ctx context.Context, cmd *runnertypes.Command, group *runnertypes.CommandGroup) (*executor.Result, error) {
	// Resolve environment variables for the command with group context
	envVars := executor.BuildProcessEnvironment(&ge.config.Global, group, cmd)

	slog.Debug("Built process environment variables",
		"command", cmd.Name,
		"group", group.Name,
		"final_vars_count", len(envVars))

	// Validate resolved environment variables
	if err := ge.validator.ValidateAllEnvironmentVars(envVars); err != nil {
		return nil, fmt.Errorf("resolved environment variables security validation failed: %w", err)
	}

	// Resolve and validate command path if verification manager is available
	if ge.verificationManager != nil {
		// Use ExpandedCmd if available, fallback to original Cmd
		cmdToResolve := cmd.ExpandedCmd
		if cmdToResolve == "" {
			cmdToResolve = cmd.Cmd
		}

		resolvedPath, err := ge.verificationManager.ResolvePath(cmdToResolve)
		if err != nil {
			return nil, fmt.Errorf("command path resolution failed: %w", err)
		}

		// Update the expanded command path (don't modify original)
		cmd.ExpandedCmd = resolvedPath
	}

	// Set effective working directory from global config if not already resolved
	if cmd.EffectiveWorkdir == "" {
		cmd.EffectiveWorkdir = ge.config.Global.WorkDir
	}

	// Validate output path before command execution if output capture is requested
	if cmd.Output != "" {
		if err := ge.resourceManager.ValidateOutputPath(cmd.Output, group.WorkDir); err != nil {
			return nil, fmt.Errorf("output path validation failed: %w", err)
		}
	}

	// Execute the command using ResourceManager
	result, err := ge.resourceManager.ExecuteCommand(ctx, *cmd, group, envVars)
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
func (ge *DefaultGroupExecutor) createCommandContext(ctx context.Context, cmd runnertypes.Command) (context.Context, context.CancelFunc) {
	// Use command-specific timeout if specified, otherwise use global timeout
	timeout := time.Duration(ge.config.Global.Timeout) * time.Second
	if cmd.Timeout > 0 {
		timeout = time.Duration(cmd.Timeout) * time.Second
	}

	return context.WithTimeout(ctx, timeout)
}

// executeSingleCommand executes a single command with proper context management
// Returns the output string, exit code, and any error encountered
func (ge *DefaultGroupExecutor) executeSingleCommand(ctx context.Context, cmd *runnertypes.Command, group *runnertypes.CommandGroup) (string, int, error) {
	// Create command context with timeout
	cmdCtx, cancel := ge.createCommandContext(ctx, *cmd)
	defer cancel()

	// Execute the command with group context
	result, err := ge.executeCommandInGroup(cmdCtx, cmd, group)
	if err != nil {
		slog.Error("Command failed", "command", cmd.Name, "exit_code", 1, "error", err)
		return "", 1, fmt.Errorf("command %s failed: %w", cmd.Name, err)
	}

	// Display result
	output := ""
	if result.Stdout != "" {
		output = result.Stdout
	}

	// Log command result with all relevant fields
	logArgs := []any{"command", cmd.Name, "exit_code", result.ExitCode}
	if result.Stdout != "" {
		logArgs = append(logArgs, "stdout", result.Stdout)
	}
	if result.Stderr != "" {
		logArgs = append(logArgs, "stderr", result.Stderr)
	}
	slog.Debug("Command execution result", logArgs...)

	// Check if command succeeded
	if result.ExitCode != 0 {
		slog.Error("Command failed with non-zero exit code", "command", cmd.Name, "exit_code", result.ExitCode)
		return output, result.ExitCode, fmt.Errorf("%w: command %s failed with exit code %d", ErrCommandFailed, cmd.Name, result.ExitCode)
	}

	return output, 0, nil
}
