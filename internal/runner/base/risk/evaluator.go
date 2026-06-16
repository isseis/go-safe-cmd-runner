// Package risk provides command risk evaluation functionality for the safe command runner.
// It analyzes commands and determines their security risk level based on various patterns and behaviors.
package risk

import (
	"path/filepath"
	"slices"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
)

// Evaluator interface defines methods for evaluating command risk levels.
// It produces a VerifiedCommandPlan so the evaluated identity and the executed
// identity are bound together (the executor runs only the plan, never the raw
// argv/env).
type Evaluator interface {
	EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error)
}

// StandardEvaluator implements risk evaluation using predefined patterns
type StandardEvaluator struct {
	networkAnalyzer *security.NetworkAnalyzer
}

// NewStandardEvaluator creates a new standard risk evaluator with a prebuilt network analyzer.
func NewStandardEvaluator(networkAnalyzer *security.NetworkAnalyzer) Evaluator {
	return &StandardEvaluator{
		networkAnalyzer: networkAnalyzer,
	}
}

// EvaluateRisk analyzes a command and returns a VerifiedCommandPlan whose
// Assessment carries the effective risk (the maximum across every applicable
// dimension) plus the reasoning. The evaluation order follows the architecture's
// dimension priority: deny gates (identity, privilege) short-circuit first, then
// the order-independent max of the remaining dimensions is taken.
//
// error is reserved for genuinely unexpected internal failures (an
// unclassifiable record-load I/O error). Policy denies -- uncertain analysis,
// symlink resolution failure, analysis disabled, unverified identity -- are
// returned as a non-error plan with Assessment.Blocking set, so the resource
// manager can audit them on a single deny path.
func (e *StandardEvaluator) EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error) {
	cmdPath := cmd.ExpandedCmd

	// The command path must be absolute by the time it reaches the evaluator
	// (callers resolve it). A non-absolute path means the identity cannot be
	// established and binary analysis would be silently skipped, so deny
	// fail-closed rather than evaluate an unanalyzable relative path.
	if !filepath.IsAbs(cmdPath) {
		return blockingPlan(risktypes.RiskAssessment{
			Blocking:       true,
			BlockingReason: risktypes.ReasonNonAbsolutePath,
			ErrorClass:     risktypes.ErrorClassPathResolution,
			ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonNonAbsolutePath},
		}), nil
	}

	// Resolve the symlink chain once up front. A resolution failure is fail-closed
	// (Blocking) so a dangerous real target is never missed by evaluating a
	// partially resolved chain.
	names, err := security.ResolveCommandNames(cmdPath)
	if err != nil {
		return blockingPlan(risktypes.RiskAssessment{
			Blocking:       true,
			BlockingReason: risktypes.ReasonSymlinkResolutionFailed,
			ErrorClass:     risktypes.ErrorClassSymlinkResolution,
			ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonSymlinkResolutionFailed},
		}), nil
	}

	// Rank 1: identity gate. Without a verified hash, or with binary analysis
	// disabled, the binary's identity cannot be confirmed; deny regardless of the
	// configured risk_level. This gate runs before every other dimension so no
	// path (coreutils, destructive, profile, arbitrary-code runner) can confirm a
	// Low/High-allowable risk for an unverified binary.
	if blocked, ok := e.identityGate(cmd); ok {
		return blockingPlan(blocked), nil
	}

	// Rank 2: indirect execution. Detect forms that run or load an artifact other
	// than the verified one (wrappers, inline shells, find/xargs child-process
	// helpers, dynamic loaders, remote-shell helpers). A privilege token there is
	// Critical and an unbindable form is a Blocking deny; both short-circuit. An
	// allowable form contributes a risk floor folded into the dimension maximum.
	indirect := security.AnalyzeIndirectExecution(cmdPath, cmd.ExpandedArgs)
	switch indirect.Kind {
	case security.IndirectCritical:
		plan := criticalPlan(cmd, risktypes.RiskAssessment{
			Level:          runnertypes.RiskLevelCritical,
			BlockingReason: risktypes.ReasonPrivilegeEscalation,
			ReasonCodes:    indirect.ReasonCodes,
		})
		plan.Artifacts = indirect.Artifacts
		return plan, nil
	case security.IndirectReject:
		a := risktypes.RiskAssessment{
			Blocking:       true,
			BlockingReason: indirectBlockingReason(indirect.ReasonCodes),
			ErrorClass:     indirect.ErrorClass,
			ReasonCodes:    indirect.ReasonCodes,
		}
		// The command passed the identity gate, so preserve the verified identity
		// even on deny — audits and artifact gating need it on reject paths.
		plan := risktypes.VerifiedCommandPlan{
			Identity:   identityFor(cmd),
			Assessment: a,
		}
		plan.Artifacts = indirect.Artifacts
		return plan, nil
	}

	// Rank 3: privilege escalation -> Critical (always denied). Detected through
	// the resolved profile so sudo/su/doas cannot be missed via a symlink alias.
	// names was resolved once (strict) at the top of EvaluateRisk.
	profile, profileFound := security.ResolveProfile(names)
	if profileFound && profile.IsPrivilege() {
		return criticalPlan(cmd, risktypes.RiskAssessment{
			Level:          runnertypes.RiskLevelCritical,
			BlockingReason: risktypes.ReasonPrivilegeEscalation,
			ReasonCodes:    []risktypes.ReasonCode{risktypes.ReasonPrivilegeEscalation},
			Reasons:        profile.GetRiskReasons(),
		}), nil
	}

	// Ranks 4-8: order-independent maximum of the applicable dimensions.
	assessment, err := e.evaluateDimensions(cmd, names, profile, profileFound)
	if err != nil {
		return risktypes.VerifiedCommandPlan{}, err
	}
	// Fold the rank-2 indirect-execution floor (an allowable wrapper/inline/package
	// form) into the maximum so a wrapped dangerous command is not under-classified.
	// Reason codes and human-readable reasons are de-duplicated: a wrapped runner
	// like "bash -c" yields ReasonArbitraryCodeExecution from both the floor and the
	// rank-7 runner dimension, and appending both only makes the audit output
	// noisier. The level is folded once regardless of duplicate codes.
	if indirect.Kind == security.IndirectFloor && indirect.Level > runnertypes.RiskLevelUnknown {
		assessment.Level = max(assessment.Level, indirect.Level)
		for _, code := range indirect.ReasonCodes {
			if !slices.Contains(assessment.ReasonCodes, code) {
				assessment.ReasonCodes = append(assessment.ReasonCodes, code)
			}
		}
		for _, reason := range indirect.Reasons {
			if !slices.Contains(assessment.Reasons, reason) {
				assessment.Reasons = append(assessment.Reasons, reason)
			}
		}
	}
	if assessment.Blocking {
		// The identity gate (rank 1) already passed, so the binary's identity is
		// verified even though a later dimension denies it. Preserve that identity
		// on the deny plan — consistent with the IndirectReject path — so audits and
		// artifact gating have it (a nil Identity is reserved for denies that never
		// established one).
		plan := risktypes.VerifiedCommandPlan{
			Identity:   identityFor(cmd),
			Assessment: assessment,
		}
		plan.Artifacts = indirect.Artifacts
		return plan, nil
	}
	plan := allowedPlan(cmd, assessment)
	plan.Artifacts = indirect.Artifacts
	return plan, nil
}

