// Package risk provides command risk evaluation functionality for the safe command runner.
// It analyzes commands and determines their security risk level based on various patterns and behaviors.
package risk

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/security"
)

// ErrIdentityHashMismatch means the content hash computed from the fd opened
// for fd-bound execution did not match the hash verified at group verification
// time (cmd.ExpandedCmdContentHash). allowedPlan maps this to
// risktypes.ReasonIdentityHashMismatch.
var ErrIdentityHashMismatch = errors.New("verified identity content hash mismatch")

// ErrIdentityNotRegular means the path opened for fd-bound execution was not a
// regular file (e.g. it was replaced with a FIFO or device node between group
// verification and open). allowedPlan maps this to
// risktypes.ReasonIdentityNotRegular.
var ErrIdentityNotRegular = errors.New("verified identity is not a regular file")

// Evaluator interface defines methods for evaluating command risk levels.
// It produces a VerifiedCommandPlan so the evaluated identity and the executed
// identity are bound together (the executor runs only the plan, never the raw
// argv/env).
type Evaluator interface {
	EvaluateRisk(cmd *runnertypes.RuntimeCommand) (risktypes.VerifiedCommandPlan, error)
}

// identityOpener opens the verified executable and returns its identity (the
// descriptor used for fd-bound execution plus the resolved path and content
// hash). It is a field so risk-classification tests can inject a descriptor-free
// opener and need not place real files on disk; production uses
// openVerifiedIdentity.
type identityOpener func(cmd *runnertypes.RuntimeCommand) (*risktypes.VerifiedIdentity, error)

// zoningParams holds the precomputed, command-independent inputs for the axis-2
// destination-zoning dispatch. It is an injectable field on StandardEvaluator
// (nil = axis 2 disabled): while nil the evaluator keeps the legacy destructive
// classification unchanged. It is a dedicated struct rather than a raw
// *security.Config so a command's per-command run-as identity (resolved at
// dispatch time) does not belong here; runAsIdent is the DEFAULT identity used
// when a command sets no run-as (the original execution identity, resolved once).
type zoningParams struct {
	systemCriticalPaths        []string
	trustedDirectories         []string
	outputCriticalPathPatterns []string
	dedicatedTempDir           string
	runAsIdent                 risktypes.RunAsIdent
}

// runAsResolver resolves a run-as user/group name pair to the identity used by the
// Trusted predicate, starting from a precomputed base identity (the default
// execution identity captured once at construction). It is an injectable field
// (default: os/user based) so tests can supply an identity differing from the live
// euid, proving the judgment never reads live identity. Passing the base in (rather
// than re-reading the process) keeps the group-only form (which keeps the base
// uid/groups) from depending on live process identity at evaluation time. An empty
// user and group means "no run-as": the caller uses the base identity directly
// instead of calling this.
type runAsResolver func(base risktypes.RunAsIdent, user, group string) (risktypes.RunAsIdent, error)

// maxZoningOperands bounds the operands resolved per command (cost ceiling); an
// invocation with more fails closed rather than walking the filesystem
// unboundedly. It is a small constant well above any real file-operation arity.
const maxZoningOperands = 64

// StandardEvaluator implements risk evaluation using predefined patterns
type StandardEvaluator struct {
	networkAnalyzer *security.NetworkAnalyzer
	openIdentity    identityOpener
	// zoning is the injectable axis-2 input (nil = disabled). Populated from the
	// security config by NewStandardEvaluator, or directly by tests.
	zoning *zoningParams
	// resolveRunAs resolves a command's run-as user/group to an identity (default:
	// os/user based; overridable in tests). Always set so a command with a run-as
	// can be resolved even when zoning was injected directly.
	resolveRunAs runAsResolver
}

