package filevalidator

import (
	"fmt"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator/encoding"
)

const (
	// MaxFilenameLength defines the maximum allowed filename length.
	// This value is set to 250 characters, which provides a safety margin below
	// the typical NAME_MAX limit of 255 characters on most filesystems.
	MaxFilenameLength = 250
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
//  2. Fallback: Use SHA256PathHashGetter when encoded length exceeds limits
//
// Examples:
//   - "/home/user/file.txt" → "~home~user~file.txt" (normal encoding, no extension)
//   - "/very/long/path/..." → "AbCdEf123456.json" (SHA256 fallback with .json extension)
type HybridHashFilePathGetter struct {
	encoder        *encoding.SubstitutionHashEscape
	fallbackGetter *SHA256PathHashGetter
}

// NewHybridHashFilePathGetter creates a new HybridHashFilePathGetter instance.
func NewHybridHashFilePathGetter() *HybridHashFilePathGetter {
	return &HybridHashFilePathGetter{
		encoder:        encoding.NewSubstitutionHashEscape(),
		fallbackGetter: NewSHA256PathHashGetter(),
	}
}

// GetHashFilePath returns the path where the given file's hash would be stored.
//
// This implementation uses hybrid encoding:
//  1. Attempt normal substitution+escape encoding (no extension)
//  2. If result exceeds NAME_MAX limits, delegate to SHA256PathHashGetter
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

	// Try normal encoding first
	encodedName, err := h.encoder.Encode(filePath.String())
	if err != nil {
		return "", fmt.Errorf("failed to encode path %q: %w", filePath.String(), err)
	}

	// Check if encoded name exceeds length limit
	if len(encodedName) <= MaxFilenameLength {
		// Use normal encoding
		return filepath.Join(hashDir, encodedName), nil
	}

	// Use SHA256 fallback via SHA256PathHashGetter
	return h.fallbackGetter.GetHashFilePath(hashDir, filePath)
}