// indirectBlockingReason returns the primary blocking reason for a rejected
// indirect-execution form, falling back to the generic rejection code when the
// resolver returned no codes.
func indirectBlockingReason(codes []risktypes.ReasonCode) risktypes.ReasonCode {
	if len(codes) > 0 {
		return codes[0]
	}
	return risktypes.ReasonIndirectExecutionRejected
}

// identityGate implements the Phase 1 identity gate: the binary must carry a
// verified content hash and binary analysis must be enabled. It returns the
// blocking assessment and true when the gate denies.
func (e *StandardEvaluator) identityGate(cmd *runnertypes.RuntimeCommand) (risktypes.RiskAssessment, bool) {
	if cmd.ExpandedCmdContentHash == "" {
		return blockingAssessment(risktypes.ReasonUncertainUnverifiedIdentity, ""), true
	}
	if !e.networkAnalyzer.AnalysisEnabled() {
		return blockingAssessment(risktypes.ReasonAnalysisDisabled, ""), true
	}
	return risktypes.RiskAssessment{}, false
}

// evaluateDimensions computes the effective risk as the maximum across the
// applicable dimensions (ranks 4-8). It returns a Blocking assessment when a
// dimension fails closed (coreutils file-info failure, uncertain binary
// analysis), or an error for a genuinely unexpected internal failure.
func (e *StandardEvaluator) evaluateDimensions(
	cmd *runnertypes.RuntimeCommand,
	names map[string]struct{},
	profile security.CommandRiskProfile,
	profileFound bool,
) (risktypes.RiskAssessment, error) {
	cmdPath := cmd.ExpandedCmd
	args := cmd.ExpandedArgs

	a := risktypes.RiskAssessment{Level: runnertypes.RiskLevelLow}

	// Rank 4: coreutils single-binary classification. When it applies it is
	// authoritative and suppresses the binary-analysis dimension (including its
	// uncertain/missing-record signal), but other dimensions still contribute.
	coreutilsRisk, coreutilsHandled, err := security.CoreutilsCommandRisk(cmdPath, args)
	if err != nil {
		return blockingAssessment(risktypes.ReasonCoreutilsClassification, risktypes.ErrorClassCoreutilsFileInfo), nil
	}
	if coreutilsHandled {
		addDimension(&a, coreutilsRisk, risktypes.ReasonCoreutilsClassification)
	}

	// Destructive operations and system modification (order-independent max).
	// names was resolved once (strict) by the caller and is shared by every
	// name-based dimension below.
	if security.IsDestructiveFileOperation(names, args) {
		addDimension(&a, runnertypes.RiskLevelHigh, risktypes.ReasonDestructiveFileOperation)
	}
	if sysmod := security.SystemModificationRisk(names, args); sysmod > runnertypes.RiskLevelUnknown {
		addDimension(&a, sysmod, risktypes.ReasonSystemModification)
	}

	// Rank 5: profile factors (privilege handled at rank 3; system modification
	// handled above so the static SystemModRisk High is not imported unconditionally).
	if profileFound {
		applyProfileFactors(&a, profile, args)
	}

	// Rank 6: dangerous argument patterns (rm -rf, dd if=, chmod -R 777, ...).
	if level, _ := security.CheckDangerousArgPatterns(names, args); level > runnertypes.RiskLevelUnknown {
		addDimension(&a, level, risktypes.ReasonDangerousArgPattern)
	}

	// Rank 7: arbitrary-code-execution runners (shells/interpreters/build runners)
	// -> High regardless of arguments.
	if security.IsArbitraryCodeExecutionRunner(names) {
		addDimension(&a, runnertypes.RiskLevelHigh, risktypes.ReasonArbitraryCodeExecution)
	}

	// Network-style arguments (URL or SSH-style address) make any command a
	// network operation (Medium), independent of whether it has a network
	// profile, so an unprofiled helper invoked with a remote target is not left at
	// Low. Profiled commands already contribute their NetworkRisk above; this
	// dimension only raises, so the duplicate Medium is harmless.
	if !profileFound && security.HasNetworkArguments(args) {
		addDimension(&a, runnertypes.RiskLevelMedium, risktypes.ReasonNetworkArgument)
	}

	// Rank 8: binary-analysis classification (suppressed for coreutils).
	if !coreutilsHandled && filepath.IsAbs(cmdPath) {
		blocked, err := e.applyBinaryAnalysis(&a, cmd)
		if err != nil {
			return risktypes.RiskAssessment{}, err
		}
		if blocked != nil {
			return *blocked, nil
		}
	}

	return a, nil
}