// NewStandardEvaluator creates a new standard risk evaluator. securityConfig
// enables axis-2 destination zoning (its SystemCriticalPaths / TrustedDirectories
// / OutputCriticalPathPatterns drive the classification); pass nil to disable
// axis 2 (legacy behavior). When enabled, the default run-as identity (used for
// commands without a run_as_user) is the original execution identity, resolved
// once here -- the zoning judgment itself never reads live identity.
func NewStandardEvaluator(networkAnalyzer *security.NetworkAnalyzer, securityConfig *security.Config) Evaluator {
	e := &StandardEvaluator{
		networkAnalyzer: networkAnalyzer,
		openIdentity:    openVerifiedIdentity,
		resolveRunAs:    risktypes.ResolveRunAsIdent,
	}
	if securityConfig != nil {
		e.zoning = &zoningParams{
			systemCriticalPaths:        securityConfig.SystemCriticalPaths,
			trustedDirectories:         securityConfig.TrustedDirectories,
			outputCriticalPathPatterns: securityConfig.OutputCriticalPathPatterns,
			runAsIdent:                 risktypes.OriginalExecutionIdentity(),
		}
	}
	return e
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
	// AnalyzeIndirectExecution re-resolves cmdPath's symlink chain internally (it is
	// exported and must be self-contained for standalone callers); the resulting
	// extra top-level Lstat is intentional and policy stays consistent because both
	// resolutions go through the single strict ResolveCommandNames.
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
	// the resolved profile so a privilege command (e.g. sudo, su) cannot be missed
	// via a symlink alias.
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
	// The level is folded once; reason codes and human-readable reasons are appended
	// and then de-duplicated (a wrapped runner like "bash -c" yields
	// ReasonArbitraryCodeExecution from both the floor and the rank-7 runner
	// dimension, and listing it twice only makes the audit output noisier).
	if indirect.Kind == security.IndirectFloor && indirect.Level > runnertypes.RiskLevelUnknown {
		assessment.Level = max(assessment.Level, indirect.Level)
		assessment.ReasonCodes = common.DedupeStable(append(assessment.ReasonCodes, indirect.ReasonCodes...))
		assessment.Reasons = common.DedupeStable(append(assessment.Reasons, indirect.Reasons...))
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
	plan := e.allowedPlan(cmd, assessment)
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

	// Axis 2: destination-path trust zoning. For a recognized file-operation command
	// the destination zone is the authoritative classification and replaces the
	// legacy fixed-High destructive dimensions (1, 2-destructive, 3, 4). When axis 2
	// does not apply (not a file op) or recognizes the command only partially, the
	// legacy dimensions stand so a form we could not fully parse is never downgraded
	// (fail-open avoidance). Disabled (legacy behavior) when zoning is not injected.
	var zone security.LocationResult
	suppressLegacy := false
	if e.zoning != nil {
		zone = security.ClassifyDestinationZone(e.zoningInput(cmd), names, cmdPath, args)
		suppressLegacy = zone.Applies && zone.Recognized
	}

	// Rank 4: coreutils single-binary classification. When it applies it is
	// authoritative and suppresses the binary-analysis dimension (including its
	// uncertain/missing-record signal), but other dimensions still contribute.
	coreutilsRisk, coreutilsHandled, err := security.CoreutilsCommandRisk(cmdPath, args)
	if err != nil {
		return blockingAssessment(risktypes.ReasonCoreutilsClassification, risktypes.ErrorClassCoreutilsFileInfo), nil
	}
	applyCoreutilsRisk(&a, cmdPath, coreutilsRisk, coreutilsHandled, suppressLegacy)

	// (1) Destructive file operation. Suppressed on full axis-2 recognition (the
	// zone/floors re-establish High where warranted). names was resolved once
	// (strict) by the caller and is shared by every name-based dimension below.
	if !suppressLegacy && security.IsDestructiveFileOperation(names, args) {
		addDimension(&a, runnertypes.RiskLevelHigh, risktypes.ReasonDestructiveFileOperation)
	}
	if sysmod := security.SystemModificationRisk(names); sysmod > runnertypes.RiskLevelUnknown {
		addDimension(&a, sysmod, risktypes.ReasonSystemModification)
	}

	// Rank 5: profile factors (privilege handled at rank 3; system modification
	// handled above so the static SystemModRisk High is not imported unconditionally).
	// (3) On full axis-2 recognition the destruction factor is suppressed at
	// component granularity; the other factors (network/data-exfil) and
	// NetworkType/Reasons still apply.
	if profileFound {
		applyProfileFactors(&a, profile, args, suppressLegacy)
	}

	// Rank 6: dangerous argument patterns (rm -rf, dd if=, chmod -R 777, ...). (4)
	// Suppressed at dispatch granularity on full axis-2 recognition: the function
	// folds multiple matches into one level, so it cannot be filtered component-wise;
	// the matched destructive entries are re-established by axis 2's zone and
	// operation-specific floors.
	if !suppressLegacy {
		if level, _ := security.CheckDangerousArgPatterns(names, args); level > runnertypes.RiskLevelUnknown {
			addDimension(&a, level, risktypes.ReasonDangerousArgPattern)
		}
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
			// Carry the per-operand audit records onto the deny so a recognized file
			// operation denied for an uncertain binary still records its zoning (the
			// level fold below is skipped on this fail-closed deny path).
			blocked.OperandZones = zone.Operands
			return *blocked, nil
		}
	}

	foldZoning(&a, zone)

	return a, nil
}

