package resource

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security"
)

// DefaultResourceManager provides a mode-aware facade that delegates to
// NormalResourceManager or DryRunResourceManager depending on ExecutionMode.
// It implements DryRunResourceManagerInterface so callers can always query dry-run results
// (returns nil in normal mode) and record analyses (no-op in normal mode).
type DefaultResourceManager struct {
	mode   ExecutionMode
	normal *NormalResourceManager
	dryrun *DryRunResourceManager
}

// NewDefaultResourceManager creates a new DefaultResourceManager.
// If mode is ExecutionModeDryRun, opts may be used to configure the dry-run behavior.
func NewDefaultResourceManager(exec executor.CommandExecutor, fs executor.FileSystem, privMgr runnertypes.PrivilegeManager, pathResolver PathResolver, logger *slog.Logger, mode ExecutionMode, dryRunOpts *DryRunOptions) (*DefaultResourceManager, error) {
	return NewDefaultResourceManagerWithOutput(exec, fs, privMgr, pathResolver, logger, mode, dryRunOpts, nil, 0)
}

// NewDefaultResourceManagerWithOutput creates a new DefaultResourceManager with output capture support.
// If mode is ExecutionModeDryRun, opts may be used to configure the dry-run behavior.
func NewDefaultResourceManagerWithOutput(exec executor.CommandExecutor, fs executor.FileSystem, privMgr runnertypes.PrivilegeManager, pathResolver PathResolver, logger *slog.Logger, mode ExecutionMode, dryRunOpts *DryRunOptions, outputMgr output.CaptureManager, maxOutputSize int64) (*DefaultResourceManager, error) {
	// Create output manager if not provided
	if outputMgr == nil {
		// Create a security validator for output validation
		securityValidator, err := security.NewValidator(nil) // Use default config
		if err != nil {
			return nil, fmt.Errorf("failed to create security validator: %w", err)
		}
		outputMgr = output.NewDefaultOutputCaptureManager(securityValidator)
	}

	mgr := &DefaultResourceManager{
		mode:   mode,
		normal: NewNormalResourceManagerWithOutput(exec, fs, privMgr, outputMgr, maxOutputSize, logger),
	}
	// Create dry-run manager eagerly to keep state like analyses across mode flips
	// and to simplify switching without re-wiring dependencies.
	dryrunManager, err := NewDryRunResourceManager(exec, privMgr, pathResolver, dryRunOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create dry-run resource manager: %w", err)
	}
	mgr.dryrun = dryrunManager
	return mgr, nil
}

// GetMode returns the current execution mode.
func (d *DefaultResourceManager) GetMode() ExecutionMode { return d.mode }

// activeManager returns the manager corresponding to the current execution mode.
func (d *DefaultResourceManager) activeManager() ResourceManager {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun
	}
	return d.normal
}

// ExecuteCommand delegates to the active manager.
func (d *DefaultResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error) {
	return d.activeManager().ExecuteCommand(ctx, cmd, group, env)
}

// CreateTempDir delegates to the active manager.
func (d *DefaultResourceManager) CreateTempDir(groupName string) (string, error) {
	return d.activeManager().CreateTempDir(groupName)
}

// CleanupTempDir delegates to the active manager.
func (d *DefaultResourceManager) CleanupTempDir(tempDirPath string) error {
	return d.activeManager().CleanupTempDir(tempDirPath)
}

// CleanupAllTempDirs delegates to the active manager.
func (d *DefaultResourceManager) CleanupAllTempDirs() error {
	return d.activeManager().CleanupAllTempDirs()
}

// WithPrivileges delegates to the active manager.
func (d *DefaultResourceManager) WithPrivileges(ctx context.Context, fn func() error) error {
	return d.activeManager().WithPrivileges(ctx, fn)
}

// SendNotification delegates to the active manager.
func (d *DefaultResourceManager) SendNotification(message string, details map[string]any) error {
	return d.activeManager().SendNotification(message, details)
}

// GetDryRunResults returns dry-run results if in dry-run mode; otherwise nil.
func (d *DefaultResourceManager) GetDryRunResults() *DryRunResult {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun.GetDryRunResults()
	}
	return nil
}

// RecordAnalysis records an analysis in dry-run mode; no-op in normal mode.
func (d *DefaultResourceManager) RecordAnalysis(analysis *ResourceAnalysis) {
	if d.mode == ExecutionModeDryRun {
		d.dryrun.RecordAnalysis(analysis)
	}
}
