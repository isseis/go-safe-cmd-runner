package fileanalysis

import (
	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

const (
	// CurrentSchemaVersion is the current analysis record schema version.
	// v16: Pass 1/Pass 2 Mach-O arm64 syscall number analysis support.
	// Version 2 adds DynLibDeps and HasDynamicLoad fields.
	// Version 3 adds NetworkSymbolAnalysis (now renamed SymbolAnalysis) and removes HasDynamicLoad.
	// Version 4 renames network_symbol_analysis to symbol_analysis and removes has_network_symbols.
	// Version 5 adds ArgEvalResults for syscall argument evaluation (mprotect PROT_EXEC detection).
	// Version 6 removes is_high_risk from summary and renames high_risk_reasons to analysis_warnings.
	// Version 7 adds pkey_mprotect PROT_EXEC detection.
	// Version 9 removes per-sub-analysis timestamps (DynLibDepsData.RecordedAt,
	// SyscallAnalysisData.AnalyzedAt, SymbolAnalysisData.AnalyzedAt); consolidated into a record-level timestamp.
	// Version 10 flattens dyn_lib_deps from {"libs": [...]} to [...] directly.
	// Version 11 adds ShebangInterpreter to Record for shebang interpreter tracking.
	// Version 12 adds RawInterpreterPath to ShebangInterpreterInfo for symlink-redirect detection.
	// Version 13 removes UpdatedAt field (was unused by verify; caused noisy diffs).
	// Version 14 adds AnalysisWarnings to Record for dynlib analysis warnings.
	// Mach-O binaries also record DynLibDeps starting with version 14.
	// Version 15 adds Mach-O arm64 svc #0x80 scanning.
	// Version 16 adds Mach-O arm64 Pass 1 (direct svc) and Pass 2 (go_wrapper) syscall
	// number analysis. Records at v16 carry precise syscall numbers with IsNetwork flags.
	// Load returns SchemaVersionMismatchError for records with schema_version != 16.
	// Store.Update treats older schemas (Actual < Expected) as overwritable;
	// re-running `record` migrates old-schema records automatically (--force not required).
	// Store.Update rejects newer schemas (Actual > Expected) to preserve forward compatibility.
	// Version 17 groups detected_syscalls by syscall number. Multiple occurrences with the
	// same syscall number are merged into a single SyscallInfo entry with an Occurrences array.
	// Location, DeterminationMethod, and Source fields are moved from top-level SyscallInfo
	// to individual SyscallOccurrence entries. This change provides clearer structure and
	// proper grouping of syscall instances that share the same number.
	// Version 18 removes is_network from SyscallInfo and category from
	// DetectedSymbolEntry. Risk classification is now derived by the runner
	// at runtime from syscall numbers and symbol names.
	// Version 19 flattens detected_symbols and dynamic_load_symbols from
	// [{"name":"..."}] to ["..."] directly.
	// Version 20 adds LibraryAnalysis to Record for per-library syscall/symbol
	// analysis results and DetectedLibraryNetworkDeps to SymbolAnalysisData.
	// Load returns SchemaVersionMismatchError for records with schema_version != 20.
	// Version 21 removes LibraryAnalysis from Record: per-library results are now stored
	// in the dynamic library analysis store (internal/dynamicanalysis) and read at
	// runner runtime rather than embedded in each executable's record.
	// Version 22 replaces dyn_lib_deps with deps(path/hash only), replaces
	// shebang_interpreter with shebang_chain(ref/path), and adds optional debug output.
	CurrentSchemaVersion = 22
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

	// SyscallAnalysis contains syscall analysis result (optional).
	// Present when at least one syscall was detected (via direct syscall instruction
	// or libc symbol import). Nil for non-ELF files and ELF binaries with no
	// detected syscalls.
	SyscallAnalysis *SyscallAnalysisData `json:"syscall_analysis,omitempty"`

	// DynLibDeps contains dependency entries recorded for hash verification.
	// JSON field name is deps in schema v22.
	DynLibDeps []LibEntry `json:"deps,omitempty"`

	// SymbolAnalysis contains the symbol analysis result cached at record time.
	// nil means not analyzed (static binary, non-ELF, or old schema record).
	SymbolAnalysis *SymbolAnalysisData `json:"symbol_analysis,omitempty"`

	// ShebangChain holds runtime verification entries for shebang resolution.
	// Each entry stores the reference token (optional) and the resolved path.
	ShebangChain []ShebangChainEntry `json:"shebang_chain,omitempty"`

	// AnalysisWarnings holds non-fatal warnings generated during dynlib analysis
	// (e.g., unknown @ tokens that prevent hash verification for specific libraries).
	// nil/empty is omitted from JSON (omitempty).
	AnalysisWarnings []string `json:"analysis_warnings,omitempty"`

	// Debug holds optional debug-only fields emitted when debug-info is enabled.
	Debug *RecordDebug `json:"debug,omitempty"`

	// ShebangInterpreter is kept as an internal migration field and is not serialized.
	// New schema output uses ShebangChain only.
	ShebangInterpreter *ShebangInterpreterInfo `json:"-"`
}

// LibEntry represents a single resolved dynamic library dependency.
type LibEntry struct {
	// SOName is retained for internal matching logic and is not serialized in v22 records.
	SOName string `json:"-"`

	// Path is the resolved full path to the library file, normalized via
	// filepath.EvalSymlinks + filepath.Clean.
	Path string `json:"path"`

	// Hash is the SHA256 hash of the library file in "sha256:<hex>" format.
	Hash string `json:"hash"`
}

// ShebangChainEntry stores one shebang verification hop.
type ShebangChainEntry struct {
	// Ref is the original shebang reference token (absolute path or bare command name).
	// Empty means no runtime re-resolution is required.
	Ref string `json:"ref,omitempty"`

	// Path is the resolved absolute path recorded at record time.
	Path string `json:"path"`
}

// RecordDebug stores optional debug-only metadata.
type RecordDebug struct {
	// DepSources maps dependency paths to source labels that contributed them.
	DepSources map[string][]string `json:"dep_sources,omitempty"`
}

// ShebangInterpreterInfo records the interpreter associated with a script file.
// For direct form (e.g., "#!/bin/sh"), InterpreterPath and RawInterpreterPath are set.
// For env form (e.g., "#!/usr/bin/env python3"), all fields are set.
type ShebangInterpreterInfo struct {
	// RawInterpreterPath is the interpreter path exactly as written in the shebang
	// line, before symlink resolution (e.g., "/bin/sh" or "/usr/bin/env").
	// At verify time this is re-resolved and compared against InterpreterPath to
	// detect symlink redirection attacks.
	// Empty in records written before schema version 12.
	RawInterpreterPath string `json:"raw_interpreter_path,omitempty"`

	// InterpreterPath is the shebang interpreter path, symlink-resolved.
	// For direct form: the interpreter itself (e.g., "/usr/bin/dash").
	// For env form: the env binary path (e.g., "/usr/bin/env").
	InterpreterPath string `json:"interpreter_path"`

	// CommandName is the command passed to env (e.g., "python3").
	// Empty for direct form.
	CommandName string `json:"command_name,omitempty"`

	// ResolvedPath is the PATH-resolved absolute path of CommandName,
	// symlink-resolved (e.g., "/usr/bin/python3.11").
	// Empty for direct form.
	ResolvedPath string `json:"resolved_path,omitempty"`
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
	DetectedSymbols []string `json:"detected_symbols,omitempty"`

	// DynamicLoadSymbols contains the dynamic library loading symbols found (dlopen, dlsym, dlvsym).
	// Empty when none were detected.
	// HasDynamicLoad is derived as len(DynamicLoadSymbols) > 0; no separate field.
	DynamicLoadSymbols []string `json:"dynamic_load_symbols,omitempty"`
}
