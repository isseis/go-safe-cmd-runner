package resource

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/audit"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// Static errors
var (
	ErrOutputManagerUnavailable = errors.New("output manager not available for validation")
)

// NormalResourceManager implements Manager for normal execution mode
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
	logger      *slog.Logger
	auditLogger *audit.Logger

	// State management
	mu       sync.RWMutex
	tempDirs []string
}

// newNormalManager creates a NormalResourceManager from cfg and a resolved outputMgr.
// outputMgr is passed separately because NewDefaultResourceManager may create one
// when cfg.OutputManager is nil.
func newNormalManager(cfg Config, outputMgr output.CaptureManager) *NormalResourceManager {
	return &NormalResourceManager{
		executor:         cfg.Executor,
		fileSystem:       cfg.FileSystem,
		privilegeManager: cfg.PrivilegeManager,
		riskEvaluator:    cfg.RiskEvaluator,
		outputManager:    outputMgr,
		maxOutputSize:    cfg.MaxOutputSize,
		logger:           cfg.Logger,
		auditLogger:      cfg.AuditLogger,
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
// Returns an empty token (not used in normal mode), execution result, and error
func (n *NormalResourceManager) ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (CommandToken, *ExecutionResult, error) {
	start := time.Now()

	// Validate command and group for consistency with dry-run mode
	if err := validateCommand(cmd); err != nil {
		return "", nil, fmt.Errorf("command validation failed: %w", err)
	}

	if err := validateCommandGroup(group); err != nil {
		return "", nil, fmt.Errorf("command group validation failed: %w", err)
	}

	// Unified Risk Evaluation Approach
	// Step 1: Evaluate security risk. The plan binds the evaluated identity to the
	// executed identity and carries the effective risk and deny reasoning.
	plan, err := n.riskEvaluator.EvaluateRisk(cmd)
	if err != nil {
		// Unexpected internal failure (the error-return path of section 4(3)): emit a
		// minimal deny audit entry before aborting so a denied command is never
		// missing from the audit trail. The only error source is an
		// unclassifiable analysis record-load failure.
		n.emitErrorAudit(ctx, cmd, risktypes.ErrorClassRecordLoad)
		return "", nil, fmt.Errorf("risk evaluation failed: %w", err)
	}
	// The plan owns any verified file descriptor opened during evaluation. Close it
	// on every path -- allowed, gate-denied, or error -- so descriptors are not
	// leaked when a command is rejected after its fd was opened.
	defer func() {
		if closeErr := plan.Close(); closeErr != nil {
			n.logger.Warn("Failed to close verified command plan", "command", cmd.Name(), "error", closeErr)
		}
	}()
	effectiveRisk := plan.Assessment.Level

	// Step 2: Get maximum allowed risk level from configuration
	maxAllowedRisk, err := cmd.GetRiskLevel()
	if err != nil {
		// Configuration error: audit as a deny (classified as a risk_level config
		// error so the entry is not a reason-less deny) before aborting.
		n.auditRiskDecision(ctx, cmd, &plan, runnertypes.RiskLevelUnknown, risktypes.DecisionDeny, risktypes.ErrorClassRiskLevelConfig)
		return "", nil, fmt.Errorf("invalid risk_level configuration: %w", err)
	}

	// Step 3: Unified risk gate. A Blocking assessment (uncertain analysis,
	// symlink failure, unverified identity, disabled analysis) denies regardless
	// of the configured maximum; otherwise the effective risk must not exceed it.
	denied := plan.Assessment.Blocking || effectiveRisk > maxAllowedRisk
	decision := risktypes.DecisionAllow
	if denied {
		decision = risktypes.DecisionDeny
	}
	// Emit the command_risk_profile audit entry for both allow and deny so every
	// risk decision is auditable.
	n.auditRiskDecision(ctx, cmd, &plan, maxAllowedRisk, decision, "")

	if denied {
		n.logger.Error(
			"Command execution rejected due to risk level violation",
			"command", cmd.Name(),
			"cmd_binary", cmd.ExpandedCmd,
			"effective_risk", effectiveRisk.String(),
			"max_allowed_risk", maxAllowedRisk.String(),
			"blocking", plan.Assessment.Blocking,
			"blocking_reason", string(plan.Assessment.BlockingReason),
			"command_path", group.Name,
		)
		if plan.Assessment.Blocking {
			return "", nil, fmt.Errorf("%w: command %s denied (reason: %s)",
				runnertypes.ErrCommandSecurityViolation, cmd.ExpandedCmd, plan.Assessment.BlockingReason)
		}
		return "", nil, fmt.Errorf("%w: command %s (effective risk: %s) exceeds maximum allowed risk level (%s)",
			runnertypes.ErrCommandSecurityViolation, cmd.ExpandedCmd, effectiveRisk.String(), maxAllowedRisk.String())
	}

	// Check if output capture is requested and delegate to executeCommandWithOutput
	if cmd.Output() != "" && n.outputManager != nil {
		result, err := n.executeCommandWithOutput(ctx, &plan, cmd, env, start)
		return "", result, err
	}

	// Execute the command using the shared execution logic
	result, err := n.executeCommandInternal(ctx, &plan, cmd, env, start, nil)
	return "", result, err
}

// auditRiskDecision emits the command_risk_profile audit entry for a completed
// risk decision (allow or deny), pulling correlation fields from the plan. The
// errClass override classifies a deny that is not carried on the assessment (e.g.
// an invalid risk_level configuration); when empty the plan's own ErrorClass is
// used. It is a no-op when no audit logger is configured.
func (n *NormalResourceManager) auditRiskDecision(ctx context.Context, cmd *runnertypes.RuntimeCommand, plan *risktypes.VerifiedCommandPlan, maxAllowed runnertypes.RiskLevel, decision risktypes.Decision, errClass risktypes.ErrorClass) {
	if n.auditLogger == nil {
		return
	}
	n.auditLogger.LogRiskProfile(ctx, risktypes.RiskAuditEntry{
		CommandName:    cmd.Name(),
		Args:           cmd.ExpandedArgs,
		Mode:           risktypes.ModeNormal,
		ResolvedPath:   planResolvedPath(plan),
		ContentHash:    planContentHash(plan),
		Assessment:     plan.Assessment,
		MaxAllowedRisk: maxAllowed,
		Decision:       decision,
		ErrorClass:     cmp.Or(errClass, plan.Assessment.ErrorClass),
		Chain:          plan.Artifacts,
	})
}

// emitErrorAudit emits a minimal deny audit entry for the error-return path,
// where no plan is available. The path is recorded best-effort from the command's
// expanded path (the evaluator never resolved it on this path). It is a no-op
// when no audit logger is configured.
func (n *NormalResourceManager) emitErrorAudit(ctx context.Context, cmd *runnertypes.RuntimeCommand, errClass risktypes.ErrorClass) {
	if n.auditLogger == nil {
		return
	}
	n.auditLogger.LogRiskProfile(ctx, risktypes.RiskAuditEntry{
		CommandName: cmd.Name(),
		Args:        cmd.ExpandedArgs,
		Mode:        risktypes.ModeNormal,
		Decision:    risktypes.DecisionDeny,
		ErrorClass:  errClass,
	})
}

// planResolvedPath returns the verified resolved path for the audit entry,
// preferring the bound identity (the source of truth) and falling back to the
// plan's ResolvedPath. Returns nil when no path was resolved.
func planResolvedPath(plan *risktypes.VerifiedCommandPlan) *string {
	if plan.Identity != nil && plan.Identity.ResolvedPath != "" {
		return &plan.Identity.ResolvedPath
	}
	if plan.ResolvedPath != "" {
		return &plan.ResolvedPath
	}
	return nil
}

// planContentHash returns the verified content hash for the audit entry, taken
// from the bound identity. Returns nil when the identity was never verified.
func planContentHash(plan *risktypes.VerifiedCommandPlan) *string {
	if plan.Identity != nil && plan.Identity.ContentHash != "" {
		return &plan.Identity.ContentHash
	}
	return nil
}

// executeCommandWithOutput executes a command with output capture
func (n *NormalResourceManager) executeCommandWithOutput(ctx context.Context, plan *risktypes.VerifiedCommandPlan, cmd *runnertypes.RuntimeCommand, env map[string]string, start time.Time) (result *ExecutionResult, err error) {
	// Prepare output capture
	// Use command-level EffectiveOutputSizeLimit which already has the resolved value
	// (command-level takes precedence, falls back to global-level, then default)
	var maxSize int64
	if cmd.EffectiveOutputSizeLimit.IsUnlimited() {
		maxSize = 0 // 0 means unlimited in output manager
	} else {
		maxSize = cmd.EffectiveOutputSizeLimit.Value()
	}

	capture, err := n.outputManager.PrepareOutput(cmd.Output(), cmd.EffectiveWorkDir, maxSize)
	if err != nil {
		return nil, fmt.Errorf("output capture preparation failed: %w", err)
	}

	// Cleanup and close capture on function exit
	// Note: Capture.Close() is idempotent, so calling it multiple times is safe
	defer func() {
		// Always close the capture to ensure file is properly closed
		if closeErr := capture.Close(); closeErr != nil {
			n.logger.Error("Failed to close capture", "error", closeErr)
			// Update the error to propagate close errors
			if err == nil {
				err = fmt.Errorf("failed to close output capture: %w", closeErr)
			} else {
				err = fmt.Errorf("%w; and also failed to close output capture: %v", err, closeErr)
			}
		}

		// Cleanup temporary files only on error paths
		if err != nil {
			if cleanupErr := n.outputManager.CleanupOutput(capture); cleanupErr != nil {
				n.logger.Error("Failed to cleanup output capture", "error", cleanupErr, "path", cmd.Output())
			}
		}
	}()

	// Execute the command using the shared execution logic with capture as output writer
	result, err = n.executeCommandInternal(ctx, plan, cmd, env, start, capture)
	if err != nil {
		// Return result even on error to preserve exit code information
		return result, err
	}

	// Finalize output capture
	// Note: FinalizeOutput calls Capture.Close() internally, but since Close() is idempotent
	// this is safe even though we also call Close() in the defer above
	if err = n.outputManager.FinalizeOutput(capture); err != nil {
		// Return result even on finalization error to preserve exit code
		return result, fmt.Errorf("output capture finalization failed: %w", err)
	}

	return result, nil
}

// executeCommandInternal contains the shared command execution logic
func (n *NormalResourceManager) executeCommandInternal(ctx context.Context, plan *risktypes.VerifiedCommandPlan, cmd *runnertypes.RuntimeCommand, env map[string]string, start time.Time, outputWriter executor.OutputWriter) (*ExecutionResult, error) {
	// Execute command with the provided output writer
	result, err := n.executor.Execute(ctx, plan, cmd, env, outputWriter)

	// Always create ExecutionResult if we have a result from executor
	// This preserves exit code information even when err is non-nil
	var execResult *ExecutionResult
	if result != nil {
		execResult = &ExecutionResult{
			ExitCode: result.ExitCode,
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			Duration: time.Since(start).Milliseconds(),
			DryRun:   false,
		}
	}

	if err != nil {
		return execResult, err
	}

	return execResult, nil
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

// RecordGroupAnalysis is a no-op in normal mode
func (n *NormalResourceManager) RecordGroupAnalysis(_ string, _ *DebugInfo) error {
	return nil
}

// UpdateCommandDebugInfo is a no-op in normal mode
func (n *NormalResourceManager) UpdateCommandDebugInfo(token CommandToken, _ *DebugInfo) error {
	if token == "" {
		return ErrInvalidCommandToken
	}
	return nil
}
