//go:build test || performance
// +build test performance

package resource

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
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

// NewDefaultResourceManagerForTest creates a DefaultResourceManager with nil SyscallAnalysisStore.
// Use only in tests; production code must supply all stores explicitly via NewDefaultResourceManager.
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
	return NewDefaultResourceManager(exec, fs, privMgr, pathResolver, logger, mode, dryRunOpts, outputMgr, maxOutputSize, symStore, nil)
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
	return NewNormalResourceManagerWithStores(exec, fs, privMgr, outputMgr, maxOutputSize, logger, store, nil)
}
