package fileanalysis

import (
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

const (
	// CurrentSchemaVersion is the current analysis record schema version.
	// Version 2 adds DynLibDeps and HasDynamicLoad fields.
	// Version 3 adds NetworkSymbolAnalysis (now renamed SymbolAnalysis) and removes HasDynamicLoad.
	// Version 4 renames network_symbol_analysis to symbol_analysis and removes has_network_symbols.
	// Version 5 adds ArgEvalResults for syscall argument evaluation (mprotect PROT_EXEC detection).
	// Version 6 removes is_high_risk from summary and renames high_risk_reasons to analysis_warnings.
	// Version 7 adds pkey_mprotect PROT_EXEC detection.
	// Version 8 adds KnownNetworkLibDeps to SymbolAnalysisData.
	// Version 9 removes per-sub-analysis timestamps (DynLibDepsData.RecordedAt,
	// SyscallAnalysisData.AnalyzedAt, SymbolAnalysisData.AnalyzedAt); use Record.UpdatedAt instead.
	// Version 10 flattens dyn_lib_deps from {"libs": [...]} to [...] directly.
	// Load returns SchemaVersionMismatchError for records with schema_version != 10.
	// Store.Update treats older schemas (Actual < Expected) as overwritable;
	// re-running `record` migrates old-schema records automatically (--force not required).
	// Store.Update rejects newer schemas (Actual > Expected) to preserve forward compatibility.
	CurrentSchemaVersion = 10
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
	// Present when at least one syscall was detected (via direct syscall instruction
	// or libc symbol import). Nil for non-ELF files and ELF binaries with no
	// detected syscalls.
	SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`

	// DynLibDeps contains the dynamic library dependency snapshot recorded at record time.
	// Only present for ELF binaries with DT_NEEDED entries.
	DynLibDeps []LibEntry `json:"dyn_lib_deps,omitempty"`

	// SymbolAnalysis contains the symbol analysis result cached at record time.
	// nil means not analyzed (static binary, non-ELF, or old schema record).
	SymbolAnalysis *SymbolAnalysisData `json:"symbol_analysis,omitempty"`
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

// SyscallAnalysisData contains syscall analysis information.
type SyscallAnalysisData struct {
	// SyscallAnalysisResultCore contains the common fields shared with
	// elfanalyzer.SyscallAnalysisResult. Embedding ensures field-level
	// consistency between packages and enables direct struct copy for
	// type conversion.
	common.SyscallAnalysisResultCore
}

// SymbolAnalysisData holds the symbol analysis result cached at record time.
// Covers both network-related symbols (DetectedSymbols) and dynamic-load symbols (DynamicLoadSymbols).
// nil means not analyzed (static binary, non-ELF, or old schema record).
type SymbolAnalysisData struct {
	// DetectedSymbols contains all network-related symbols found (excluding dynamic_load category).
	// Non-empty when network symbols were detected.
	DetectedSymbols []DetectedSymbolEntry `json:"detected_symbols,omitempty"`

	// DynamicLoadSymbols contains the dynamic library loading symbols found (dlopen, dlsym, dlvsym).
	// Empty when none were detected.
	// HasDynamicLoad is derived as len(DynamicLoadSymbols) > 0; no separate field.
	DynamicLoadSymbols []DetectedSymbolEntry `json:"dynamic_load_symbols,omitempty"`

	// KnownNetworkLibDeps lists SOName values of known network libraries
	// detected from DynLibDeps during record.
	// If non-empty, this binary is treated as network-capable.
	KnownNetworkLibDeps []string `json:"known_network_lib_deps,omitempty"`
}

// DetectedSymbolEntry represents a single detected symbol in the analysis record.
type DetectedSymbolEntry struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}
