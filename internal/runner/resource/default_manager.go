package resource

import (
	"context"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
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
func NewDefaultResourceManager(exec executor.CommandExecutor, fs executor.FileSystem, privMgr runnertypes.PrivilegeManager, logger *slog.Logger, mode ExecutionMode, opts *DryRunOptions) *DefaultResourceManager {
	mgr := &DefaultResourceManager{
		mode:   mode,
		normal: NewNormalResourceManager(exec, fs, privMgr, logger),
	}
	// Create dry-run manager eagerly to keep state like analyses across mode flips
	// and to simplify switching without re-wiring dependencies.
	mgr.dryrun = NewDryRunResourceManager(exec, privMgr, opts)
	return mgr
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
