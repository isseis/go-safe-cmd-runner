//go:build test || performance
// +build test performance

package resource

import (
	"log/slog"
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
)

// cleanRecordStore returns a clean analysis record for any path so the risk
// evaluator's identity gate sees analysis as enabled. The record reports the same
// content hash that CreateRuntimeCommand attaches by default, so an absolute-path
// command classifies as Clean (no dangerous signals) instead of triggering a
// hash mismatch. The record carries no signals, so binary analysis contributes
// nothing to the effective risk.
type cleanRecordStore struct{}

func (cleanRecordStore) LoadRecord(_ string) (*fileanalysis.Record, error) {
	return &fileanalysis.Record{ContentHash: executortestutil.DefaultTestContentHash}, nil
}

func defaultTestEvaluator() risk.Evaluator {
	deps := security.AnalysisDeps{RecordStore: cleanRecordStore{}}
	return risk.NewStandardEvaluator(security.NewNetworkAnalyzer(runtime.GOOS, deps))
}

// permissiveTestEvaluator always allows commands at Low risk. It is used by
// manager-mechanics tests (error handling, output capture, concurrency) that
// build commands which are not absolute paths and are not exercising risk
// classification; risk gating is covered by the risk package and the dedicated
// risk-control tests that use the standard evaluator.
type permissiveTestEvaluator struct{}

func (permissiveTestEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
	// A VerifiedIdentity must carry a real, non-empty content hash; when the
	// command has no hash, represent absence with a nil Identity per the contract.
	var identity *risktypes.VerifiedIdentity
	if cmd.ExpandedCmdContentHash != "" {
		identity = &risktypes.VerifiedIdentity{ResolvedPath: cmd.ExpandedCmd, ContentHash: cmd.ExpandedCmdContentHash}
	}
	return risktypes.VerifiedCommandPlan{
		ResolvedPath: cmd.ExpandedCmd,
		Identity:     identity,
		Assessment:   risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
	}, nil
}

// keyedRiskEvaluator returns a preset assessment per command name (falling back to
// Low for unknown names), so dry-run preview tests can drive allow/deny/blocking
// outcomes deterministically without on-disk binaries or hash records.
type keyedRiskEvaluator map[string]risktypes.RiskAssessment

func (m keyedRiskEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
	a, ok := m[cmd.Name()]
	if !ok {
		a = risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow}
	}
	return risktypes.VerifiedCommandPlan{
		ResolvedPath: cmd.ExpandedCmd,
		Identity:     &risktypes.VerifiedIdentity{ResolvedPath: cmd.ExpandedCmd, ContentHash: cmd.ExpandedCmdContentHash},
		Assessment:   a,
	}, nil
}

// passthroughPathResolver returns the command path unchanged so dry-run preview
// tests do not depend on real binaries being present on disk.
type passthroughPathResolver struct{}

func (passthroughPathResolver) ResolvePath(cmd string) (string, error) { return cmd, nil }

// NewNormalResourceManager creates a NormalResourceManager for manager-mechanics
// tests. It uses a permissive evaluator (see permissiveTestEvaluator); tests that
// exercise risk classification use NewNormalResourceManagerWithOutput, which wires
// the standard evaluator.
func NewNormalResourceManager(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	logger *slog.Logger,
) *NormalResourceManager {
	return NewNormalResourceManagerWithEvaluator(exec, fs, privMgr, nil, 0, logger, permissiveTestEvaluator{})
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
