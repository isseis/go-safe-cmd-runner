package binaryanalyzer

import "fmt"

// AnalysisResult represents the result type of binary network symbol analysis.
type AnalysisResult int

const (
	// NetworkDetected indicates that network-related symbols were found
	// in the binary. The binary is capable of network operations.
	NetworkDetected AnalysisResult = iota

	// NoNetworkSymbols indicates that no network-related symbols were found.
	// The binary does not appear to use standard network APIs.
	NoNetworkSymbols

	// NotSupportedBinary indicates that the file format is not supported
	// by this analyzer (e.g., ELF analyzer receiving a Mach-O file,
	// or Mach-O analyzer receiving an ELF file).
	NotSupportedBinary

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
	case NotSupportedBinary:
		return "not_supported_binary"
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
	// Name is the symbol name as it appears in the binary (e.g., "socket", "curl_easy_init")
	Name string

	// Category is the classification of the symbol (e.g., "socket", "http", "tls")
	Category string
}

// AnalysisOutput contains the complete result of binary analysis.
type AnalysisOutput struct {
	// Result is the overall analysis result type
	Result AnalysisResult

	// DetectedSymbols contains all network-related symbols found.
	// Only populated when Result == NetworkDetected.
	// Useful for logging and debugging purposes.
	DetectedSymbols []DetectedSymbol

	// DynamicLoadSymbols contains the dynamic library loading symbols found
	// (dlopen, dlsym, or dlvsym). This is set independently of Result
	// and network symbol detection. Use len(DynamicLoadSymbols) > 0 to
	// determine if any dynamic load symbols were detected.
	DynamicLoadSymbols []DetectedSymbol

	// Error contains the error details when Result == AnalysisError.
	// May also be set for other result types to provide diagnostic context
	// (e.g., NotSupportedBinary when the target is not a regular file).
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

// BinaryAnalyzer defines the interface for binary network analysis,
// independent of the binary format (ELF, Mach-O, etc.).
type BinaryAnalyzer interface {
	// AnalyzeNetworkSymbols examines the binary at the given path
	// and determines if it contains network-related symbols.
	//
	// contentHash is the pre-computed hash in "algo:hex" format (e.g. "sha256:abc123...").
	// Must be non-empty; callers that cannot provide a hash must skip binary analysis entirely.
	//
	// Returns:
	//   - NetworkDetected: Binary contains network-related symbols
	//   - NoNetworkSymbols: Binary has no network-related symbols
	//   - NotSupportedBinary: File format is not supported by this analyzer
	//   - StaticBinary: Binary is statically linked (ELF-specific)
	//   - AnalysisError: An error occurred (check Error field)
	AnalyzeNetworkSymbols(path string, contentHash string) AnalysisOutput
}
