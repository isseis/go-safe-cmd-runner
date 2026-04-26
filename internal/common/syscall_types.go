//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

// DeterminationMethodDirectSVC0x80 indicates the syscall was detected as a
// direct svc #0x80 instruction in a Mach-O arm64 binary, bypassing libSystem.
const DeterminationMethodDirectSVC0x80 = "direct_svc_0x80"

// SyscallArgEvalStatus is a typed string for argument evaluation status values.
type SyscallArgEvalStatus string

const (
	// SyscallArgEvalExecConfirmed indicates prot value was obtained
	// and PROT_EXEC flag (0x4) is set.
	SyscallArgEvalExecConfirmed SyscallArgEvalStatus = "exec_confirmed"

	// SyscallArgEvalExecUnknown indicates prot value could not be
	// statically determined.
	SyscallArgEvalExecUnknown SyscallArgEvalStatus = "exec_unknown"

	// SyscallArgEvalExecNotSet indicates prot value was obtained
	// and PROT_EXEC flag (0x4) is NOT set.
	SyscallArgEvalExecNotSet SyscallArgEvalStatus = "exec_not_set"
)

// SyscallArgEvalResult represents the static evaluation result
// of a syscall argument.
type SyscallArgEvalResult struct {
	// SyscallName is the syscall being evaluated (e.g., "mprotect").
	SyscallName string `json:"syscall_name"`

	// Status is the evaluation outcome.
	Status SyscallArgEvalStatus `json:"status"`

	// Details provides supplementary info.
	// For exec_confirmed/exec_not_set: prot value (e.g., "prot=0x5").
	// For exec_unknown: reason (e.g., "scan limit exceeded").
	Details string `json:"details,omitempty"`
}

// SyscallOccurrence represents a single occurrence of a syscall.
// Multiple occurrences can be grouped under a single SyscallInfo entry
// when they have the same syscall number.
type SyscallOccurrence struct {
	// Location is the virtual address of the syscall instruction
	// (typically located within the .text section).
	Location uint64 `json:"location"`

	// DeterminationMethod describes how the syscall number was determined.
	// See the DeterminationMethod* constants in the elfanalyzer package for
	// possible values.
	DeterminationMethod string `json:"determination_method"`

	// Source describes how this syscall was detected.
	// Empty string (omitted in JSON) means detection via direct syscall instruction.
	// "libc_symbol_import" means detection via libc import symbol matching.
	Source string `json:"source,omitempty"`
}

// SyscallInfo represents information about a detected syscall,
// potentially with multiple occurrences (all with the same syscall number).
// An occurrence can be either a direct syscall instruction or an indirect syscall
// via a Go wrapper function call.
type SyscallInfo struct {
	// Number is the syscall number (e.g., 41 for socket on x86_64).
	// -1 indicates the number could not be determined.
	Number int `json:"number"`

	// Name is the human-readable syscall name (e.g., "socket").
	// Empty if the number is unknown or not in the table.
	Name string `json:"name,omitempty"`

	// IsNetwork indicates whether this syscall is network-related.
	IsNetwork bool `json:"is_network"`

	// Occurrences contains all detected occurrences of this syscall.
	// Multiple occurrences with the same number are grouped together.
	// Occurrences should be sorted by Location in ascending order.
	Occurrences []SyscallOccurrence `json:"occurrences,omitempty"`
}

// SyscallAnalysisResultCore contains the common fields shared between
// elfanalyzer.SyscallAnalysisResult and fileanalysis.SyscallAnalysisResult.
// Both packages embed this type to ensure field-level consistency at the
// type level. Each package may add its own package-specific fields.
type SyscallAnalysisResultCore struct {
	// Architecture is the ELF machine architecture that was analyzed (e.g., "x86_64").
	Architecture string `json:"architecture"`

	// DetectedSyscalls contains all detected syscall events with their numbers.
	// This includes both direct architecture-specific syscall instructions and
	// indirect syscalls via Go wrapper function calls (e.g., syscall.Syscall).
	DetectedSyscalls []SyscallInfo `json:"detected_syscalls"`

	// AnalysisWarnings contains observations and warnings generated during analysis.
	// Examples: "syscall number could not be determined", "mprotect PROT_EXEC confirmed".
	// Note: With omitempty, nil and empty slice ([]string{}) have different JSON output:
	//   - nil: field is omitted entirely
	//   - []string{}: field appears as "analysis_warnings": []
	// When initializing, use nil (not empty slice) for no warnings
	// to ensure the field is omitted in JSON output.
	AnalysisWarnings []string `json:"analysis_warnings,omitempty"`

	// ArgEvalResults contains static evaluation results for syscall arguments.
	// Currently used for mprotect PROT_EXEC detection.
	// Only populated when relevant syscalls are detected; otherwise nil.
	ArgEvalResults []SyscallArgEvalResult `json:"arg_eval_results,omitempty"`
}
