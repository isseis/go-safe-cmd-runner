package resource

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/dynamicanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
)

// Config holds all dependencies and settings for DefaultResourceManager.
// Any nil store disables the corresponding analysis.
type Config struct {
	Executor           executor.CommandExecutor
	FileSystem         executor.FileSystem
	PrivilegeManager   runnertypes.PrivilegeManager
	PathResolver       PathResolver
	Logger             *slog.Logger
	Mode               ExecutionMode
	DryRunOpts         *DryRunOptions
	OutputManager      output.CaptureManager
	MaxOutputSize      int64
	NetworkSymbolStore fileanalysis.NetworkSymbolStore
	SyscallStore       fileanalysis.SyscallAnalysisStore
	DynLibDepsStore    fileanalysis.DynLibDepsStore
	LibAnalysisStore   dynamicanalysis.Store
}

// DefaultResourceManager provides a mode-aware facade that delegates to
// NormalResourceManager or DryRunResourceManager depending on ExecutionMode.
// It implements Manager so callers can always query dry-run results
// (returns nil in normal mode) and record analyses (no-op in normal mode).
type DefaultResourceManager struct {
	mode   ExecutionMode
	normal *NormalResourceManager
	dryrun *DryRunResourceManager
}

// NewDefaultResourceManager creates a new DefaultResourceManager from cfg.
func NewDefaultResourceManager(cfg Config) (*DefaultResourceManager, error) {
	outputMgr := cfg.OutputManager
	if outputMgr == nil {
		securityValidator, err := security.NewValidator(nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create security validator: %w", err)
		}
		outputMgr = output.NewDefaultOutputCaptureManager(securityValidator)
	}

	mgr := &DefaultResourceManager{
		mode: cfg.Mode,
		normal: NewNormalResourceManagerWithStores(
			cfg.Executor, cfg.FileSystem, cfg.PrivilegeManager,
			outputMgr, cfg.MaxOutputSize, cfg.Logger,
			cfg.NetworkSymbolStore, cfg.SyscallStore, cfg.DynLibDepsStore, cfg.LibAnalysisStore,
		),
	}
	// Create dry-run manager eagerly to keep state like analyses across mode flips
	// and to simplify switching without re-wiring dependencies.
	dryrunManager, err := NewDryRunResourceManager(cfg.Executor, cfg.PrivilegeManager, cfg.PathResolver, cfg.DryRunOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create dry-run resource manager: %w", err)
	}
	mgr.dryrun = dryrunManager
	return mgr, nil
}

// GetMode returns the current execution mode.
func (d *DefaultResourceManager) GetMode() ExecutionMode { return d.mode }

// activeManager returns the manager corresponding to the current execution mode.
func (d *DefaultResourceManager) activeManager() Manager {
	if d.mode == ExecutionModeDryRun {
		return d.dryrun
	}
	return d.normal
}

// ExecuteCommand delegates to the active manager.
func (d *DefaultResourceManager) ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, groupSpec *runnertypes.GroupSpec, env map[string]string) (CommandToken, *ExecutionResult, error) {
	return d.activeManager().ExecuteCommand(ctx, cmd, groupSpec, env)
}

// ValidateOutputPath delegates to the active manager.
func (d *DefaultResourceManager) ValidateOutputPath(outputPath, workDir string) error {
	return d.activeManager().ValidateOutputPath(outputPath, workDir)
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
func (d *DefaultResourceManager) RecordAnalysis(analysis *Analysis) {
	if d.mode == ExecutionModeDryRun {
		d.dryrun.RecordAnalysis(analysis)
	}
}

// RecordGroupAnalysis records group analysis in dry-run mode; no-op in normal mode.
func (d *DefaultResourceManager) RecordGroupAnalysis(groupName string, debugInfo *DebugInfo) error {
	return d.activeManager().RecordGroupAnalysis(groupName, debugInfo)
}

// UpdateCommandDebugInfo updates a command's debug info using its token in dry-run mode; no-op in normal mode.
func (d *DefaultResourceManager) UpdateCommandDebugInfo(token CommandToken, debugInfo *DebugInfo) error {
	return d.activeManager().UpdateCommandDebugInfo(token, debugInfo)
}
