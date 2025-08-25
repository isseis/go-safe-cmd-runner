package resource

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// NormalResourceManager implements ResourceManager for normal execution mode
type NormalResourceManager struct {
	// Core dependencies
	executor         executor.CommandExecutor
	fileSystem       executor.FileSystem
	privilegeManager runnertypes.PrivilegeManager
	riskEvaluator    risk.Evaluator

	// Phase 1: New security components
	privilegeAnalyzer security.PrivilegeEscalationAnalyzer
	securityEvaluator security.RiskEvaluator
	logger            *slog.Logger

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
	return &NormalResourceManager{
		executor:         exec,
		fileSystem:       fs,
		privilegeManager: privMgr,
		riskEvaluator:    risk.NewStandardEvaluator(),
		// Phase 1: Initialize new security components
		privilegeAnalyzer: security.NewDefaultPrivilegeEscalationAnalyzer(logger),
		securityEvaluator: security.NewDefaultRiskEvaluator(logger),
		logger:            logger,
		tempDirs:          make([]string, 0),
	}
}

// ExecuteCommand executes a command in normal mode
func (n *NormalResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error) {
	start := time.Now()

	// Validate command and group for consistency with dry-run mode
	if err := validateCommand(cmd); err != nil {
		return nil, fmt.Errorf("command validation failed: %w", err)
	}

	if err := validateCommandGroup(group); err != nil {
		return nil, fmt.Errorf("command group validation failed: %w", err)
	}

	// Unified Risk Evaluation Approach
	// Step 1: Evaluate basic security risk
	riskLevel, err := n.riskEvaluator.EvaluateRisk(&cmd)
	if err != nil {
		return nil, fmt.Errorf("risk evaluation failed: %w", err)
	}

	// Step 2: Analyze privilege escalation
	privilegeResult, err := n.privilegeAnalyzer.AnalyzePrivilegeEscalation(ctx, cmd.Cmd, cmd.Args)
	if err != nil {
		return nil, fmt.Errorf("privilege escalation analysis failed: %w", err)
	}

	// Step 3: Calculate effective risk level including privilege escalation
	effectiveRisk := n.calculateEffectiveRisk(riskLevel, privilegeResult, &cmd)

	// Step 4: Get maximum allowed risk level from configuration
	maxAllowedRisk, err := cmd.GetMaxRiskLevel()
	if err != nil {
		return nil, fmt.Errorf("invalid max_risk_level configuration: %w", err)
	}

	// Step 5: Unified risk level comparison (replaces dual control mechanism)
	if effectiveRisk > maxAllowedRisk {
		n.logger.Error("Command execution rejected due to risk level violation",
			"command", cmd.Name,
			"cmd_binary", cmd.Cmd,
			"effective_risk", effectiveRisk.String(),
			"max_allowed_risk", maxAllowedRisk.String(),
			"privilege_escalation", privilegeResult.IsPrivilegeEscalation,
			"escalation_type", privilegeResult.EscalationType,
			"command_path", group.Name,
		)
		return nil, fmt.Errorf("%w: command %s (effective risk: %s) exceeds maximum allowed risk level (%s)",
			runnertypes.ErrCommandSecurityViolation, cmd.Cmd, effectiveRisk.String(), maxAllowedRisk.String())
	}

	result, err := n.executor.Execute(ctx, cmd, env)
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

// calculateEffectiveRisk calculates the effective risk considering privilege escalation
func (n *NormalResourceManager) calculateEffectiveRisk(
	baseRisk runnertypes.RiskLevel,
	privilegeResult *security.PrivilegeEscalationResult,
	_ *runnertypes.Command,
) runnertypes.RiskLevel {
	// If no privilege escalation detected, return base risk
	if privilegeResult == nil || !privilegeResult.IsPrivilegeEscalation {
		return baseRisk
	}

	// Privilege escalation detected - classify as Critical risk unless explicitly allowed
	// Note: In this unified approach, we don't have a Privileged field yet,
	// so all privilege escalation commands are treated as Critical risk
	return runnertypes.RiskLevelCritical
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
