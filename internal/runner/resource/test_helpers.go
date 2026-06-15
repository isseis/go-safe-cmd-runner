//go:build test || performance
// +build test performance

package resource

import (
	"log/slog"
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
)

// cleanRecordStore returns a clean analysis record for any path, so the risk
// evaluator's identity gate sees analysis as enabled. Bare command names used in
// these tests are not absolute, so binary analysis is skipped and the record is
// never actually consulted; the store only needs to be non-nil.
type cleanRecordStore struct{}

func (cleanRecordStore) LoadRecord(_ string) (*fileanalysis.Record, error) {
	return &fileanalysis.Record{}, nil
}

func defaultTestEvaluator() risk.Evaluator {
	deps := security.AnalysisDeps{RecordStore: cleanRecordStore{}}
	return risk.NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, deps))
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
	return NewNormalResourceManagerWithEvaluator(exec, fs, privMgr, outputMgr, maxOutputSize, logger, defaultTestEvaluator())
}

// NewNormalResourceManagerWithEvaluator creates a NormalResourceManager with an
// explicit risk evaluator, so tests that run real commands with file validation
// disabled can supply a permissive evaluator (the production identity gate would
// otherwise deny them).
func NewNormalResourceManagerWithEvaluator(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	outputMgr output.CaptureManager,
	maxOutputSize int64,
	logger *slog.Logger,
	evaluator risk.Evaluator,
) *NormalResourceManager {
	return newNormalManager(Config{
		Executor:         exec,
		FileSystem:       fs,
		PrivilegeManager: privMgr,
		MaxOutputSize:    maxOutputSize,
		Logger:           logger,
		RiskEvaluator:    evaluator,
	}, outputMgr)
}
