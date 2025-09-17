package encoding

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

const (
	// DefaultMaxFilenameLength defines the maximum allowed filename length (NAME_MAX - safety margin)
	DefaultMaxFilenameLength = 250
	// DefaultHashLength defines the number of characters to use from SHA256 hash
	DefaultHashLength = 12
)

// SubstitutionHashEscape implements hybrid substitution + double escape encoding
type SubstitutionHashEscape struct {
	// MaxFilenameLength defines the maximum allowed filename length
	MaxFilenameLength int

	// HashLength defines the number of characters to use from SHA256 hash
	HashLength int
}

// NewSubstitutionHashEscape creates a new encoder with default settings
func NewSubstitutionHashEscape() *SubstitutionHashEscape {
	return &SubstitutionHashEscape{
		MaxFilenameLength: DefaultMaxFilenameLength,
		HashLength:        DefaultHashLength,
	}
}

// Encode encodes a file path using substitution + double escape method
// Returns the encoded filename (without directory path)
func (e *SubstitutionHashEscape) Encode(path string) string {
	if path == "" {
		return ""
	}

	// Step 1: Substitution (/ ↔ ~)
	substituted := e.substitute(path)

	// Step 2: Double escape (# → #1, / → ##)
	escaped := e.doubleEscape(substituted)

	return escaped
}

// substitute performs character substitution (/ ↔ ~)
func (e *SubstitutionHashEscape) substitute(path string) string {
	var builder strings.Builder
	builder.Grow(len(path))

	for _, char := range path {
		switch char {
		case '/':
			builder.WriteRune('~')
		case '~':
			builder.WriteRune('/')
		default:
			builder.WriteRune(char)
		}
	}

	return builder.String()
}

// doubleEscape performs meta-character double escaping
func (e *SubstitutionHashEscape) doubleEscape(substituted string) string {
	// Replace # → #1 first to avoid interference
	escaped := strings.ReplaceAll(substituted, "#", "#1")
	// Replace / → ##
	escaped = strings.ReplaceAll(escaped, "/", "##")

	return escaped
}

// Decode decodes an encoded filename back to original file path
func (e *SubstitutionHashEscape) Decode(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}

	// Check if this is a fallback format (not start with ~)
	if len(encoded) > 0 && encoded[0] != '~' {
		return "", ErrFallbackNotReversible{EncodedName: encoded}
	}

	// Step 1: Reverse double escape (## → /, #1 → #)
	decoded := strings.ReplaceAll(encoded, "##", "/")
	decoded = strings.ReplaceAll(decoded, "#1", "#")

	// Step 2: Reverse substitution (/ ↔ ~)
	result := e.reverseSubstitute(decoded)

	return result, nil
}

// reverseSubstitute reverses the character substitution
func (e *SubstitutionHashEscape) reverseSubstitute(decoded string) string {
	var builder strings.Builder
	builder.Grow(len(decoded))

	for _, char := range decoded {
		switch char {
		case '/':
			builder.WriteRune('~')
		case '~':
			builder.WriteRune('/')
		default:
			builder.WriteRune(char)
		}
	}

	return builder.String()
}

// EncodeWithFallback encodes with automatic fallback to SHA256 for long paths
func (e *SubstitutionHashEscape) EncodeWithFallback(path string) Result {
	if path == "" {
		return Result{
			EncodedName:    "",
			IsFallback:     false,
			OriginalLength: 0,
			EncodedLength:  0,
		}
	}

	// Try normal encoding first
	normalEncoded := e.Encode(path)

	// Check length constraint
	if len(normalEncoded) <= e.MaxFilenameLength {
		return Result{
			EncodedName:    normalEncoded,
			IsFallback:     false,
			OriginalLength: len(path),
			EncodedLength:  len(normalEncoded),
		}
	}

	// Use SHA256 fallback for long paths (always enabled)
	fallbackEncoded := e.generateSHA256Fallback(path)

	return Result{
		EncodedName:    fallbackEncoded,
		IsFallback:     true,
		OriginalLength: len(path),
		EncodedLength:  len(fallbackEncoded),
	}
}

// generateSHA256Fallback generates SHA256-based filename for long paths
func (e *SubstitutionHashEscape) generateSHA256Fallback(path string) string {
	hash := sha256.Sum256([]byte(path))
	hashStr := base64.URLEncoding.EncodeToString(hash[:])

	// Use configured hash length, ensure it fits within limits
	hashLength := e.HashLength
	if hashLength > len(hashStr) {
		hashLength = len(hashStr)
	}

	// Format: {hash}.json (hashLength + 5 characters)
	return hashStr[:hashLength] + ".json"
}

// AnalyzeEncoding provides detailed analysis of encoding process
func (e *SubstitutionHashEscape) AnalyzeEncoding(path string) Analysis {
	result := e.EncodeWithFallback(path)

	// Calculate expansion ratio safely (avoid division by zero)
	var expansionRatio float64
	if result.OriginalLength > 0 {
		expansionRatio = float64(result.EncodedLength) / float64(result.OriginalLength)
	} else {
		expansionRatio = 0.0 // or 1.0 depending on desired semantics
	}

	analysis := Analysis{
		OriginalPath:   path,
		EncodedName:    result.EncodedName,
		IsFallback:     result.IsFallback,
		OriginalLength: result.OriginalLength,
		EncodedLength:  result.EncodedLength,
		ExpansionRatio: expansionRatio,
	}

	if !result.IsFallback {
		// Analyze character frequency for normal encoding
		analysis.CharFrequency = e.analyzeCharFrequency(path)
		analysis.EscapeCount = e.countEscapeOperations(path)
	}

	return analysis
}

// analyzeCharFrequency counts character frequency in original path
func (e *SubstitutionHashEscape) analyzeCharFrequency(path string) map[rune]int {
	frequency := make(map[rune]int)

	for _, char := range path {
		frequency[char]++
	}

	return frequency
}

// countEscapeOperations counts the number of escape operations needed
func (e *SubstitutionHashEscape) countEscapeOperations(path string) OperationCount {
	substituted := e.substitute(path)

	hashCount := strings.Count(substituted, "#")
	slashCount := strings.Count(substituted, "/")

	return OperationCount{
		HashEscapes:  hashCount,              // # → #1
		SlashEscapes: slashCount,             // / → ##
		TotalEscapes: hashCount + slashCount, // Total escape operations
		AddedChars:   hashCount + slashCount, // Each escape adds 1 character
	}
}

// IsNormalEncoding determines if an encoded filename uses normal encoding
func (e *SubstitutionHashEscape) IsNormalEncoding(encoded string) bool {
	if len(encoded) == 0 {
		return false
	}

	// Normal encoding always starts with ~ (since all full paths start with /)
	return encoded[0] == '~'
}

// IsFallbackEncoding determines if an encoded filename uses SHA256 fallback
func (e *SubstitutionHashEscape) IsFallbackEncoding(encoded string) bool {
	if len(encoded) == 0 {
		return false
	}

	return !e.IsNormalEncoding(encoded)
}
