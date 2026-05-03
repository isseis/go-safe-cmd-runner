//go:build test || performance

// Package testutil provides test helper functions for resource management testing.
package testutil

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// NewNormalResourceManager creates a new NormalResourceManager for normal execution mode
func NewNormalResourceManager(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	logger *slog.Logger,
) *resource.NormalResourceManager {
	// Delegate to NewNormalResourceManagerWithOutput with nil outputManager and 0 maxOutputSize
	return resource.NewNormalResourceManagerWithOutput(exec, fs, privMgr, nil, 0, logger, nil)
}

// NewDefaultResourceManager creates a DefaultResourceManager with nil analysis stores
// (except the optional symStore). Use only in tests.
func NewDefaultResourceManager(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	pathResolver resource.PathResolver,
	logger *slog.Logger,
	mode resource.ExecutionMode,
	dryRunOpts *resource.DryRunOptions,
	outputMgr output.CaptureManager,
	maxOutputSize int64,
	symStore fileanalysis.NetworkSymbolStore,
) (*resource.DefaultResourceManager, error) {
	return resource.NewDefaultResourceManager(resource.Config{
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
