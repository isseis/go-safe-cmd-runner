// Package encoding provides file path encoding utilities for safe filename generation.
package encoding

// Result represents the result of encoding a file path
type Result struct {
	// EncodedName is the encoded filename
	EncodedName string
	// IsFallback indicates if SHA256 fallback was used
	IsFallback bool
	// OriginalLength is the length of the original path
	OriginalLength int
	// EncodedLength is the length of the encoded path
	EncodedLength int
}

// IsNormalEncoding returns true if normal encoding was used (not fallback)
func (r Result) IsNormalEncoding() bool {
	return !r.IsFallback
}

// IsFallbackEncoding returns true if SHA256 fallback was used
func (r Result) IsFallbackEncoding() bool {
	return r.IsFallback
}