// applyCoreutilsRisk folds the coreutils single-binary classification into the
// assessment. On full axis-2 recognition the legacy destructive/unknown High is
// dropped (axis 2's zone level replaces it) but the setuid/setgid-binary signal is
// preserved from the existing stat-based signal (a coreutils hardlink is never expected
// to be setuid; a set bit is a compromise indicator independent of the
// destination). coreutilsHandled stays the caller's gate for binary-analysis
// suppression; this only governs the risk contribution.
func applyCoreutilsRisk(a *risktypes.RiskAssessment, cmdPath string, coreutilsRisk runnertypes.RiskLevel, coreutilsHandled, suppressLegacy bool) {
	if !coreutilsHandled {
		return
	}
	if !suppressLegacy {
		addDimension(a, coreutilsRisk, risktypes.ReasonCoreutilsClassification)
		return
	}
	setuid, err := security.CommandHasSetuidOrSetgidBit(cmdPath)
	if err != nil || setuid {
		// A stat failure here is anomalous (CoreutilsCommandRisk stat'd the same
		// binary without error moments ago) and must not silently drop the
		// setuid-binary signal, which would be a High->Low regression. Fail closed
		// to High on either a set bit or an unexpected error.
		addDimension(a, runnertypes.RiskLevelHigh, risktypes.ReasonPermissionGrant)
	}
}

// foldZoning folds the axis-2 result into the maximum and carries its per-operand
// audit records. On full recognition this is the replacement for the suppressed
// destructive dimensions; when it applies but is only partially recognized it
// contributes the fail-closed unresolved floor (the legacy dimensions were kept);
// when it does not apply (including zoning disabled, the zero value) it contributes
// nothing.
func foldZoning(a *risktypes.RiskAssessment, zone security.LocationResult) {
	if !zone.Applies {
		return
	}
	a.Level = max(a.Level, zone.Level)
	a.ReasonCodes = common.DedupeStable(append(a.ReasonCodes, zone.ReasonCodes...))
	a.OperandZones = zone.Operands
}

