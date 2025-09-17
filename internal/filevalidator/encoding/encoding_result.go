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
