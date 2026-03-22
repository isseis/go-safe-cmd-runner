//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

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

// SyscallInfo represents information about a single detected syscall event.
// An event can be either a direct syscall instruction or an indirect syscall
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

// SyscallSummary provides aggregated analysis information.
type SyscallSummary struct {
	// HasNetworkSyscalls indicates presence of network-related syscalls.
	HasNetworkSyscalls bool `json:"has_network_syscalls"`

	// IsHighRisk indicates the analysis could not fully determine network capability.
	IsHighRisk bool `json:"is_high_risk"`

	// TotalDetectedEvents is the count of detected syscall events.
	// This includes both direct syscall instructions and indirect syscalls
	// via Go wrapper function calls.
	TotalDetectedEvents int `json:"total_detected_events"`

	// NetworkSyscallCount is the count of network-related syscall events.
	NetworkSyscallCount int `json:"network_syscall_count"`
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

	// HasUnknownSyscalls indicates whether any syscall number could not be determined.
	HasUnknownSyscalls bool `json:"has_unknown_syscalls"`

	// HighRiskReasons explains why the analysis resulted in high risk, if applicable.
	// Note: With omitempty, nil and empty slice ([]string{}) have different JSON output:
	//   - nil: field is omitted entirely
	//   - []string{}: field appears as "high_risk_reasons": []
	// When initializing, use nil (not empty slice) for no high risk
	// to ensure the field is omitted in JSON output.
	HighRiskReasons []string `json:"high_risk_reasons,omitempty"`

	// Summary provides aggregated information about the analysis.
	Summary SyscallSummary `json:"summary"`

	// ArgEvalResults contains static evaluation results for syscall arguments.
	// Currently used for mprotect PROT_EXEC detection.
	// Only populated when relevant syscalls are detected; otherwise nil.
	ArgEvalResults []SyscallArgEvalResult `json:"arg_eval_results,omitempty"`
}
