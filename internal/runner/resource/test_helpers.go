//go:build test || performance
// +build test performance

package resource

import (
	"log/slog"
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
)

func defaultTestEvaluator() risk.Evaluator {
	return risk.NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, security.AnalysisDeps{}))
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

// NewDefaultResourceManagerForTest creates a DefaultResourceManager for tests.
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
) (*DefaultResourceManager, error) {
	return NewDefaultResourceManager(Config{
		Executor:         exec,
		FileSystem:       fs,
		PrivilegeManager: privMgr,
		PathResolver:     pathResolver,
		Logger:           logger,
		Mode:             mode,
		DryRunOpts:       dryRunOpts,
		OutputManager:    outputMgr,
		MaxOutputSize:    maxOutputSize,
		RiskEvaluator:    defaultTestEvaluator(),
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
) *NormalResourceManager {
	return newNormalManager(Config{
		Executor:         exec,
		FileSystem:       fs,
		PrivilegeManager: privMgr,
		MaxOutputSize:    maxOutputSize,
		Logger:           logger,
		RiskEvaluator:    defaultTestEvaluator(),
	}, outputMgr)
}
