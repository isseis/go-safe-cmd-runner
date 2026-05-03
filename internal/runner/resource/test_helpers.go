//go:build test || performance
// +build test performance

package resource

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// NewNormalResourceManager creates a new NormalResourceManager for normal execution mode
func NewNormalResourceManager(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	logger *slog.Logger,
) *NormalResourceManager {
	// Delegate to NewNormalResourceManagerWithOutput with nil outputManager and 0 maxOutputSize
	return NewNormalResourceManagerWithOutput(exec, fs, privMgr, nil, 0, logger, nil)
}

// NewDefaultResourceManagerForTest creates a DefaultResourceManager with nil analysis stores
// (except the optional symStore). Use only in tests.
func NewDefaultResourceManagerForTest(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	pathResolver PathResolver,
	logger *slog.Logger,
	mode ExecutionMode,
	dryRunOpts *DryRunOptions,
	outputMgr output.CaptureManager,
	maxOutputSize int64,
	symStore fileanalysis.NetworkSymbolStore,
) (*DefaultResourceManager, error) {
	return NewDefaultResourceManager(Config{
		Executor:           exec,
		FileSystem:         fs,
		PrivilegeManager:   privMgr,
		PathResolver:       pathResolver,
		Logger:             logger,
		Mode:               mode,
		DryRunOpts:         dryRunOpts,
		OutputManager:      outputMgr,
		MaxOutputSize:      maxOutputSize,
		NetworkSymbolStore: symStore,
	})
}

// NewNormalResourceManagerWithOutput creates a new NormalResourceManager with output capture support
func NewNormalResourceManagerWithOutput(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	outputMgr output.CaptureManager,
	maxOutputSize int64,
	logger *slog.Logger,
	store fileanalysis.NetworkSymbolStore,
) *NormalResourceManager {
	return newNormalManager(Config{
		Executor:           exec,
		FileSystem:         fs,
		PrivilegeManager:   privMgr,
		MaxOutputSize:      maxOutputSize,
		Logger:             logger,
		NetworkSymbolStore: store,
	}, outputMgr)
}