// addDimension folds one dimension's level into the assessment (taking the max)
// and records its reason code.
func addDimension(a *risktypes.RiskAssessment, level runnertypes.RiskLevel, code risktypes.ReasonCode) {
	if level > a.Level {
		a.Level = level
	}
	a.ReasonCodes = append(a.ReasonCodes, code)
}

// applyProfileFactors folds the profile's non-privilege, non-system-modification
// risk factors into the assessment and records the human-readable reasons.
func applyProfileFactors(a *risktypes.RiskAssessment, profile security.CommandRiskProfile, args []string) {
	if level, codes := security.ProfileFactorRisk(profile, args); level > runnertypes.RiskLevelUnknown {
		for _, code := range codes {
			addDimension(a, level, code)
		}
	}
	a.Reasons = profile.GetRiskReasons()
	a.NetworkType = networkTypeString(profile.NetworkType)
}

// applyBinaryAnalysis folds the binary-analysis dimension into the assessment.
// It returns a non-nil blocking assessment when the binary is uncertain
// (fail-closed), or an error for a genuinely unexpected record-load failure. The
// identity gate already guaranteed a verified hash and that analysis is enabled.
func (e *StandardEvaluator) applyBinaryAnalysis(a *risktypes.RiskAssessment, cmd *runnertypes.RuntimeCommand) (*risktypes.RiskAssessment, error) {
	result, err := e.networkAnalyzer.Classify(cmd.ExpandedCmd, cmd.ExpandedCmdContentHash)
	if err != nil {
		return nil, err
	}
	switch result.Class {
	case risktypes.BinaryAnalysisUncertain:
		blocked := blockingAssessment("", "")
		blocked.ReasonCodes = result.ReasonCodes
		if len(result.ReasonCodes) > 0 {
			blocked.BlockingReason = result.ReasonCodes[0]
		}
		return &blocked, nil
	case risktypes.BinaryAnalysisHighRisk:
		a.Level = max(a.Level, runnertypes.RiskLevelHigh)
		a.ReasonCodes = append(a.ReasonCodes, result.ReasonCodes...)
	case risktypes.BinaryAnalysisNetwork:
		a.Level = max(a.Level, runnertypes.RiskLevelMedium)
		a.ReasonCodes = append(a.ReasonCodes, result.ReasonCodes...)
	case risktypes.BinaryAnalysisClean:
		// No contribution.
	}
	return nil, nil
}

