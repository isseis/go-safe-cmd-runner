package risktypes

// ReasonCode is the machine-readable code for an evaluation reason. It is a
// string-derived type defined through constants; raw string literals must not be
// used at call sites (this guards against typos, aids discoverability, and lets
// a test verify the set of codes is exhaustive and distinct).
//
// Each constant's string value is snake_case English.
type ReasonCode string

const (
	// ReasonDestructiveFileOperation marks a destructive file operation in the argv.
	ReasonDestructiveFileOperation ReasonCode = "destructive_file_operation"
	// ReasonSystemModification marks a system-modification command.
	ReasonSystemModification ReasonCode = "system_modification"
	// ReasonPrivilegeEscalation marks privilege escalation (always Critical).
	ReasonPrivilegeEscalation ReasonCode = "privilege_escalation"
	// ReasonCoreutilsClassification marks classification by the coreutils dimension.
	ReasonCoreutilsClassification ReasonCode = "coreutils_classification"

	// Profile-derived factors, one code per factor so audit can distinguish the
	// profile origin from a runtime/argument origin.

	// ReasonProfilePrivilege marks the profile's PrivilegeRisk factor.
	ReasonProfilePrivilege ReasonCode = "profile_privilege"
	// ReasonProfileDestruction marks the profile's DestructionRisk factor.
	ReasonProfileDestruction ReasonCode = "profile_destruction"
	// ReasonProfileDataExfil marks the profile's DataExfilRisk factor.
	ReasonProfileDataExfil ReasonCode = "profile_data_exfil"
	// ReasonProfileNetwork marks the profile's NetworkRisk factor.
	ReasonProfileNetwork ReasonCode = "profile_network"
	// ReasonProfileSystemMod marks the profile's SystemModRisk factor.
	ReasonProfileSystemMod ReasonCode = "profile_system_mod"

	// Binary-analysis signal codes.

	// ReasonBinaryAnalysisNetwork marks network signals from binary analysis.
	ReasonBinaryAnalysisNetwork ReasonCode = "binary_analysis_network"
	// ReasonBinaryAnalysisDynamicLoad marks dynamic loading (dlopen) signals.
	ReasonBinaryAnalysisDynamicLoad ReasonCode = "binary_analysis_dynamic_load"
	// ReasonBinaryAnalysisExec marks exec signals.
	ReasonBinaryAnalysisExec ReasonCode = "binary_analysis_exec"
	// ReasonBinaryAnalysisSVC marks raw service-call (svc) signals.
	ReasonBinaryAnalysisSVC ReasonCode = "binary_analysis_svc"
	// ReasonBinaryAnalysisMprotectExec marks mprotect-to-executable signals.
	ReasonBinaryAnalysisMprotectExec ReasonCode = "binary_analysis_mprotect_exec"

	// Uncertain (fail-closed) codes.

	// ReasonUncertainMissingRecord marks a missing analysis record for the binary.
	ReasonUncertainMissingRecord ReasonCode = "uncertain_missing_record"
	// ReasonUncertainSchemaMismatch marks an analysis record whose schema did not match.
	ReasonUncertainSchemaMismatch ReasonCode = "uncertain_schema_mismatch"
	// ReasonUncertainHashMismatch marks a content hash that did not match the record.
	ReasonUncertainHashMismatch ReasonCode = "uncertain_hash_mismatch"
	// ReasonUncertainUnsupportedFormat marks an unsupported binary format.
	ReasonUncertainUnsupportedFormat ReasonCode = "uncertain_unsupported_format"
	// ReasonUncertainUnverifiedIdentity marks an identity that could not be verified.
	ReasonUncertainUnverifiedIdentity ReasonCode = "uncertain_unverified_identity"
	// ReasonAnalysisDisabled marks analysis/verification being disabled in the environment.
	ReasonAnalysisDisabled ReasonCode = "analysis_disabled"

	// Runtime argument / form codes.

	// ReasonArbitraryCodeExecution marks an arbitrary-code-execution runner (shell,
	// interpreter, build/task runner).
	ReasonArbitraryCodeExecution ReasonCode = "arbitrary_code_execution"
	// ReasonDangerousArgPattern marks a dangerous argument pattern (rm -rf, dd if=, ...).
	ReasonDangerousArgPattern ReasonCode = "dangerous_arg_pattern"
	// ReasonSymlinkResolutionFailed marks a symlink resolution failure (blocking).
	ReasonSymlinkResolutionFailed ReasonCode = "symlink_resolution_failed"
	// ReasonIdentityUnbound marks an identity that could not be bound until exec.
	ReasonIdentityUnbound ReasonCode = "identity_unbound"
	// ReasonIdentityHashMismatch marks an identity whose open-time content hash
	// did not match the hash verified at group verification time.
	ReasonIdentityHashMismatch ReasonCode = "identity_hash_mismatch"
	// ReasonIdentityNotRegular marks an identity whose resolved path was not a
	// regular file at open time (e.g. replaced with a FIFO or device node).
	ReasonIdentityNotRegular ReasonCode = "identity_not_regular_file"
	// ReasonIndirectExecutionRejected marks a rejected indirect-execution form.
	ReasonIndirectExecutionRejected ReasonCode = "indirect_execution_rejected"
	// ReasonIndirectExecutionWrapper marks an allowable wrapper indirection
	// (env/timeout/nice ...) that contributes a risk floor without being rejected.
	ReasonIndirectExecutionWrapper ReasonCode = "indirect_execution_wrapper"
	// ReasonForbiddenEnvVar marks a forbidden environment variable being supplied.
	ReasonForbiddenEnvVar ReasonCode = "forbidden_env_var"
	// ReasonNonAbsolutePath marks a command path that was not absolute when it
	// reached the evaluator (identity cannot be established; fail-closed).
	ReasonNonAbsolutePath ReasonCode = "non_absolute_path"
	// ReasonNetworkArgument marks a network-style argument (URL or SSH-style
	// address) detected on a command without a network profile.
	ReasonNetworkArgument ReasonCode = "network_argument"

	// Destination-path trust-zoning codes (axis 2). Each marks the origin of a
	// level contributed by classifying a file operation's destination/source
	// operands by trust zone.

	// ReasonTrustBoundaryWrite marks a write/delete to a trust-critical path.
	ReasonTrustBoundaryWrite ReasonCode = "trust_boundary_write"
	// ReasonDestinationZone marks a Medium from an ordinary destination.
	ReasonDestinationZone ReasonCode = "destination_zone"
	// ReasonPermissionGrant marks a setuid/world-writable/ownership/attribute grant.
	ReasonPermissionGrant ReasonCode = "permission_grant"
	// ReasonDeviceIO marks dd I/O to a block or dangerous character device.
	ReasonDeviceIO ReasonCode = "device_io"
	// ReasonRecursiveOutsideSafeZone marks recursion reaching outside the safe-zone.
	ReasonRecursiveOutsideSafeZone ReasonCode = "recursive_outside_safe_zone"
	// ReasonSensitiveSourceCopy marks a copy whose source is sensitive/trust-critical.
	ReasonSensitiveSourceCopy ReasonCode = "sensitive_source_copy"
	// ReasonUnresolvedDestination marks a fail-closed unresolved operand.
	ReasonUnresolvedDestination ReasonCode = "unresolved_destination"
)

