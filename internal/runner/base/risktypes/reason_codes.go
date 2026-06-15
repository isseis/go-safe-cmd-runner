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
	// ReasonIndirectExecutionRejected marks a rejected indirect-execution form.
	ReasonIndirectExecutionRejected ReasonCode = "indirect_execution_rejected"
	// ReasonForbiddenEnvVar marks a forbidden environment variable being supplied.
	ReasonForbiddenEnvVar ReasonCode = "forbidden_env_var"
)
