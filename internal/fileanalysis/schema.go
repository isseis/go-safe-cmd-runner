package fileanalysis

import (
	"time"
)

const (
	// CurrentSchemaVersion is the current analysis record schema version.
	// Increment this when making breaking changes to the analysis record format.
	CurrentSchemaVersion = 1
)

// Record represents a unified file analysis record containing both
// hash validation and syscall analysis data.
// Note: This type was renamed from FileAnalysisRecord to avoid stuttering
// (fileanalysis.Record instead of fileanalysis.FileAnalysisRecord).
type Record struct {
	// SchemaVersion identifies the analysis record format version.
	SchemaVersion int `json:"schema_version"`

	// FilePath is the absolute path to the analyzed file.
	FilePath string `json:"file_path"`

	// ContentHash is the SHA256 hash of the file content in prefixed format.
	// Format: "sha256:<64-char-hex>" (e.g., "sha256:abc123...def789")
	// Note: filevalidator.SHA256.Sum() returns unprefixed hex, so callers
	// must prepend "sha256:" prefix when constructing ContentHash values.
	// Example: fmt.Sprintf("%s:%s", hashAlgo.Name(), rawHash)
	// This prefixed format ensures consistency with record command output
	// and enables future support for multiple hash algorithms.
	// Used by both filevalidator and elfanalyzer for validation.
	ContentHash string `json:"content_hash"`

	// UpdatedAt is when the analysis record was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// SyscallAnalysis contains syscall analysis result (optional).
	// Only present for static ELF binaries that have been analyzed.
	SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`
}

// SyscallInfo represents information about a single detected syscall event.
// This type mirrors elfanalyzer.SyscallInfo to avoid import cycles.
// The SyscallAnalysisStore converts between the two types.
type SyscallInfo struct {
	// Number is the syscall number (e.g., 41 for socket on x86_64).
	// -1 indicates the number could not be determined.
	Number int `json:"number"`

	// Name is the human-readable syscall name (e.g., "socket").
	// Empty if the number is unknown or not in the table.
	Name string `json:"name,omitempty"`

	// IsNetwork indicates whether this syscall is network-related.
	IsNetwork bool `json:"is_network"`

	// Location is the virtual address of the syscall instruction.
	Location uint64 `json:"location"`

	// DeterminationMethod describes how the syscall number was determined.
	DeterminationMethod string `json:"determination_method"`
}

// SyscallSummary provides aggregated analysis information.
// This type mirrors elfanalyzer.SyscallSummary to avoid import cycles.
type SyscallSummary struct {
	// HasNetworkSyscalls indicates presence of network-related syscalls.
	HasNetworkSyscalls bool `json:"has_network_syscalls"`

	// IsHighRisk indicates the analysis could not fully determine network capability.
	IsHighRisk bool `json:"is_high_risk"`

	// TotalDetectedEvents is the count of detected syscall events.
	TotalDetectedEvents int `json:"total_detected_events"`

	// NetworkSyscallCount is the count of network-related syscall events.
	NetworkSyscallCount int `json:"network_syscall_count"`
}

// SyscallAnalysisData contains syscall analysis information.
type SyscallAnalysisData struct {
	// Architecture is the target architecture (e.g., "x86_64").
	Architecture string `json:"architecture"`

	// AnalyzedAt is when the syscall analysis was performed.
	AnalyzedAt time.Time `json:"analyzed_at"`

	// DetectedSyscalls contains all syscall instructions found.
	DetectedSyscalls []SyscallInfo `json:"detected_syscalls"`

	// HasUnknownSyscalls indicates if any syscall number could not be determined.
	HasUnknownSyscalls bool `json:"has_unknown_syscalls"`

	// HighRiskReasons explains why the analysis resulted in high risk.
	// Note: With omitempty, nil and empty slice ([]string{}) have different JSON output:
	//   - nil: field is omitted entirely
	//   - []string{}: field appears as "high_risk_reasons": []
	// When initializing SyscallAnalysisResult, use nil (not empty slice) for no high risk
	// to ensure the field is omitted in JSON output.
	HighRiskReasons []string `json:"high_risk_reasons,omitempty"`

	// Summary provides aggregated information about the analysis.
	Summary SyscallSummary `json:"summary"`
}