// ReasonFamily groups reason codes by their origin so the audit stream can
// mechanically distinguish, for incident correlation, whether a reason came from
// name/profile classification (axis 1), destination trust-zoning (axis 2), a
// runtime argument, and so on. It is audit metadata only and does not affect the
// allow/deny decision.
type ReasonFamily string

const (
	// FamilyNameClassification is name/profile-derived classification (axis 1).
	FamilyNameClassification ReasonFamily = "name_classification"
	// FamilyPrivilege is privilege escalation.
	FamilyPrivilege ReasonFamily = "privilege"
	// FamilyBinaryAnalysis is derived from binary-analysis signals.
	FamilyBinaryAnalysis ReasonFamily = "binary_analysis"
	// FamilyUncertain is a fail-closed uncertainty.
	FamilyUncertain ReasonFamily = "uncertain"
	// FamilyRuntimeArgument is derived from the runtime, arguments, or invocation form.
	FamilyRuntimeArgument ReasonFamily = "runtime_argument"
	// FamilyPathTrustZone is destination-path trust-zoning (axis 2).
	FamilyPathTrustZone ReasonFamily = "path_trust_zone"
)

// reasonFamilies maps every ReasonCode to its family. This table is the canonical
// enumeration of all reason codes: the exhaustiveness and family-assignment tests
// derive their code set from its keys rather than a parallel hardcoded list, so a
// newly added ReasonCode is caught only if it is also added here (Go cannot
// reflect over a const block). Adding a new code therefore requires adding it to
// this table.
var reasonFamilies = map[ReasonCode]ReasonFamily{
	// Name/profile classification (axis 1).
	ReasonDestructiveFileOperation: FamilyNameClassification,
	ReasonSystemModification:       FamilyNameClassification,
	ReasonCoreutilsClassification:  FamilyNameClassification,
	ReasonProfileDestruction:       FamilyNameClassification,
	ReasonProfileDataExfil:         FamilyNameClassification,
	ReasonProfileNetwork:           FamilyNameClassification,
	ReasonProfileSystemMod:         FamilyNameClassification,

	// Privilege.
	ReasonPrivilegeEscalation: FamilyPrivilege,
	ReasonProfilePrivilege:    FamilyPrivilege,

	// Binary analysis.
	ReasonBinaryAnalysisNetwork:      FamilyBinaryAnalysis,
	ReasonBinaryAnalysisDynamicLoad:  FamilyBinaryAnalysis,
	ReasonBinaryAnalysisExec:         FamilyBinaryAnalysis,
	ReasonBinaryAnalysisSVC:          FamilyBinaryAnalysis,
	ReasonBinaryAnalysisMprotectExec: FamilyBinaryAnalysis,

	// Uncertain (fail-closed).
	ReasonUncertainMissingRecord:      FamilyUncertain,
	ReasonUncertainSchemaMismatch:     FamilyUncertain,
	ReasonUncertainHashMismatch:       FamilyUncertain,
	ReasonUncertainUnsupportedFormat:  FamilyUncertain,
	ReasonUncertainUnverifiedIdentity: FamilyUncertain,
	ReasonAnalysisDisabled:            FamilyUncertain,

	// Runtime, argument, or invocation form.
	ReasonArbitraryCodeExecution:    FamilyRuntimeArgument,
	ReasonDangerousArgPattern:       FamilyRuntimeArgument,
	ReasonSymlinkResolutionFailed:   FamilyRuntimeArgument,
	ReasonIdentityUnbound:           FamilyRuntimeArgument,
	ReasonIdentityHashMismatch:      FamilyRuntimeArgument,
	ReasonIdentityNotRegular:        FamilyRuntimeArgument,
	ReasonIndirectExecutionRejected: FamilyRuntimeArgument,
	ReasonIndirectExecutionWrapper:  FamilyRuntimeArgument,
	ReasonForbiddenEnvVar:           FamilyRuntimeArgument,
	ReasonNonAbsolutePath:           FamilyRuntimeArgument,
	ReasonNetworkArgument:           FamilyRuntimeArgument,

	// Destination-path trust-zoning (axis 2).
	ReasonTrustBoundaryWrite:       FamilyPathTrustZone,
	ReasonDestinationZone:          FamilyPathTrustZone,
	ReasonPermissionGrant:          FamilyPathTrustZone,
	ReasonDeviceIO:                 FamilyPathTrustZone,
	ReasonRecursiveOutsideSafeZone: FamilyPathTrustZone,
	ReasonSensitiveSourceCopy:      FamilyPathTrustZone,
	ReasonUnresolvedDestination:    FamilyPathTrustZone,
}

// FamilyOf returns the family a reason code belongs to. A code absent from the
// table returns ("", false); the family-assignment test treats that as a missing
// assignment to be fixed rather than a runtime condition.
func FamilyOf(code ReasonCode) (ReasonFamily, bool) {
	f, ok := reasonFamilies[code]
	return f, ok
}
