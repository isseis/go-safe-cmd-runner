package resource

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Static errors
var (
	ErrOutputManagerUnavailable = errors.New("output manager not available for validation")
)

// NormalResourceManager implements ResourceManager for normal execution mode
type NormalResourceManager struct {
	// Core dependencies
	executor         executor.CommandExecutor
	fileSystem       executor.FileSystem
	privilegeManager runnertypes.PrivilegeManager
	riskEvaluator    risk.Evaluator

	// Output capture dependencies
	outputManager output.CaptureManager
	maxOutputSize int64

	// Logging
	logger *slog.Logger

	// State management
	mu       sync.RWMutex
	tempDirs []string
}

// NewNormalResourceManager creates a new NormalResourceManager for normal execution mode
func NewNormalResourceManager(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	logger *slog.Logger,
) *NormalResourceManager {
	// Delegate to NewNormalResourceManagerWithOutput with nil outputManager and 0 maxOutputSize
	return NewNormalResourceManagerWithOutput(exec, fs, privMgr, nil, 0, logger)
}

// NewNormalResourceManagerWithOutput creates a new NormalResourceManager with output capture support
func NewNormalResourceManagerWithOutput(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	outputMgr output.CaptureManager,
	maxOutputSize int64,
	logger *slog.Logger,
) *NormalResourceManager {
	return &NormalResourceManager{
		executor:         exec,
		fileSystem:       fs,
		privilegeManager: privMgr,
		riskEvaluator:    risk.NewStandardEvaluator(),
		outputManager:    outputMgr,
		maxOutputSize:    maxOutputSize,
		logger:           logger,
		tempDirs:         make([]string, 0),
	}
}

// ValidateOutputPath validates an output path before command execution
func (n *NormalResourceManager) ValidateOutputPath(outputPath, workDir string) error {
	if outputPath == "" {
		return nil // No output path to validate
	}

	if n.outputManager == nil {
		return ErrOutputManagerUnavailable
	}

	// Use the output manager's validation-only method
	return n.outputManager.ValidateOutputPath(outputPath, workDir)
}

// ExecuteCommand executes a command in normal mode
func (n *NormalResourceManager) ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (*ExecutionResult, error) {
	start := time.Now()

	// Validate command and group for consistency with dry-run mode
	if err := validateCommand(cmd); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	if err := validateCommandGroup(group); err != nil {
		return nil, fmt.Errorf("command group validation failed: %w", err)
	}

	// Unified Risk Evaluation Approach
	// Step 1: Evaluate security risk (includes privilege escalation detection)
	effectiveRisk, err := n.riskEvaluator.EvaluateRisk(cmd)
	if err != nil {
		return nil, fmt.Errorf("risk evaluation failed: %w", err)
	}

	// Step 2: Get maximum allowed risk level from configuration
	maxAllowedRisk, err := cmd.GetMaxRiskLevel()
	if err != nil {
		return nil, fmt.Errorf("invalid max_risk_level configuration: %w", err)
	}

	// Step 3: Unified risk level comparison
	if effectiveRisk > maxAllowedRisk {
		n.logger.Error("Command execution rejected due to risk level violation",
			"command", cmd.Name(),
			"cmd_binary", cmd.ExpandedCmd,
			"effective_risk", effectiveRisk.String(),
			"max_allowed_risk", maxAllowedRisk.String(),
			"command_path", group.Name,
		)
		return nil, fmt.Errorf("%w: command %s (effective risk: %s) exceeds maximum allowed risk level (%s)",
			runnertypes.ErrCommandSecurityViolation, cmd.ExpandedCmd, effectiveRisk.String(), maxAllowedRisk.String())
	}

	// Check if output capture is requested and delegate to executeCommandWithOutput
	if cmd.Spec.Output != "" && n.outputManager != nil {
		return n.executeCommandWithOutput(ctx, cmd, group, env, start)
	}

	// Execute the command using the shared execution logic
	return n.executeCommandInternal(ctx, cmd, env, start, nil)
}

