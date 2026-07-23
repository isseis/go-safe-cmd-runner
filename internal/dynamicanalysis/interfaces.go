package dynamicanalysis

import "github.com/isseis/go-safe-cmd-runner/internal/safefileio"

// Store provides storage and retrieval for dynamic library analysis results,
// keyed by library path and hash.
type Store interface {
	// LoadOrAnalyzeAndStore retrieves existing analysis for the given library.
	// If no valid analysis is found (not found, hash mismatch, schema mismatch,
	// or parse error), it runs a fresh analysis, persists the result, and returns it.
	// This method is intended for use by the record command.
	LoadOrAnalyzeAndStore(libPath, libHash string) (*Result, error)

	// LoadAnalysis retrieves stored analysis for the given library.
	// Returns ErrAnalysisNotFound if no valid analysis exists
	// (not found, hash mismatch, schema mismatch, or parse error).
	// This method is intended for use by the runner command.
	LoadAnalysis(libPath, libHash string) (*Result, error)
}

// Analyzer performs the actual analysis of a dynamic library file.
// Implementations are provided by callers (e.g., filevalidator.Validator).
type Analyzer interface {
	// AnalyzeLibrary analyzes the library already open as file (an fd opened
	// on libPath by the caller). Implementations must read file's content
	// rather than reopening libPath themselves: the caller (LoadOrAnalyzeAndStore)
	// verifies file's actual content hash against the caller-supplied hash key
	// before calling this method, and that guarantee only holds if analysis
	// reads the exact same fd rather than a fresh, potentially different, open.
	AnalyzeLibrary(file safefileio.File, libPath string) (*Result, error)
}
