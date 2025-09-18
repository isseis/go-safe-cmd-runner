package filevalidator

import (
	"fmt"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator/encoding"
)

// HybridHashFilePathGetter implements HashFilePathGetter using hybrid encoding strategy.
//
// This implementation provides:
//   - Efficient encoding for typical paths (1.00x expansion ratio)
//   - Mathematical reversibility for normal encoding
//   - Automatic SHA256 fallback for paths exceeding NAME_MAX limits
//   - Full compatibility with the HashFilePathGetter interface
//
// Encoding strategy:
//  1. Primary: Use SubstitutionHashEscape with ~path format
//  2. Fallback: Use SHA256 hash when encoded length exceeds limits
//
// Examples:
//   - "/home/user/file.txt" → "~home~user~file.txt" (normal encoding, no extension)
//   - "/very/long/path/..." → "AbCdEf123456.json" (SHA256 fallback with .json extension)
type HybridHashFilePathGetter struct {
	encoder *encoding.SubstitutionHashEscape
}

// NewHybridHashFilePathGetter creates a new HybridHashFilePathGetter instance.
func NewHybridHashFilePathGetter() *HybridHashFilePathGetter {
	return &HybridHashFilePathGetter{
		encoder: &encoding.SubstitutionHashEscape{},
	}
}

// GetHashFilePath returns the path where the given file's hash would be stored.
//
// This implementation uses hybrid encoding:
//  1. Attempt normal substitution+escape encoding (no extension)
//  2. If result exceeds NAME_MAX limits, use SHA256 fallback (.json extension included)
//  3. Combine with hash directory
//
// Parameters:
//   - hashDir: Directory where hash files are stored
//   - filePath: The file path to generate hash file path for
//
// Returns:
//   - Full path to the hash file
//   - Error if encoding fails or parameters are invalid
func (h *HybridHashFilePathGetter) GetHashFilePath(hashDir string, filePath common.ResolvedPath) (string, error) {
	if hashDir == "" {
		return "", ErrEmptyHashDir
	}

	// Encode the file path using hybrid strategy
	result, err := h.encoder.EncodeWithFallback(filePath.String())
	if err != nil {
		return "", fmt.Errorf("failed to encode path %q: %w", filePath.String(), err)
	}

	return filepath.Join(hashDir, result.EncodedName), nil
}
