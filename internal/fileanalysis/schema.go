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
	// Version 8 adds KnownNetworkLibDeps to SymbolAnalysisData.
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
	CurrentSchemaVersion = 16
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

	// DynLibDeps contains the dynamic library dependency snapshot recorded at record time.
	// Only present for ELF binaries with DT_NEEDED entries.
	DynLibDeps []LibEntry `json:"dyn_lib_deps,omitempty"`

	// SymbolAnalysis contains the symbol analysis result cached at record time.
	// nil means not analyzed (static binary, non-ELF, or old schema record).
	SymbolAnalysis *SymbolAnalysisData `json:"symbol_analysis,omitempty"`

	// ShebangInterpreter holds interpreter information parsed from the file's
	// shebang line. nil for non-script files (ELF binaries, text files, etc.).
	ShebangInterpreter *ShebangInterpreterInfo `json:"shebang_interpreter,omitempty"`

	// AnalysisWarnings holds non-fatal warnings generated during dynlib analysis
	// (e.g., unknown @ tokens that prevent hash verification for specific libraries).
	// nil/empty is omitted from JSON (omitempty).
	AnalysisWarnings []string `json:"analysis_warnings,omitempty"`
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
