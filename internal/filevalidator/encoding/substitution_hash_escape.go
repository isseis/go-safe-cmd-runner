package encoding

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

const (
	// maxPathLength defines the maximum length for encoded paths
	maxPathLength = 255
	// fallbackPrefix is used to identify SHA256 fallback encodings
	fallbackPrefix = "sha256_"
)

// substitutionMap defines the character substitutions for encoding
var substitutionMap = map[rune]string{
	'/':  "_slash_",
	'\\': "_backslash_",
	':':  "_colon_",
	'*':  "_asterisk_",
	'?':  "_question_",
	'"':  "_quote_",
	'<':  "_lt_",
	'>':  "_gt_",
	'|':  "_pipe_",
	' ':  "_space_",
}

// reverseSubstitutionMap is the reverse mapping for decoding
var reverseSubstitutionMap map[string]rune

func init() {
	reverseSubstitutionMap = make(map[string]rune)
	for char, replacement := range substitutionMap {
		reverseSubstitutionMap[replacement] = char
	}
}

// Encode encodes a file path using substitution encoding
func Encode(path string) (string, error) {
	if path == "" {
		return "", ErrEmptyPath
	}

	encoded := substitute(path)
	encoded = doubleEscape(encoded)

	if len(encoded) > maxPathLength {
		return "", ErrPathTooLong
	}

	return encoded, nil
}

// EncodeWithFallback encodes a file path, falling back to SHA256 if necessary
func EncodeWithFallback(path string) Result {
	if path == "" {
		return Result{
			EncodedName:    "",
			IsFallback:     false,
			OriginalLength: 0,
			EncodedLength:  0,
		}
	}

	// Try normal encoding first
	encoded, err := Encode(path)
	if err == nil {
		return Result{
			EncodedName:    encoded,
			IsFallback:     false,
			OriginalLength: len(path),
			EncodedLength:  len(encoded),
		}
	}

	// Fall back to SHA256 if normal encoding fails
	fallback := generateSHA256Fallback(path)
	return Result{
		EncodedName:    fallback,
		IsFallback:     true,
		OriginalLength: len(path),
		EncodedLength:  len(fallback),
	}
}

// Decode decodes an encoded file path back to the original
func Decode(encoded string) (string, error) {
	if encoded == "" {
		return "", ErrEmptyPath
	}

	// Check if it's a fallback encoding
	if strings.HasPrefix(encoded, fallbackPrefix) {
		return "", ErrFallbackNotReversible
	}

	// Reverse the double escape
	unescaped := reverseDoubleEscape(encoded)

	// Reverse the substitution
	original := reverseSubstitute(unescaped)

	return original, nil
}

// substitute replaces problematic characters with safe substitutes
func substitute(path string) string {
	result := path
	for char, replacement := range substitutionMap {
		result = strings.ReplaceAll(result, string(char), replacement)
	}
	return result
}

// reverseSubstitute reverses the character substitution
func reverseSubstitute(encoded string) string {
	result := encoded
	for replacement, char := range reverseSubstitutionMap {
		result = strings.ReplaceAll(result, replacement, string(char))
	}
	return result
}

// doubleEscape escapes underscore characters to prevent conflicts
func doubleEscape(s string) string {
	// Replace existing underscores with double underscores
	return strings.ReplaceAll(s, "_", "__")
}

// reverseDoubleEscape reverses the double escape operation
func reverseDoubleEscape(s string) string {
	// Replace double underscores back to single underscores
	return strings.ReplaceAll(s, "__", "_")
}

// generateSHA256Fallback creates a SHA256-based fallback encoding
func generateSHA256Fallback(path string) string {
	hash := sha256.Sum256([]byte(path))
	return fallbackPrefix + fmt.Sprintf("%x", hash)
}

// IsNormalEncoding checks if the encoded name uses normal encoding
func IsNormalEncoding(encoded string) bool {
	return !strings.HasPrefix(encoded, fallbackPrefix)
}

// IsFallbackEncoding checks if the encoded name uses SHA256 fallback
func IsFallbackEncoding(encoded string) bool {
	return strings.HasPrefix(encoded, fallbackPrefix)
}
