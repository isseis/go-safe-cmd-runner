package encoding

import (
	"path/filepath"
	"strings"
)

// SubstitutionHashEscape implements substitution + double escape encoding for file paths.
//
// This encoder provides:
//   - Space-efficient encoding: 1.00x expansion for typical paths
//   - Reversible encoding: mathematical guarantee
//   - Performance optimization: single-pass encoding and decoding
//
// Encoding Algorithm:
//  1. Substitution: '/' ↔ '~' character mapping
//  2. Double Escape: Escape '#' → '#1' and handle '~' → '##'
//
// Example:
//
//	Input:  "/home/user/file.txt"
//	Output: "~home~user~file.txt"
type SubstitutionHashEscape struct{}

// NewSubstitutionHashEscape creates a new SubstitutionHashEscape encoder.
//
// Returns a configured encoder ready for use. The encoder is stateless
// and safe for concurrent use.
//
// Example:
//
//	encoder := NewSubstitutionHashEscape()
//	encoded, err := encoder.Encode("/home/user/file.txt")
func NewSubstitutionHashEscape() *SubstitutionHashEscape {
	return &SubstitutionHashEscape{}
}

// Encode encodes a file path using substitution + double escape method.
//
// The input path must be an absolute, normalized path (filepath.Clean applied).
// This function validates the input and applies the core encoding algorithm.
// This method always uses normal encoding and does not apply length limits
// or fallback strategies.
//
// Encoding Process:
//  1. Input validation: checks for empty path, absolute path, normalized path
//  2. Single-pass encoding: combines substitution and escaping for efficiency
//  3. Character transformation:
//     - '/' → '~' (substitution)
//     - '~' → '##' (combined substitution + escape)
//     - '#' → '#1' (escape only)
//     - other characters → unchanged
//
// Parameters:
//
//	path: Absolute, normalized file path to encode
//
// Returns:
//
//	string: Encoded filename suitable for filesystem use
//	error: ErrInvalidPath if path validation fails
//
// Example:
//
//	encoded, err := encoder.Encode("/home/user/file.txt")
//	// Result: "~home~user~file.txt"
func (e *SubstitutionHashEscape) Encode(path string) (string, error) {
	if path == "" {
		return "", ErrInvalidPath{Path: path, Err: ErrEmptyPath}
	}

	// Ensure path is absolute and canonical
	if !filepath.IsAbs(path) {
		return "", ErrInvalidPath{Path: path, Err: ErrNotAbsoluteOrNormalized}
	}
	if filepath.Clean(path) != path {
		return "", ErrInvalidPath{Path: path, Err: ErrNotAbsoluteOrNormalized}
	}
	// Single-pass encoding optimization
	return e.encodeOptimized(path), nil
}

// encodeOptimized performs single-pass encoding combining substitution and double escape.
//
// This is the core encoding implementation optimized for performance.
// It uses strings.Builder with pre-allocated capacity to minimize allocations.
//
// Algorithm:
//   - '/' → '~': Direct substitution (step 1)
//   - '~' → '##': Combined substitution (~ → /) + double escape (/ → ##)
//   - '#' → '#1': Double escape only
//   - Other chars: No transformation
//
// Performance:
//   - Single pass through input string
//   - Pre-allocated buffer with 10% growth estimate
//   - Minimal memory allocations
//
// Time Complexity: O(n) where n is input length
// Space Complexity: O(n) for output buffer
func (e *SubstitutionHashEscape) encodeOptimized(path string) string {
	var builder strings.Builder
	// Pre-allocate: typical expansion is minimal, but allow for some escaping
	// Most characters (especially /) don't expand, only ~ and # expand to 2 chars
	builder.Grow(len(path) + len(path)/10) // +10% buffer for typical cases

	for _, char := range path {
		switch char {
		case '/':
			// Substitution: / → ~, then double escape: ~ → (unchanged), then / would be ##
			// Combined: / → ~ (substitution step result has no / to double escape)
			builder.WriteRune('~')
		case '~':
			// Substitution: ~ → /, then double escape: / → ##
			// Combined: ~ → ##
			builder.WriteString("##")
		case '#':
			// No substitution: # → #, then double escape: # → #1
			// Combined: # → #1
			builder.WriteString("#1")
		default:
			// No substitution or escaping needed
			builder.WriteRune(char)
		}
	}

	return builder.String()
}

// Decode decodes an encoded filename back to the original absolute file path.
//
// This function reverses the encoding process to recover the original path.
// Only paths encoded with normal encoding (starting with '~') can be decoded.
// SHA256 fallback encodings cannot be reversed.
//
// Decoding Process:
//  1. Fallback detection: Check if encoded name uses SHA256 format
//  2. Reverse double escape: '##' → '~', '#1' → '#'
//  3. Reverse substitution: '~' → '/'
//
// Parameters:
//
//	encoded: Encoded filename to decode
//
// Returns:
//
//	string: Original absolute file path
//	error: ErrFallbackNotReversible if trying to decode SHA256 fallback
//
// Example:
//
//	original, err := encoder.Decode("~home~user~file.txt")
//	// Result: "/home/user/file.txt"
//
// Note: Empty input returns empty string without error.
func (e *SubstitutionHashEscape) Decode(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}

	// Check if this is a fallback format (not start with ~)
	if len(encoded) > 0 && encoded[0] != '~' {
		return "", ErrFallbackNotReversible{EncodedName: encoded}
	}

	// Single-pass decoding optimization
	result := e.decodeOptimized(encoded)

	return result, nil
}

// decodeOptimized performs single-pass decoding combining reverse double escape and substitution.
//
// This is the core decoding implementation optimized for performance.
// It processes escape sequences and substitutions in a single pass.
//
// Algorithm:
//   - '##' → '~': Combined reverse double escape (## → /) + reverse substitution (/ → ~)
//   - '#1' → '#': Reverse double escape only
//   - '~' → '/': Reverse substitution only
//   - Other chars: No transformation
//
// Performance:
//   - Single pass through input string
//   - Pre-allocated buffer sized to input length
//   - Handles multi-character escape sequences efficiently
//
// Time Complexity: O(n) where n is input length
// Space Complexity: O(n) for output buffer
func (e *SubstitutionHashEscape) decodeOptimized(encoded string) string {
	var builder strings.Builder
	// Pre-allocate: decoded result is always <= encoded length
	// (## → ~, #1 → #, ~ → /, / → ~, others unchanged)
	builder.Grow(len(encoded))

	runes := []rune(encoded)
	for i := 0; i < len(runes); i++ {
		char := runes[i]

		switch char {
		case '#':
			// Check for escape sequences: ## → / or #1 → #
			if i+1 < len(runes) {
				next := runes[i+1]
				switch next {
				case '#':
					// ## → / (reverse double escape), then / → ~ (reverse substitution)
					// Combined: ## → ~
					builder.WriteRune('~')
					i++ // Skip next character
				case '1':
					// #1 → # (reverse double escape), then # → # (no substitution)
					// Combined: #1 → #
					builder.WriteRune('#')
					i++ // Skip next character
				default:
					// Single # without escape sequence (shouldn't happen in valid encoding)
					builder.WriteRune(char)
				}
			} else {
				// Single # at end (shouldn't happen in valid encoding)
				builder.WriteRune(char)
			}
		case '~':
			// ~ → / (reverse substitution)
			builder.WriteRune('/')
		default:
			// No decoding needed
			builder.WriteRune(char)
		}
	}

	return builder.String()
}
