package common

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
	// This includes both direct syscall instructions (opcode 0F 05) and
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
}