// networkTypeString renders a NetworkOperationType for the audit NetworkType field.
func networkTypeString(t security.NetworkOperationType) string {
	switch t {
	case security.NetworkTypeAlways:
		return "always"
	case security.NetworkTypeConditional:
		return "conditional"
	default:
		return "none"
	}
}

// blockingAssessment builds a Blocking (deny) assessment with the given reason
// and optional error class.
func blockingAssessment(reason risktypes.ReasonCode, errClass risktypes.ErrorClass) risktypes.RiskAssessment {
	a := risktypes.RiskAssessment{
		Blocking:       true,
		BlockingReason: reason,
		ErrorClass:     errClass,
	}
	if reason != "" {
		a.ReasonCodes = []risktypes.ReasonCode{reason}
	}
	return a
}

// blockingPlan wraps a blocking assessment in a plan with no verified identity.
func blockingPlan(a risktypes.RiskAssessment) risktypes.VerifiedCommandPlan {
	return risktypes.VerifiedCommandPlan{Assessment: a}
}

// criticalPlan wraps a Critical (privilege escalation) assessment. The identity
// is verified (it passed the gate) but the command is denied by its level.
func criticalPlan(cmd *runnertypes.RuntimeCommand, a risktypes.RiskAssessment) risktypes.VerifiedCommandPlan {
	return risktypes.VerifiedCommandPlan{
		Identity:   identityFor(cmd),
		Assessment: a,
	}
}

// allowedPlan builds an executable plan carrying the verified identity. fd
// binding is wired in Phase 2; for now the identity records the resolved path
// and content hash.
func allowedPlan(cmd *runnertypes.RuntimeCommand, a risktypes.RiskAssessment) risktypes.VerifiedCommandPlan {
	return risktypes.VerifiedCommandPlan{
		ResolvedPath: cmd.ExpandedCmd,
		ResolvedArgv: append([]string{cmd.ExpandedCmd}, cmd.ExpandedArgs...),
		Identity:     identityFor(cmd),
		Assessment:   a,
	}
}

// identityFor builds the verified identity for a command that passed the gate.
func identityFor(cmd *runnertypes.RuntimeCommand) *risktypes.VerifiedIdentity {
	return &risktypes.VerifiedIdentity{
		ResolvedPath: cmd.ExpandedCmd,
		ContentHash:  cmd.ExpandedCmdContentHash,
	}
}
