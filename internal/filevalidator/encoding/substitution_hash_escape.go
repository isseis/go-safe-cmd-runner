package encoding

import (
	"crypto/sha256"
	"encoding/base64"
	"path/filepath"
	"strings"
)

const (
	// MaxFilenameLength defines the maximum allowed filename length.
	//
	// This value is set to 250 characters, which provides a safety margin below
	// the typical NAME_MAX limit of 255 characters on most filesystems.
	// The margin accounts for potential filesystem variations and future extensions.
	//
	// Paths that encode to filenames longer than this limit will automatically
	// use SHA256 fallback encoding.
	MaxFilenameLength = 250

	// HashLength defines the number of characters to use from SHA256 hash.
	//
	// This value determines the length of the hash prefix used in SHA256 fallback
	// encoding. 12 characters provides excellent collision resistance while
	// keeping the total filename length short (17 chars: 12 + ".json").
	//
	// With base64url encoding, 12 characters provide approximately 2^72 unique
	// values, which is sufficient for practical collision avoidance.
	HashLength = 12
)

// SubstitutionHashEscape implements hybrid substitution + double escape encoding for file paths.
//
// This encoder provides:
//   - Space-efficient encoding: 1.00x expansion for typical paths
//   - Reversible encoding: mathematical guarantee for normal encoding
//   - Automatic fallback: SHA256 fallback for paths exceeding NAME_MAX limits
//   - Performance optimization: single-pass encoding and decoding
//
// Encoding Algorithm:
//  1. Substitution: '/' ↔ '~' character mapping
//  2. Double Escape: Escape '#' → '#1' and handle '~' → '##'
//  3. Length Check: Use SHA256 fallback if result exceeds MaxFilenameLength
//
// Example:
//
//	Input:  "/home/user/file.txt"
//	Output: "~home~user~file.txt"
//
// Long Path Example:
//
//	Input:  "/very/long/path/that/exceeds/filename/length/limits/..."
//	Output: "AbCdEf123456.json" (SHA256 fallback)
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
//
// Note: This function does not apply length limits. Use EncodeWithFallback()
// for automatic SHA256 fallback on long paths.
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

// EncodeWithFallback encodes a path with automatic fallback to SHA256 for long paths.
//
// This is the recommended encoding method that provides hybrid functionality:
// normal encoding for typical paths and SHA256 fallback for paths that would
// exceed filesystem filename length limits.
//
// Process:
//  1. Apply normal encoding with full path validation
//  2. Check encoded length against MaxFilenameLength (250 chars)
//  3. Use SHA256 fallback if length exceeds limit
//  4. Return detailed Result with encoding metadata
//
// Fallback Strategy:
//   - Threshold: 250 characters (NAME_MAX - safety margin)
//   - Format: "{12-char-hash}.json" (17 chars total)
//   - Hash: SHA256 with base64url encoding
//   - Always reversible detection via Result.IsFallback
//
// Parameters:
//
//	path: Absolute, normalized file path to encode
//
// Returns:
//
//	Result: Detailed encoding result with metadata
//	error: ErrInvalidPath if path validation fails
//
// Example:
//
//	result, err := encoder.EncodeWithFallback("/home/user/file.txt")
//	if result.IsFallback {
//	    // SHA256 fallback was used
//	}
func (e *SubstitutionHashEscape) EncodeWithFallback(path string) (Result, error) {
	// Try normal encoding (includes all path validation)
	normalEncoded, err := e.Encode(path)
	if err != nil {
		return Result{}, err
	}

	// Check length constraint
	if len(normalEncoded) <= MaxFilenameLength {
		return Result{
			EncodedName:    normalEncoded,
			IsFallback:     false,
			OriginalLength: len(path),
			EncodedLength:  len(normalEncoded),
		}, nil
	}

	// Use SHA256 fallback for long paths (always enabled)
	fallbackEncoded := e.generateSHA256Fallback(path)

	return Result{
		EncodedName:    fallbackEncoded,
		IsFallback:     true,
		OriginalLength: len(path),
		EncodedLength:  len(fallbackEncoded),
	}, nil
}

// generateSHA256Fallback generates SHA256-based filename for long paths.
//
// This fallback mechanism ensures that any path, regardless of length,
// can be represented as a valid filename within filesystem constraints.
//
// Algorithm:
//  1. Generate SHA256 hash of full path string
//  2. Encode hash using base64url (filesystem-safe)
//  3. Truncate to HashLength characters (12 chars)
//  4. Add ".json" extension
//
// Output Format:
//   - Pattern: "{hash}.json"
//   - Hash length: 12 characters
//   - Total length: 17 characters
//   - Character set: [A-Za-z0-9_-] (base64url)
//
// Properties:
//   - Deterministic: same path always produces same hash
//   - Collision-resistant: SHA256 provides cryptographic strength
//   - Filesystem-safe: no special characters, reasonable length
//   - Not reversible: original path cannot be recovered
//
// Performance: ~1ms per path for typical usage
func (e *SubstitutionHashEscape) generateSHA256Fallback(path string) string {
	hash := sha256.Sum256([]byte(path))
	hashStr := base64.URLEncoding.EncodeToString(hash[:])

	// Use default hash length, ensure it fits within limits
	hashLength := min(HashLength, len(hashStr))

	// Format: {hash}.json (hashLength + 5 characters)
	return hashStr[:hashLength] + ".json"
}

// IsNormalEncoding determines if an encoded filename uses normal encoding.
//
// Normal encoding can be distinguished by the first character:
// - Normal encoding always starts with '~' (since all absolute paths start with '/')
// - SHA256 fallback never starts with '~' (uses base64url character set)
//
// This detection is used for:
//   - Choosing appropriate decoding strategy
//   - Migration between encoding formats
//   - Debugging and analysis tools
//
// Parameters:
//
//	encoded: Encoded filename to analyze
//
// Returns:
//
//	bool: true if normal encoding is used, false otherwise
//
// Example:
//
//	isNormal := encoder.IsNormalEncoding("~home~user~file.txt")  // true
//	isNormal := encoder.IsNormalEncoding("AbCdEf123456.json")     // false
func (e *SubstitutionHashEscape) IsNormalEncoding(encoded string) bool {
	if len(encoded) == 0 {
		return false
	}

	// Normal encoding always starts with ~ (since all full paths start with /)
	return encoded[0] == '~'
}

// IsFallbackEncoding determines if an encoded filename uses SHA256 fallback.
//
// This is the logical inverse of IsNormalEncoding(). Any encoded filename
// that doesn't use normal encoding is considered to use SHA256 fallback.
//
// Fallback Detection:
//   - Does not start with '~': indicates SHA256 format
//   - Typical pattern: base64url characters + ".json" extension
//   - Length: usually 17 characters for standard hash length
//
// Usage:
//   - Validation of encoding format expectations
//   - Error handling for decode operations
//   - Statistical analysis of encoding usage
//
// Parameters:
//
//	encoded: Encoded filename to analyze
//
// Returns:
//
//	bool: true if SHA256 fallback is used, false otherwise
//
// Note: Returns false for empty strings.
func (e *SubstitutionHashEscape) IsFallbackEncoding(encoded string) bool {
	if len(encoded) == 0 {
		return false
	}

	return !e.IsNormalEncoding(encoded)
}