// executeCommandWithOutput executes a command with output capture
func (n *NormalResourceManager) executeCommandWithOutput(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string, start time.Time) (result *ExecutionResult, err error) {
	// Prepare output capture
	maxSize := n.maxOutputSize
	if maxSize <= 0 {
		maxSize = output.DefaultMaxOutputSize // Use default from output package
	}

	capture, err := n.outputManager.PrepareOutput(cmd.Spec.Output, group.WorkDir, maxSize)
	if err != nil {
		return nil, fmt.Errorf("output capture preparation failed: %w", err)
	}

	// Ensure cleanup only on error paths
	defer func() {
		if err != nil {
			if cleanupErr := n.outputManager.CleanupOutput(capture); cleanupErr != nil {
				n.logger.Error("Failed to cleanup output capture", "error", cleanupErr, "path", cmd.Spec.Output)
			}
		}
	}()

	// Create TeeOutputWriter for both console and file output
	// Use console writer for standard output display
	consoleWriter := output.NewConsoleOutputWriter()
	teeWriter := output.NewTeeOutputWriter(capture, consoleWriter)
	defer func() {
		if closeErr := teeWriter.Close(); closeErr != nil {
			n.logger.Error("Failed to close tee writer", "error", closeErr)
			// Update the error to propagate close errors
			if err == nil {
				err = fmt.Errorf("failed to close output writer: %w", closeErr)
			}
		}
	}()

	// Execute the command using the shared execution logic with output writer
	result, err = n.executeCommandInternal(ctx, cmd, env, start, teeWriter)
	if err != nil {
		return nil, err
	}

	// Finalize output capture
	if err = n.outputManager.FinalizeOutput(capture); err != nil {
		return nil, fmt.Errorf("output capture finalization failed: %w", err)
	}

	return result, err
}

// executeCommandInternal contains the shared command execution logic
func (n *NormalResourceManager) executeCommandInternal(ctx context.Context, cmd *runnertypes.RuntimeCommand, env map[string]string, start time.Time, outputWriter executor.OutputWriter) (*ExecutionResult, error) {
	var result *executor.Result
	var err error

	// Execute command with the provided output writer
	result, err = n.executor.Execute(ctx, cmd, env, outputWriter)
	if err != nil {
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	return &ExecutionResult{
		ExitCode: result.ExitCode,
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		Duration: time.Since(start).Milliseconds(),
		DryRun:   false,
	}, nil
}

// CreateTempDir creates a temporary directory in normal mode
func (n *NormalResourceManager) CreateTempDir(groupName string) (string, error) {
	tempDir, err := n.fileSystem.CreateTempDir("", fmt.Sprintf("scr-%s-", groupName))
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	n.mu.Lock()
	n.tempDirs = append(n.tempDirs, tempDir)
	n.mu.Unlock()

	return tempDir, nil
}

// CleanupTempDir cleans up a specific temporary directory in normal mode
func (n *NormalResourceManager) CleanupTempDir(tempDirPath string) error {
	err := n.fileSystem.RemoveAll(tempDirPath)
	if err != nil {
		return fmt.Errorf("failed to cleanup temp dir %s: %w", tempDirPath, err)
	}

	// Remove from tracking
	n.mu.Lock()
	for i, dir := range n.tempDirs {
		if dir == tempDirPath {
			n.tempDirs = append(n.tempDirs[:i], n.tempDirs[i+1:]...)
			break
		}
	}
	n.mu.Unlock()

	return nil
}

// CleanupAllTempDirs cleans up all temporary directories in normal mode
func (n *NormalResourceManager) CleanupAllTempDirs() error {
	n.mu.RLock()
	tempDirs := make([]string, len(n.tempDirs))
	copy(tempDirs, n.tempDirs)
	n.mu.RUnlock()

	var errors []error

	for _, dir := range tempDirs {
		if err := n.CleanupTempDir(dir); err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("%w: %v", ErrTempDirCleanupFailed, errors)
	}

	return nil
}

// WithPrivileges executes a function with elevated privileges in normal mode
func (n *NormalResourceManager) WithPrivileges(_ context.Context, fn func() error) error {
	if n.privilegeManager == nil {
		return ErrPrivilegeManagerNotAvailable
	}
	elevationCtx := runnertypes.ElevationContext{
		// TODO: Add appropriate fields when needed
	}
	return n.privilegeManager.WithPrivileges(elevationCtx, fn)
}

// SendNotification sends a notification in normal mode
func (n *NormalResourceManager) SendNotification(_ string, _ map[string]any) error {
	// In normal mode, we would send actual notifications
	// For now, we just log the notification (no-op)
	return nil
}

// GetDryRunResults returns nil in normal mode since there are no dry-run results
func (n *NormalResourceManager) GetDryRunResults() *DryRunResult {
	return nil
}
