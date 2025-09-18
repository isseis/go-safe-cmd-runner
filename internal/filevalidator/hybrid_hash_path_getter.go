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
//   - "/home/user/file.txt" → "~home~user~file.txt.json"
//   - "/very/long/path/..." → "AbCdEf123456.json" (SHA256 fallback)
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
//  1. Attempt normal substitution+escape encoding
//  2. If result exceeds NAME_MAX limits, use SHA256 fallback
//  3. Add .json extension and combine with hash directory
//
// Parameters:
//   - hashAlgorithm: The hash algorithm (required, used for validation)
//   - hashDir: Directory where hash files are stored
//   - filePath: The file path to generate hash file path for
//
// Returns:
//   - Full path to the hash file
//   - Error if encoding fails or parameters are invalid
func (h *HybridHashFilePathGetter) GetHashFilePath(
	hashAlgorithm HashAlgorithm,
	hashDir string,
	filePath common.ResolvedPath,
) (string, error) {
	// Validate required parameters
	if hashAlgorithm == nil {
		return "", ErrNilAlgorithm
	}

	if hashDir == "" {
		return "", ErrEmptyHashDir
	}

	// Encode the file path using hybrid strategy
	result, err := h.encoder.EncodeWithFallback(filePath.String())
	if err != nil {
		return "", fmt.Errorf("failed to encode path %q: %w", filePath.String(), err)
	}

	// Create the hash filename with .json extension
	hashFilename := result.EncodedName + ".json"

	// Combine with hash directory
	fullPath := filepath.Join(hashDir, hashFilename)

	return fullPath, nil
}
