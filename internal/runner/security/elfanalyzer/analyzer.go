package elfanalyzer

import "fmt"

// AnalysisResult represents the result type of ELF network symbol analysis.
type AnalysisResult int

const (
	// NetworkDetected indicates that network-related symbols were found
	// in the .dynsym section. The binary is capable of network operations.
	NetworkDetected AnalysisResult = iota

	// NoNetworkSymbols indicates that no network-related symbols were found.
	// The binary does not appear to use standard network APIs.
	NoNetworkSymbols

	// NotELFBinary indicates that the file is not an ELF binary.
	// This includes scripts, text files, and other non-ELF executables.
	NotELFBinary

	// StaticBinary indicates a statically linked binary with no .dynsym section.
	// Network capability cannot be determined by this analyzer.
	// Falls back to alternative analysis methods (Task 0070).
	StaticBinary

	// AnalysisError indicates that an error occurred during analysis.
	// This should be treated as a potential network operation for safety.
	AnalysisError
)

// String returns a string representation of AnalysisResult.
func (r AnalysisResult) String() string {
	switch r {
	case NetworkDetected:
		return "network_detected"
	case NoNetworkSymbols:
		return "no_network_symbols"
	case NotELFBinary:
		return "not_elf_binary"
	case StaticBinary:
		return "static_binary"
	case AnalysisError:
		return "analysis_error"
	default:
		return fmt.Sprintf("unknown(%d)", int(r))
	}
}

// DetectedSymbol contains information about a detected network symbol.
type DetectedSymbol struct {
	// Name is the symbol name as it appears in .dynsym (e.g., "socket", "curl_easy_init")
	Name string

	// Category is the classification of the symbol (e.g., "socket", "http", "tls")
	Category string
}

// AnalysisOutput contains the complete result of ELF analysis.
type AnalysisOutput struct {
	// Result is the overall analysis result type
	Result AnalysisResult

	// DetectedSymbols contains all network-related symbols found in .dynsym.
	// Only populated when Result == NetworkDetected.
	// Useful for logging and debugging purposes.
	DetectedSymbols []DetectedSymbol

	// Error contains the error details when Result == AnalysisError.
	// May also be set for other result types to provide diagnostic context
	// (e.g., NotELFBinary when the file is not a regular file).
	Error error
}

// IsNetworkCapable returns true if the analysis indicates the binary
// might perform network operations. This includes both detected network symbols
// and analysis errors (treated as potential network operations for safety).
func (o AnalysisOutput) IsNetworkCapable() bool {
	return o.Result == NetworkDetected || o.Result == AnalysisError
}

// IsIndeterminate returns true if the analysis could not determine
// network capability (error or static binary).
func (o AnalysisOutput) IsIndeterminate() bool {
	return o.Result == AnalysisError || o.Result == StaticBinary
}

// ELFAnalyzer defines the interface for ELF binary network analysis.
type ELFAnalyzer interface {
	// AnalyzeNetworkSymbols examines the ELF binary at the given path
	// and determines if it contains network-related symbols in .dynsym.
	//
	// The path must be an absolute path to an executable file.
	//
	// Returns:
	//   - NetworkDetected: Binary contains network symbols
	//   - NoNetworkSymbols: Binary has .dynsym but no network symbols
	//   - NotELFBinary: File is not an ELF binary
	//   - StaticBinary: Binary is statically linked (no .dynsym)
	//   - AnalysisError: An error occurred (check Error field)
	AnalyzeNetworkSymbols(path string) AnalysisOutput
}
