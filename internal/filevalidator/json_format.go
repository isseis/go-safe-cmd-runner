package filevalidator

import (
	"encoding/json"
	"fmt"
	"time"
)

// HashManifest defines the JSON format for hash files
type HashManifest struct {
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

// createHashManifest creates a hash manifest structure
func createHashManifest(path, hash, algorithm string) HashManifest {
	return HashManifest{
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

// unmarshalHashManifest unmarshals the JSON content into a HashManifest and handles any parsing errors.
func unmarshalHashManifest(content []byte) (HashManifest, error) {
	var format HashManifest
	if err := json.Unmarshal(content, &format); err != nil {
		if jsonErr, ok := err.(*json.SyntaxError); ok {
			return HashManifest{}, fmt.Errorf("%w: invalid JSON syntax at offset %d", ErrInvalidJSONFormat, jsonErr.Offset)
		}
		return HashManifest{}, fmt.Errorf("%w: %v", ErrJSONParseError, err)
	}
	return format, nil
}

// validateHashManifest validates the content of JSON format hash files
func validateHashManifest(format HashManifest, algoName string, targetPath string) error {
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
