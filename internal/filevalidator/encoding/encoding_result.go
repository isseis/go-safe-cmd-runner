// Package encoding provides hybrid hash filename encoding functionality.
// It implements substitution + double escape encoding with SHA256 fallback
// for long paths that exceed filesystem filename length limits.
package encoding

// Result represents the result of an encoding operation
type Result struct {
	EncodedName    string // The encoded filename
	IsFallback     bool   // Whether SHA256 fallback was used
	OriginalLength int    // Length of original path
	EncodedLength  int    // Length of encoded filename
}

// Analysis provides detailed analysis of encoding process
type Analysis struct {
	OriginalPath   string         // Original file path
	EncodedName    string         // Encoded filename
	IsFallback     bool           // Whether fallback was used
	OriginalLength int            // Length of original path
	EncodedLength  int            // Length of encoded name
	ExpansionRatio float64        // Encoded length / Original length
	CharFrequency  map[rune]int   // Character frequency in original path
	EscapeCount    OperationCount // Number of escape operations
}

// OperationCount tracks escape operation statistics
type OperationCount struct {
	HashEscapes  int // Number of # → #1 operations
	SlashEscapes int // Number of / → ## operations
	TotalEscapes int // Total escape operations
	AddedChars   int // Total characters added by escaping
}
