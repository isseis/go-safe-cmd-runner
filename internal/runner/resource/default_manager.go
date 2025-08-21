package resource

import (
	"context"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// DefaultResourceManager provides a mode-aware facade that delegates to
// NormalResourceManager or DryRunResourceManagerImpl depending on ExecutionMode.
// It implements DryRunResourceManager so callers can always query dry-run results
// (returns nil in normal mode) and record analyses (no-op in normal mode).
type DefaultResourceManager struct {
	mode   ExecutionMode
	normal *NormalResourceManager
	dryrun *DryRunResourceManagerImpl
}

// NewDefaultResourceManager creates a new DefaultResourceManager.
// If mode is ExecutionModeDryRun, opts may be used to configure the dry-run behavior.
func NewDefaultResourceManager(exec executor.CommandExecutor, fs executor.FileSystem, privMgr runnertypes.PrivilegeManager, mode ExecutionMode, opts *DryRunOptions) *DefaultResourceManager {
	mgr := &DefaultResourceManager{
		mode:   mode,
		normal: NewNormalResourceManager(exec, fs, privMgr),
	}
	// Create dry-run manager eagerly to keep state like analyses across mode flips
	// and to simplify switching without re-wiring dependencies.
	mgr.dryrun = NewDryRunResourceManager(exec, fs, privMgr, opts)
	return mgr
}

// SetMode switches the execution mode. When switching to dry-run, the previously
// created dry-run manager will be used; opts can be provided to update options.
func (d *DefaultResourceManager) SetMode(mode ExecutionMode, opts *DryRunOptions) {
	d.mode = mode
	if mode == ExecutionModeDryRun && opts != nil && d.dryrun != nil {
		// Update options in place; keep accumulated analyses intact unless
		// the caller resets explicitly by creating a new manager.
		d.dryrun.mu.Lock()
		d.dryrun.dryRunOptions = opts
		d.dryrun.mu.Unlock()
	}
}

// GetMode returns the current execution mode.
func (d *DefaultResourceManager) GetMode() ExecutionMode { return d.mode }

// ExecuteCommand delegates to the active manager.
func (d *DefaultResourceManager) ExecuteCommand(ctx context.Context, cmd runnertypes.Command, group *runnertypes.CommandGroup, env map[string]string) (*ExecutionResult, error) {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun.ExecuteCommand(ctx, cmd, group, env)
	}
	return d.normal.ExecuteCommand(ctx, cmd, group, env)
}

// CreateTempDir delegates to the active manager.
func (d *DefaultResourceManager) CreateTempDir(groupName string) (string, error) {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun.CreateTempDir(groupName)
	}
	return d.normal.CreateTempDir(groupName)
}

// CleanupTempDir delegates to the active manager.
func (d *DefaultResourceManager) CleanupTempDir(tempDirPath string) error {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun.CleanupTempDir(tempDirPath)
	}
	return d.normal.CleanupTempDir(tempDirPath)
}

// CleanupAllTempDirs delegates to the active manager.
func (d *DefaultResourceManager) CleanupAllTempDirs() error {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun.CleanupAllTempDirs()
	}
	return d.normal.CleanupAllTempDirs()
}

// WithPrivileges delegates to the active manager.
func (d *DefaultResourceManager) WithPrivileges(ctx context.Context, fn func() error) error {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun.WithPrivileges(ctx, fn)
	}
	return d.normal.WithPrivileges(ctx, fn)
}

// IsPrivilegeEscalationRequired delegates to the active manager.
func (d *DefaultResourceManager) IsPrivilegeEscalationRequired(cmd runnertypes.Command) (bool, error) {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun.IsPrivilegeEscalationRequired(cmd)
	}
	return d.normal.IsPrivilegeEscalationRequired(cmd)
}

// SendNotification delegates to the active manager.
func (d *DefaultResourceManager) SendNotification(message string, details map[string]interface{}) error {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun.SendNotification(message, details)
	}
	return d.normal.SendNotification(message, details)
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
