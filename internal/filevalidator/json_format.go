package filevalidator

import (
	"fmt"
	"time"
)

// HashFileFormat defines the JSON format for hash files
type HashFileFormat struct {
	Version   string    `json:"version"`
	Format    string    `json:"format"`
	Timestamp time.Time `json:"timestamp"`
	File      FileInfo  `json:"file"`
}

// FileInfo defines file information
type FileInfo struct {
	Path string   `json:"path"`
	Hash HashInfo `json:"hash"`
}

// HashInfo defines hash information
type HashInfo struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

// createHashFileFormat creates a hash file format structure
func createHashFileFormat(path, hash, algorithm string) HashFileFormat {
	return HashFileFormat{
		Version:   "1.0",
		Format:    "file-hash",
		Timestamp: time.Now().UTC(),
		File: FileInfo{
			Path: path,
			Hash: HashInfo{
				Algorithm: algorithm,
				Value:     hash,
			},
		},
	}
}

// validateHashFile validates the content of JSON format hash files
func validateHashFile(format HashFileFormat, algoName string, targetPath string) error {
	// Version validation
	if format.Version != "1.0" {
		return fmt.Errorf("%w: version %s", ErrUnsupportedVersion, format.Version)
	}

	// Format validation
	if format.Format != "file-hash" {
		return fmt.Errorf("%w: format %s", ErrInvalidJSONFormat, format.Format)
	}

	// File path validation
	if format.File.Path == "" {
		return fmt.Errorf("%w: empty file path", ErrInvalidJSONFormat)
	}

	// Path match confirmation
	if format.File.Path != targetPath {
		return fmt.Errorf("%w: path mismatch", ErrHashCollision)
	}

	// Hash algorithm validation
	if format.File.Hash.Algorithm != algoName {
		return fmt.Errorf("%w: algorithm mismatch", ErrInvalidJSONFormat)
	}

	// Hash value validation
	if format.File.Hash.Value == "" {
		return fmt.Errorf("%w: empty hash value", ErrInvalidJSONFormat)
	}

	// Timestamp validation
	if format.Timestamp.IsZero() {
		return fmt.Errorf("%w: zero timestamp", ErrInvalidTimestamp)
	}

	return nil
}
