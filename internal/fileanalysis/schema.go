package fileanalysis

import (
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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

// SyscallInfo is an alias for common.SyscallInfo.
// Using a type alias preserves backward compatibility for code that references
// fileanalysis.SyscallInfo while the canonical definition lives in common.
type SyscallInfo = common.SyscallInfo

// SyscallSummary is an alias for common.SyscallSummary.
// Using a type alias preserves backward compatibility for code that references
// fileanalysis.SyscallSummary while the canonical definition lives in common.
type SyscallSummary = common.SyscallSummary

// SyscallAnalysisData contains syscall analysis information.
type SyscallAnalysisData struct {
	// SyscallAnalysisResultCore contains the common fields shared with
	// elfanalyzer.SyscallAnalysisResult. Embedding ensures field-level
	// consistency between packages and enables direct struct copy for
	// type conversion.
	common.SyscallAnalysisResultCore

	// AnalyzedAt is when the syscall analysis was performed.
	AnalyzedAt time.Time `json:"analyzed_at"`
}
