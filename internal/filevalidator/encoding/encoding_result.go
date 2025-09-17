// Package encoding provides hybrid hash filename encoding functionality.
//
// This package implements a space-efficient, reversible encoding system for
// converting file paths to filesystem-safe filenames. It combines substitution
// encoding with automatic SHA256 fallback for long paths.
//
// Key Features:
//   - Space Efficiency: 1.00x expansion ratio for typical paths
//   - Reversibility: Mathematical guarantee for normal encoding
//   - Automatic Fallback: SHA256 for paths exceeding NAME_MAX limits
//   - Performance: Single-pass encoding/decoding algorithms
//   - Safety: Comprehensive input validation and error handling
//
// Primary Use Case:
// Converting absolute file paths to hash filenames for integrity verification
// in the go-safe-cmd-runner project.
//
// Example Usage:
//
//	encoder := NewSubstitutionHashEscape()
//	result, err := encoder.EncodeWithFallback("/home/user/file.txt")
//	if err != nil {
//	    // Handle error
//	}
//	if result.IsFallback {
//	    // SHA256 fallback was used
//	}
package encoding

// Result represents the result of an encoding operation with detailed metadata.
//
// This structure provides comprehensive information about the encoding process,
// enabling callers to make informed decisions about fallback handling,
// performance monitoring, and debugging.
//
// Fields:
//
//	EncodedName: The final encoded filename ready for filesystem use
//	IsFallback: Whether SHA256 fallback was used (affects reversibility)
//	OriginalLength: Length of input path (for expansion ratio calculation)
//	EncodedLength: Length of output filename (for length limit validation)
//
// Usage Patterns:
//   - Check IsFallback to determine if decoding is possible
//   - Calculate expansion ratio: float64(EncodedLength) / float64(OriginalLength)
//   - Monitor fallback usage for performance optimization
//   - Validate length constraints for specific filesystems
//
// Example:
//
//	result, _ := encoder.EncodeWithFallback("/long/path")
//	expansionRatio := float64(result.EncodedLength) / float64(result.OriginalLength)
//	if expansionRatio > 1.1 {
//	    // Unusually high expansion detected
//	}
type Result struct {
	EncodedName    string // The encoded filename ready for use as a filesystem filename
	IsFallback     bool   // Whether SHA256 fallback was used (true means original path cannot be recovered)
	OriginalLength int    // Length of original path in bytes (UTF-8 encoding)
	EncodedLength  int    // Length of encoded filename in bytes (UTF-8 encoding)
}
