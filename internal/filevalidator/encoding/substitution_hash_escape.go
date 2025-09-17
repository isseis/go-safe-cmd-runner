package encoding

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
)

const (
	// MaxFilenameLength defines the maximum allowed filename length (NAME_MAX - safety margin)
	MaxFilenameLength = 250
	// HashLength defines the number of characters to use from SHA256 hash
	HashLength = 12
)

// ErrInvalidPath represents an error for invalid file paths during encoding operations
type ErrInvalidPath struct {
	Path string // The invalid path
	Err  error  // The underlying error, if any
}

func (e ErrInvalidPath) Error() string {
	return fmt.Sprintf("invalid path: %s (error: %v)", e.Path, e.Err)
}

func (e *ErrInvalidPath) Unwrap() error {
	return e.Err
}

// SubstitutionHashEscape implements hybrid substitution + double escape encoding
type SubstitutionHashEscape struct{}

// NewSubstitutionHashEscape creates a new encoder
func NewSubstitutionHashEscape() *SubstitutionHashEscape {
	return &SubstitutionHashEscape{}
}

// Encode encodes a file path using substitution + double escape method.
// The path will be converted to an absolute, normalized path.
// Returns the encoded filename (without directory path).
func (e *SubstitutionHashEscape) Encode(path string) (string, error) {
	if path == "" {
		return "", ErrInvalidPath{Path: path, Err: ErrEmptyPath}
	}

	// Convert to absolute and normalized path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", ErrInvalidPath{Path: path, Err: err}
	}
	if absPath != path {
		return "", ErrInvalidPath{Path: path, Err: ErrNotAbsoluteOrNormalized}
	}

	// Single-pass encoding optimization
	encoded := e.encodeOptimized(absPath)

	return encoded, nil
}

// encodeOptimized performs single-pass encoding combining substitution and double escape
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

// Decode decodes an encoded filename back to original absolute file path.
// Only absolute paths are supported as inputs during encoding.
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

// decodeOptimized performs single-pass decoding combining reverse double escape and substitution
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
		case '/':
			// / → ~ (reverse substitution)
			builder.WriteRune('~')
		default:
			// No decoding needed
			builder.WriteRune(char)
		}
	}

	return builder.String()
}

// EncodeWithFallback encodes a path with automatic fallback to SHA256 for long paths.
// The path will be converted to an absolute, normalized path.
func (e *SubstitutionHashEscape) EncodeWithFallback(path string) Result {
	if path == "" {
		return Result{
			EncodedName:    "",
			IsFallback:     false,
			OriginalLength: 0,
			EncodedLength:  0,
		}
	}

	// Convert to absolute path first for consistent path handling
	absPath, err := filepath.Abs(path)
	if err != nil {
		// If path conversion fails, use SHA256 fallback
		fallbackEncoded := e.generateSHA256Fallback(path)
		return Result{
			EncodedName:    fallbackEncoded,
			IsFallback:     true,
			OriginalLength: len(path),
			EncodedLength:  len(fallbackEncoded),
		}
	}

	// Try normal encoding
	normalEncoded, err := e.Encode(absPath)
	if err != nil {
		// If encoding fails, use SHA256 fallback
		fallbackEncoded := e.generateSHA256Fallback(absPath)
		return Result{
			EncodedName:    fallbackEncoded,
			IsFallback:     true,
			OriginalLength: len(absPath),
			EncodedLength:  len(fallbackEncoded),
		}
	}

	// Check length constraint
	if len(normalEncoded) <= MaxFilenameLength {
		return Result{
			EncodedName:    normalEncoded,
			IsFallback:     false,
			OriginalLength: len(absPath),
			EncodedLength:  len(normalEncoded),
		}
	}

	// Use SHA256 fallback for long paths (always enabled)
	fallbackEncoded := e.generateSHA256Fallback(absPath)

	return Result{
		EncodedName:    fallbackEncoded,
		IsFallback:     true,
		OriginalLength: len(absPath),
		EncodedLength:  len(fallbackEncoded),
	}
}

// generateSHA256Fallback generates SHA256-based filename for long paths
func (e *SubstitutionHashEscape) generateSHA256Fallback(path string) string {
	hash := sha256.Sum256([]byte(path))
	hashStr := base64.URLEncoding.EncodeToString(hash[:])

	// Use default hash length, ensure it fits within limits
	hashLength := min(HashLength, len(hashStr))

	// Format: {hash}.json (hashLength + 5 characters)
	return hashStr[:hashLength] + ".json"
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
