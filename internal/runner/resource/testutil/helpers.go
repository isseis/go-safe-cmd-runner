//go:build test || performance

// Package resourcetestutil provides test helper functions for resource management testing.
package resourcetestutil

import (
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risk"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
)

// allowAllEvaluator is a permissive risk evaluator for resource-flow tests
// (execution, output capture, logging, redaction) that are not exercising risk
// classification. It always returns an allowed Low plan with a verified identity,
// so these tests are not coupled to the evaluator's gating; risk classification is
// covered by the risk package's own tests.
type allowAllEvaluator struct{}

func (allowAllEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
	return risktypes.VerifiedCommandPlan{
		ResolvedPath: cmd.ExpandedCmd,
		// A VerifiedIdentity must carry a real, non-empty content hash; when the
		// command has no hash, represent absence with a nil Identity per the
		// VerifiedIdentity contract.
		Identity:   allowedTestIdentity(cmd),
		Assessment: risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow},
	}, nil
}

// allowedTestIdentity builds a VerifiedIdentity for a permissively-allowed test
// command, or nil when the command carries no content hash (the contract forbids
// an Identity with an empty ContentHash).
func allowedTestIdentity(cmd *runnertypes.RuntimeCommand) *risktypes.VerifiedIdentity {
	if cmd.ExpandedCmdContentHash == "" {
		return nil
	}
	return &risktypes.VerifiedIdentity{
		ResolvedPath: cmd.ExpandedCmd,
		ContentHash:  cmd.ExpandedCmdContentHash,
	}
}

// NewAllowAllEvaluator returns a permissive risk evaluator for end-to-end and
// performance tests that run real commands with file validation disabled (where
// the production identity gate would otherwise deny). Risk classification itself
// is covered by the risk package's unit tests.
func NewAllowAllEvaluator() risk.Evaluator {
	return allowAllEvaluator{}
}

func defaultTestEvaluator() risk.Evaluator {
	return allowAllEvaluator{}
}

// NewNormalResourceManager creates a new NormalResourceManager for normal execution mode
func NewNormalResourceManager(
	exec executor.CommandExecutor,
	fs executor.FileSystem,
	privMgr runnertypes.PrivilegeManager,
	logger *slog.Logger,
) *resource.NormalResourceManager {
	// Use a permissive evaluator: callers run real commands with file validation
	// disabled, where the production identity gate would otherwise deny them.
	return resource.NewNormalResourceManagerWithEvaluator(exec, fs, privMgr, nil, 0, logger, allowAllEvaluator{})
}

// NewDefaultResourceManager creates a DefaultResourceManager for tests.
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
) (*resource.DefaultResourceManager, error) {
	return resource.NewDefaultResourceManager(resource.Config{
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
