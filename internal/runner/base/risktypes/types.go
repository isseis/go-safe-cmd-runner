// Package risktypes holds the shared data-transfer objects exchanged between the
// risk evaluator, the audit logger, the security analyzers, and the resource
// managers. It is the lowest neutral package in this area: it depends only on
// runnertypes (for RiskLevel) and the standard library, so the risk and audit
// packages can both reference these types without forming a risk -> audit -> risk
// import cycle.
package risktypes

import (
	"errors"
	"sync/atomic"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// VerifiedFD is a closable wrapper around a verified file descriptor.
//
// Contract:
//   - It owns the underlying descriptor and is the single place that closes it,
//     so callers must not hold or close the raw int separately (this prevents
//     double-close and divided ownership).
//   - Close is idempotent and safe for concurrent use: it guarantees the
//     underlying descriptor is closed exactly once even if several callers race,
//     and a nil receiver returns nil.
//   - Fd returns the raw descriptor for callers that build /proc/self/fd/<n>
//     paths or pass the descriptor to a child process.
//
// The descriptor itself is opened and consumed in Phase 2 (fd-bound execution);
// this type only declares the ownership contract.
type VerifiedFD struct {
	fd     int
	closed atomic.Bool
}

// NewVerifiedFD wraps an already-opened, verified file descriptor.
func NewVerifiedFD(fd int) *VerifiedFD {
	return &VerifiedFD{fd: fd}
}

// Fd returns the raw file descriptor. The descriptor remains owned by the
// VerifiedFD; callers must not close it directly.
func (f *VerifiedFD) Fd() int {
	return f.fd
}

// Close closes the underlying descriptor. It is idempotent, safe for concurrent
// use, and safe to call on a nil receiver. The atomic swap ensures syscall.Close
// runs for exactly one caller, avoiding a double-close race (CWE-1341).
func (f *VerifiedFD) Close() error {
	if f == nil {
		return nil
	}
	if f.closed.Swap(true) {
		return nil
	}
	return syscall.Close(f.fd)
}

// VerifiedIdentity binds a verified executable to the moment of exec.
//
// A VerifiedIdentity exists only when verification succeeded, so ResolvedPath and
// ContentHash always hold real values here; "absent" is represented by a nil
// *VerifiedIdentity on the owning plan, never by a sentinel empty string.
type VerifiedIdentity struct {
	// FD is the verified file descriptor used for fd-bound execution where the
	// environment supports it. nil means no descriptor is held.
	FD *VerifiedFD
	// ResolvedPath is the verified absolute path of the executable.
	ResolvedPath string
	// ContentHash is the finalized content hash of the verified executable.
	ContentHash string
}

// VerifiedCommandPlan is the confirmed, ready-to-exec description of a command.
// The executor binds the executed inode to the verified Identity.FD rather than
// re-resolving the path. The execution fields (ResolvedPath/ResolvedArgv/
// ResolvedEnv) are set only for allowed (executable) plans; for plans denied
// before verification completes they are empty, and the audit resolved_path is
// derived from Identity rather than from these fields.
//
// Note: ResolvedArgv/ResolvedEnv are populated for allowed plans but are not yet
// consumed by the executor (which currently takes argv/env from the command);
// binding argv/env from the plan is a future step.
type VerifiedCommandPlan struct {
	ResolvedPath string            // absolute path to exec (allowed plans only)
	ResolvedArgv []string          // final argv after indirect-execution expansion (allowed plans only)
	ResolvedEnv  map[string]string // validated env with forbidden variables removed (allowed plans only)
	Identity     *VerifiedIdentity // identity bound until exec; nil = denied plan with no verified identity
	Artifacts    []ExecutedArtifact
	Assessment   RiskAssessment
}

// Close releases every verified file descriptor owned by the plan: the command's
// own identity and each chained artifact's identity. It aggregates errors with
// errors.Join and is safe to call on a zero plan and to call more than once
// (VerifiedFD.Close is idempotent), so the command's owner can defer it
// unconditionally for allowed, denied, and preview plans alike.
func (p *VerifiedCommandPlan) Close() error {
	var errs []error
	if p.Identity != nil {
		errs = append(errs, p.Identity.FD.Close())
	}
	for i := range p.Artifacts {
		if id := p.Artifacts[i].Identity; id != nil {
			errs = append(errs, id.FD.Close())
		}
	}
	return errors.Join(errs...)
}

// RiskAssessment is the effective risk and the reasoning behind it.
type RiskAssessment struct {
	// Level is the effective risk, the maximum across all evaluated dimensions.
	Level runnertypes.RiskLevel
	// Blocking, when true, denies execution regardless of the configured
	// risk_level (uncertain analysis or an identity that cannot be bound).
	Blocking bool
	// BlockingReason is the machine-readable deny reason. It is set for every
	// deny, whether Blocking is true or Level is Critical; it is empty when the
	// command is allowed.
	BlockingReason ReasonCode
	// ErrorClass classifies a failure-induced deny (symlink resolution failure,
	// coreutils file-info failure, etc.). Empty for non-failure denies and for
	// allowed commands. It is copied verbatim into the audit entry.
	ErrorClass ErrorClass
	// ReasonCodes carries the machine-readable evaluation reasons.
	ReasonCodes []ReasonCode
	// Reasons carries human-readable reasons (e.g. profile-derived reasons).
	// Emitted as risk_factors in the audit log.
	Reasons []string
	// NetworkType is recorded for audit (none/always/conditional).
	NetworkType string
}

// BinaryAnalysisClass is the classification produced by binary signal analysis.
// The zero value is the safest one, Uncertain (fail-closed): a value left
// uninitialized never falls through to "safe" (Low).
type BinaryAnalysisClass int

const (
	// BinaryAnalysisUncertain (zero value) means the analysis could not reach a
	// confident verdict (missing record, schema mismatch, unsupported format,
	// unexpected state) and the command must be blocked.
	BinaryAnalysisUncertain BinaryAnalysisClass = iota
	// BinaryAnalysisClean means no dangerous signal was found -> Low.
	BinaryAnalysisClean
	// BinaryAnalysisNetwork means only network signals were found -> Medium.
	BinaryAnalysisNetwork
	// BinaryAnalysisHighRisk means dangerous signals (dlopen/exec/svc/mprotect)
	// were found -> High.
	BinaryAnalysisHighRisk
)

// BinaryAnalysisResult carries the classification together with the
// machine-readable reason codes that justified it, so the originating signals
// are preserved (collapsing to Class alone would lose the reasoning).
type BinaryAnalysisResult struct {
	Class       BinaryAnalysisClass
	ReasonCodes []ReasonCode
}

// Decision is the final audit verdict. Per the audit contract it is the two
// values allow/deny only; an error-induced abort is recorded as deny with the
// failure kind carried separately in ErrorClass.
type Decision string

const (
	// DecisionAllow indicates the command was allowed.
	DecisionAllow Decision = "allow"
	// DecisionDeny indicates the command was denied.
	DecisionDeny Decision = "deny"
)

// ExecutionMode is the audit execution mode. It is a local string type to avoid
// a resource -> audit -> resource import cycle.
type ExecutionMode string

const (
	// ModeNormal is normal (executing) mode.
	ModeNormal ExecutionMode = "normal"
	// ModeDryRun is dry-run (preview) mode.
	ModeDryRun ExecutionMode = "dry-run"
)

// ErrorClass classifies a failure-induced deny.
type ErrorClass string

const (
	// ErrorClassSymlinkResolution is a symlink resolution failure.
	ErrorClassSymlinkResolution ErrorClass = "symlink_resolution"
	// ErrorClassCoreutilsFileInfo is a coreutils file-info lookup failure.
	ErrorClassCoreutilsFileInfo ErrorClass = "coreutils_file_info"
	// ErrorClassRecordLoad is an analysis record load failure.
	ErrorClassRecordLoad ErrorClass = "record_load"
	// ErrorClassPathResolution is a command path resolution failure.
	ErrorClassPathResolution ErrorClass = "path_resolution"
	// ErrorClassRiskLevelConfig is an invalid risk_level configuration value.
	ErrorClassRiskLevelConfig ErrorClass = "risk_level_config"
)

// ArtifactRole is the role of an artifact within an indirect-execution chain.
type ArtifactRole string

const (
	// RoleWrapper is a wrapper command (env/timeout/nice/...).
	RoleWrapper ArtifactRole = "wrapper"
	// RoleInner is the inner command extracted from a wrapper.
	RoleInner ArtifactRole = "inner"
	// RoleInterpreter is a shell or interpreter resolved from a shebang.
	RoleInterpreter ArtifactRole = "interpreter"
	// RolePreload is a loader preload/library artifact.
	RolePreload ArtifactRole = "preload"
	// RoleExecTarget is the final executed target.
	RoleExecTarget ArtifactRole = "exec-target"
)

// ArtifactDisposition is how an artifact was handled by the gate.
type ArtifactDisposition string

const (
	// DispVerified means the artifact was verified and identity-bound.
	DispVerified ArtifactDisposition = "verified"
	// DispRejected means the artifact could not be bound and was rejected.
	DispRejected ArtifactDisposition = "rejected"
	// DispAllowlistFailed means the artifact failed the allowlist/hash gate.
	DispAllowlistFailed ArtifactDisposition = "allowlist-failed"
)

// ExecutedArtifact is the audit information for a single artifact actually
// executed or loaded during indirect execution. Each artifact in a chain
// (shebang interpreter, loader preload/library, wrapper helper) carries its own
// VerifiedIdentity, since it must be identity-bound until its own exec/load.
// An artifact that cannot be bound is marked DispRejected and its form is
// refused.
type ExecutedArtifact struct {
	// Path identifies the artifact. At the detection stage it may be a command
	// name (a bare wrapped inner command, a shebang interpreter) pending
	// resolution; it is resolved to an absolute path when the execution layer
	// binds the artifact's identity.
	Path string
	// ContentHash is nil when unverified (not used for matching; Identity is the
	// source of truth when present).
	ContentHash *string
	// Identity is the verified identity of this artifact (nil = cannot bind ->
	// rejected).
	Identity *VerifiedIdentity
	// Role is the artifact's role in the chain.
	Role ArtifactRole
	// Disposition is how the artifact was handled.
	Disposition ArtifactDisposition
}

// RiskAuditEntry is the parameter object passed to audit.LogRiskProfile. It is
// defined here (rather than in the audit package) so audit can receive it
// without importing the risk package, avoiding an import cycle.
//
// Values that cannot be obtained are represented by nil pointers (absence),
// never by sentinel strings inside value fields; rendering absence as a fixed
// marker happens only at the log-output boundary.
type RiskAuditEntry struct {
	CommandName string
	// Args are the command arguments recorded for forensic correlation. They are
	// masked at the log-output boundary (the existing redaction mechanism) before
	// being written, so secrets passed as arguments are not leaked.
	Args []string
	Mode ExecutionMode
	// ResolvedPath is nil when the path was not resolved (e.g. deny on resolution failure).
	ResolvedPath *string
	// ContentHash is nil when unverified (not used for matching).
	ContentHash *string
	// RecordID is nil when no analysis record was used.
	RecordID       *string
	Assessment     RiskAssessment
	MaxAllowedRisk runnertypes.RiskLevel
	Decision       Decision
	// VerificationUnavailable marks a dry-run deny caused by analysis/verification
	// being unavailable in the environment (deny, but environment-induced).
	VerificationUnavailable bool
	// ErrorClass classifies a failure-induced deny. For Blocking denies it mirrors
	// Assessment.ErrorClass; on the error-return path where no plan is available
	// the manager sets it directly. Empty when the deny is not failure-induced.
	ErrorClass ErrorClass
	// Chain is every artifact in an indirect-execution chain.
	Chain []ExecutedArtifact
}