// zoningInput assembles the per-command axis-2 input from the injected,
// command-independent zoning parameters plus this command's working directory and
// run-as identity. A command's run_as_user/group is resolved here (the embedding
// layer); the zoning judgment receives only the precomputed identity. If the
// run-as name cannot be resolved, IdentityUnresolved is set so the judgment fails
// closed (every operand ZoneUnresolved) rather than trusting an unknown identity.
func (e *StandardEvaluator) zoningInput(cmd *runnertypes.RuntimeCommand) security.ZoningInput {
	z := e.zoning
	ident := z.runAsIdent // default: the original execution identity (run-as unset)
	identUnresolved := false
	// cmd.RunAsUser/RunAsGroup require a non-nil Spec (always set in production via
	// NewRuntimeCommand). Guard so a Spec-less command falls back to the default
	// identity rather than panicking.
	if cmd.Spec != nil {
		if u, g := cmd.RunAsUser(), cmd.RunAsGroup(); u != "" || g != "" {
			resolved, err := e.resolveRunAs(ident, u, g)
			if err != nil {
				identUnresolved = true
			} else {
				ident = resolved
			}
		}
	}
	return security.ZoningInput{
		EffectiveWorkDir:           cmd.EffectiveWorkDir,
		DedicatedTempDir:           z.dedicatedTempDir,
		SystemCriticalPaths:        z.systemCriticalPaths,
		TrustedDirectories:         z.trustedDirectories,
		RunAsIdent:                 ident,
		IdentityUnresolved:         identUnresolved,
		OutputCriticalPathPatterns: z.outputCriticalPathPatterns,
		MaxOperands:                maxZoningOperands,
		MaxSymlinkHops:             security.MaxSymlinkDepth,
	}
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
func applyProfileFactors(a *risktypes.RiskAssessment, profile security.CommandRiskProfile, args []string, suppressDestruction bool) {
	if level, codes := security.ProfileFactorRisk(profile, args, suppressDestruction); level > runnertypes.RiskLevelUnknown {
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
	return applyClassResult(a, result), nil
}

// applyClassResult maps a BinaryAnalysisResult to a RiskAssessment update. An
// unknown class is treated as fail-closed (Blocking), mirroring the Uncertain
// case. Known classes: Uncertain → Blocking, HighRisk/Network → elevate level,
// Clean → no contribution.
func applyClassResult(a *risktypes.RiskAssessment, result risktypes.BinaryAnalysisResult) *risktypes.RiskAssessment {
	switch result.Class {
	case risktypes.BinaryAnalysisUncertain:
		blocked := blockingAssessment("", "")
		blocked.ReasonCodes = result.ReasonCodes
		if len(result.ReasonCodes) > 0 {
			blocked.BlockingReason = result.ReasonCodes[0]
		}
		return &blocked
	case risktypes.BinaryAnalysisHighRisk:
		a.Level = max(a.Level, runnertypes.RiskLevelHigh)
		a.ReasonCodes = append(a.ReasonCodes, result.ReasonCodes...)
	case risktypes.BinaryAnalysisNetwork:
		a.Level = max(a.Level, runnertypes.RiskLevelMedium)
		a.ReasonCodes = append(a.ReasonCodes, result.ReasonCodes...)
	case risktypes.BinaryAnalysisClean:
		// No contribution.
	default:
		blocked := blockingAssessment("", "")
		blocked.ReasonCodes = result.ReasonCodes
		if len(result.ReasonCodes) > 0 {
			blocked.BlockingReason = result.ReasonCodes[0]
		}
		return &blocked
	}
	return nil
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

// allowedPlan builds an executable plan carrying the verified identity. It opens
// the verified executable for fd-bound execution once here, so the executor binds
// to this exact inode without re-resolving the path. If the open fails
// the binary cannot be bound, so deny fail-closed rather than fall back to an
// unbound path exec. The opened descriptor is owned by the returned plan and is
// released via VerifiedCommandPlan.Close.
func (e *StandardEvaluator) allowedPlan(cmd *runnertypes.RuntimeCommand, a risktypes.RiskAssessment) risktypes.VerifiedCommandPlan {
	identity, err := e.openIdentity(cmd)
	if err != nil {
		switch {
		case errors.Is(err, ErrIdentityHashMismatch):
			return blockingPlan(blockingAssessment(risktypes.ReasonIdentityHashMismatch, risktypes.ErrorClassPathResolution))
		case errors.Is(err, ErrIdentityNotRegular):
			return blockingPlan(blockingAssessment(risktypes.ReasonIdentityNotRegular, risktypes.ErrorClassPathResolution))
		default:
			return blockingPlan(blockingAssessment(risktypes.ReasonIdentityUnbound, risktypes.ErrorClassPathResolution))
		}
	}
	return risktypes.VerifiedCommandPlan{
		ResolvedPath: cmd.ExpandedCmd,
		ResolvedArgv: append([]string{cmd.ExpandedCmd}, cmd.ExpandedArgs...),
		Identity:     identity,
		Assessment:   a,
	}
}

// identityFor builds the verified identity for a denied command that passed the
// identity gate (privilege escalation, indirect-execution reject, or a
// later-dimension deny). Deny plans are not executed, so no descriptor is opened.
func identityFor(cmd *runnertypes.RuntimeCommand) *risktypes.VerifiedIdentity {
	return &risktypes.VerifiedIdentity{
		ResolvedPath: cmd.ExpandedCmd,
		ContentHash:  cmd.ExpandedCmdContentHash,
	}
}

// openVerifiedIdentity opens the resolved executable read-only for fd-bound
// execution and returns its identity. The descriptor is opened
// O_RDONLY|O_CLOEXEC|O_NONBLOCK once here so the executor can bind execution to
// this exact inode without re-resolving the path; ownership transfers to the
// returned VerifiedIdentity (closed via VerifiedCommandPlan.Close).
//
// Invariant: the fd returned is (a) confirmed to be a regular file (fstat) and
// (b) its content hash, computed from this same fd, matches
// cmd.ExpandedCmdContentHash (the hash verified at group verification time).
// Either check failing closes the fd and returns a distinguishable sentinel
// error rather than an unbound/mismatched identity (fail-closed). This closes
// the window between "group verification hashed the file" and "this open
// re-reads it" by re-checking the hash against the exact bytes this fd sees.
func openVerifiedIdentity(cmd *runnertypes.RuntimeCommand) (*risktypes.VerifiedIdentity, error) {
	// O_NONBLOCK is a safety net against an unbounded open-time block if the
	// path was replaced with a FIFO; it is a no-op once the file is confirmed
	// regular below (the fstat check is the real guarantee, not O_NONBLOCK).
	fd, err := syscall.Open(cmd.ExpandedCmd, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}

	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("fstat verified identity: %w", err)
	}
	if stat.Mode&syscall.S_IFMT != syscall.S_IFREG {
		_ = syscall.Close(fd)
		return nil, ErrIdentityNotRegular
	}

	// Regular file confirmed: clear O_NONBLOCK so the fd behaves normally for
	// the hash read below and for the executor that shares it afterward.
	if err := syscall.SetNonblock(fd, false); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("clear O_NONBLOCK verified identity: %w", err)
	}

	if err := verifyIdentityContentHash(fd, stat.Size, cmd.ExpandedCmdContentHash); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}

	return &risktypes.VerifiedIdentity{
		FD:           risktypes.NewVerifiedFD(fd),
		ResolvedPath: cmd.ExpandedCmd,
		ContentHash:  cmd.ExpandedCmdContentHash,
	}, nil
}

