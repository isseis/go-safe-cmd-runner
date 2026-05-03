package dynlibanalysisstore

import "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"

const (
	// DynLibAnalysisSchemaVersion is the current schema version for
	// dynamic library analysis store files.
	// Increment when making backward-incompatible schema changes.
	//
	// Version history:
	//   1 - initial schema
	DynLibAnalysisSchemaVersion = 1
)

// DynamicLibAnalysisFile is the JSON schema for a dynamic library analysis store file.
// Each file stores analysis results for a single library identified by lib_path and lib_hash.
type DynamicLibAnalysisFile struct {
	SchemaVersion      int                               `json:"schema_version"`
	LibPath            string                            `json:"lib_path"`
	LibHash            string                            `json:"lib_hash"`
	SyscallAnalysis    *fileanalysis.SyscallAnalysisData `json:"syscall_analysis,omitempty"`
	SymbolAnalysis     *fileanalysis.SymbolAnalysisData  `json:"symbol_analysis,omitempty"`
	DynamicLoadSymbols []string                          `json:"dynamic_load_symbols,omitempty"`
}

// DynamicLibAnalysisResult holds the in-memory result of a dynamic library analysis.
// Warnings contain non-fatal messages generated during analysis and are not persisted to disk.
type DynamicLibAnalysisResult struct {
	SyscallAnalysis    *fileanalysis.SyscallAnalysisData
	SymbolAnalysis     *fileanalysis.SymbolAnalysisData
	DynamicLoadSymbols []string
	Warnings           []string
}
