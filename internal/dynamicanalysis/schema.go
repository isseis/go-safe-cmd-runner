package dynamicanalysis

import "github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"

const (
	// SchemaVersion is the current schema version for dynamic library analysis
	// store files. Increment when making backward-incompatible schema changes.
	//
	// Version history:
	//   1 - initial schema
	//   2 - remove top-level dynamic_load_symbols; use symbol_analysis.dynamic_load_symbols only
	SchemaVersion = 2
)

// File is the JSON schema for a dynamic library analysis store file.
// Each file stores analysis results for a single library identified by lib_path and lib_hash.
type File struct {
	SchemaVersion   int                               `json:"schema_version"`
	LibPath         string                            `json:"lib_path"`
	LibHash         string                            `json:"lib_hash"`
	SyscallAnalysis *fileanalysis.SyscallAnalysisData `json:"syscall_analysis,omitempty"`
	SymbolAnalysis  *fileanalysis.SymbolAnalysisData  `json:"symbol_analysis,omitempty"`
}

// Result holds the in-memory result of a dynamic library analysis.
// Warnings contain non-fatal messages generated during analysis and are not persisted to disk.
type Result struct {
	SyscallAnalysis *fileanalysis.SyscallAnalysisData
	SymbolAnalysis  *fileanalysis.SymbolAnalysisData
	Warnings        []string
}

// DynamicLoadSymbols returns the list of dynamic load symbols from the symbol analysis.
func (r *Result) DynamicLoadSymbols() []string {
	if r == nil || r.SymbolAnalysis == nil {
		return nil
	}
	return r.SymbolAnalysis.DynamicLoadSymbols
}