// verifyIdentityContentHash computes the content hash of the first size bytes
// readable from fd (via pread, so the fd's offset is left untouched for the
// executor that shares it afterward) and compares it against expectedHash
// ("algo:hex", e.g. "sha256:..."). It returns ErrIdentityHashMismatch both on an
// actual mismatch and on an unsupported algorithm prefix, since either way the
// content cannot be confirmed to match what group verification hashed
// (fail-closed).
//
// After hashing, it re-fstats fd and rejects if the size changed from size
// (the size captured before hashing). Execution later shares this same fd
// unbounded by size, so if the file grew between the initial fstat and this
// hash read, bytes appended after the hashed region would never have been
// hashed yet could still be executed; shrinking likewise means the hash was
// computed over bytes no longer backing the inode's current content. Either
// case means the verified hash no longer describes what the fd will yield,
// so this fails closed with ErrIdentityHashMismatch rather than trusting a
// hash of a size window that no longer matches the file.
func verifyIdentityContentHash(fd int, size int64, expectedHash string) error {
	var hasher filevalidator.SHA256
	algo, _, ok := strings.Cut(expectedHash, ":")
	if !ok || algo != hasher.Name() {
		return ErrIdentityHashMismatch
	}

	actualSum, err := hasher.Sum(io.NewSectionReader(fdReaderAt{fd: fd}, 0, size))
	if err != nil {
		return fmt.Errorf("compute verified identity hash: %w", err)
	}
	if fmt.Sprintf("%s:%s", algo, actualSum) != expectedHash {
		return ErrIdentityHashMismatch
	}

	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		return fmt.Errorf("re-fstat verified identity: %w", err)
	}
	if stat.Size != size {
		return ErrIdentityHashMismatch
	}
	return nil
}

// fdReaderAt reads from a raw file descriptor via pread, leaving the fd's
// offset untouched. It exists so the content hash can be computed from the
// exact fd used for fd-bound execution without wrapping it in an *os.File
// (which would attach a GC finalizer that could close the fd out from under
// its owning VerifiedFD).
type fdReaderAt struct {
	fd int
}

func (r fdReaderAt) ReadAt(p []byte, off int64) (int, error) {
	var total int
	for len(p) > 0 {
		n, err := syscall.Pread(r.fd, p, off)
		if err != nil && errors.Is(err, syscall.EINTR) {
			// A signal may interrupt pread after it has already stored bytes;
			// consume them before retrying so they aren't re-read and duplicated.
			if n > 0 {
				total += n
				p = p[n:]
				off += int64(n)
			}
			continue
		}
		if err != nil {
			return total + n, err
		}
		if n == 0 {
			return total, io.EOF
		}
		total += n
		p = p[n:]
		off += int64(n)
	}
	return total, nil
}
