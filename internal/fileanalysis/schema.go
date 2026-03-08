package fileanalysis

import (
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

const (
	// CurrentSchemaVersion is the current analysis record schema version.
	// Version 2 adds DynLibDeps and HasDynamicLoad fields.
	// Load returns SchemaVersionMismatchError for records with schema_version != 2.
	// Store.Update treats older schemas (Actual < Expected) as overwritable
	// (enables `record --force` migration).
	// Store.Update rejects newer schemas (Actual > Expected) to preserve forward compatibility.
	CurrentSchemaVersion = 2
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

	// DynLibDeps contains the dynamic library dependency snapshot recorded at record time.
	// Only present for ELF binaries with DT_NEEDED entries.
	DynLibDeps *DynLibDepsData `json:"dyn_lib_deps,omitempty"`

	// HasDynamicLoad indicates that dlopen/dlsym/dlvsym symbols were found in the binary
	// at record time. This is an informational snapshot; the runner re-runs live binary
	// analysis at runtime and does NOT read this field directly.
	HasDynamicLoad bool `json:"has_dynamic_load,omitempty"`
}

// DynLibDepsData contains the dynamic library dependency snapshot.
type DynLibDepsData struct {
	RecordedAt time.Time  `json:"recorded_at"`
	Libs       []LibEntry `json:"libs"`
}

// LibEntry represents a single resolved dynamic library dependency.
type LibEntry struct {
	// SOName is the DT_NEEDED library name (e.g., "libssl.so.3").
	SOName string `json:"soname"`

	// Path is the resolved full path to the library file, normalized via
	// filepath.EvalSymlinks + filepath.Clean.
	Path string `json:"path"`

	// Hash is the SHA256 hash of the library file in "sha256:<hex>" format.
	Hash string `json:"hash"`
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
