package risktypes

// PathTrustZone classifies a resolved (symlink-followed, absolute) operand path
// by how much trust a write/delete to it breaches. The level mapping is:
// trust-critical -> High, ordinary -> Medium, safe-zone -> Low (Trusted) or
// Medium (fallback), unresolved -> fail-closed (High write / Medium read).
type PathTrustZone string

const (
	// ZoneTrustCritical is a system-critical path (write/delete -> High).
	ZoneTrustCritical PathTrustZone = "trust-critical"
	// ZoneOrdinary is neither critical nor safe (-> Medium).
	ZoneOrdinary PathTrustZone = "ordinary"
	// ZoneSafeZone is a run-owned safe area (-> Low when Trusted, Medium fallback).
	ZoneSafeZone PathTrustZone = "safe-zone"
	// ZoneUnresolved is a path that could not be resolved; fail-closed
	// (High write/delete, Medium read).
	ZoneUnresolved PathTrustZone = "unresolved"
)

// OperandRole distinguishes a write/delete target from a read source. It drives
// the asymmetric fail-closed level for ZoneUnresolved (write=High, read=Medium):
// the worst case of a write is destruction, of a read is information exposure.
type OperandRole string

const (
	// OperandRoleWrite is the destination of a write/delete.
	OperandRoleWrite OperandRole = "write"
	// OperandRoleRead is a source read for a copy/reference (cp source, dd if=).
	OperandRoleRead OperandRole = "read"
)

// OperandZone is the per-operand audit record. It is stored on RiskAssessment so
// both the allow and deny paths can carry it to the audit logger (logger output
// is task 0143). An empty []OperandZone means axis 2 did not apply (not a
// file-operation command); an applied-but-unresolvable operand remains as an
// element with Zone == ZoneUnresolved.
type OperandZone struct {
	// Index is the operand position within the command.
	Index int
	// Raw is the operand as written on the command line.
	Raw string
	// Resolved is the symlink-followed absolute path (empty if unresolved).
	Resolved string
	// Zone is the classified trust zone.
	Zone PathTrustZone
	// Role distinguishes a write/delete target from a read source.
	Role OperandRole
	// MatchedCritical is the SystemCriticalPaths entry matched, if any
	// (set only when Zone == ZoneTrustCritical).
	MatchedCritical string
	// Trusted reports whether the per-operand Trusted predicate was satisfied
	// (a safe-zone operand is Low only when Trusted is true).
	Trusted bool
	// UnresolvedErr is a human-readable cause set when Zone == ZoneUnresolved.
	UnresolvedErr string
}

// RunAsIdent is the precomputed identity used for the Trusted predicate. It is
// resolved from the config run-as values OUTSIDE the zoning judgment; the
// judgment never reads live identity (os.Geteuid/os.Getuid/user.Current) so its
// result is deterministic and identical between dry-run and runtime.
type RunAsIdent struct {
	UID    uint32
	GID    uint32
	Groups []uint32
}
