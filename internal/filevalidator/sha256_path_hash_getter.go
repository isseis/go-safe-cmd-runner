package filevalidator

import (
	"crypto/sha256"
	"encoding/base64"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// SHA256PathHashGetter is a concrete implementation of HashFilePathGetter.
//
// This implementation uses SHA256 hash-based file naming for compatibility with
// existing hash file storage. It generates deterministic file paths using:
//   - SHA256 hash of the full file path
//   - Base64URL encoding (filesystem-safe)
//   - 12-character truncation with .json extension
//
// Examples:
//   - "/home/user/file.txt" → "AbCdEf123456.json"
//   - "/var/log/system.log" → "XyZwVu789012.json"
//
// This is the legacy implementation maintained for backward compatibility
// with existing hash files and systems expecting this format.
type SHA256PathHashGetter struct{}

// NewSHA256PathHashGetter creates a new SHA256PathHashGetter instance.
//
// This constructor ensures consistent initialization and provides a clear
// creation pattern matching other HashFilePathGetter implementations.
//
// Returns:
//   - *SHA256PathHashGetter: Ready-to-use instance
func NewSHA256PathHashGetter() *SHA256PathHashGetter {
	return &SHA256PathHashGetter{}
}

// GetHashFilePath returns the path where the given file's hash would be stored.
//
// This implementation uses a simple SHA256-based hash function to generate a
// deterministic hash file path. The algorithm:
//  1. Calculate SHA256 hash of the full file path string
//  2. Encode using base64URL (filesystem-safe encoding)
//  3. Truncate to 12 characters and append .json extension
//  4. Combine with hash directory to create full path
//
// Parameters:
//   - hashDir: Directory where hash files are stored
//   - filePath: The file path to generate hash file path for
//
// Returns:
//   - Full path to the hash file
//   - Error if path generation fails
//
// Note: This implementation always produces .json files regardless of the
// original file type, for consistency with the hash storage format.
func (p *SHA256PathHashGetter) GetHashFilePath(hashDir string, filePath common.ResolvedPath) (string, error) {
	if hashDir == "" {
		return "", ErrEmptyHashDir
	}
	h := sha256.Sum256([]byte(filePath.String()))
	hashStr := base64.URLEncoding.EncodeToString(h[:])

	return filepath.Join(hashDir, hashStr[:12]+".json"), nil
}
