package filevalidator

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

const (
	// HashManifestVersion is the current version of the hash manifest format
	HashManifestVersion = "1.0"
	// HashManifestFormat is the current format of the hash manifest
	HashManifestFormat = "file-hash"
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
func createHashManifest(path common.ResolvedPath, hash, algorithm string) HashManifest {
	return HashManifest{
		Version:   HashManifestVersion,
		Format:    HashManifestFormat,
		Timestamp: time.Now().UTC(),
		File: FileInfo{
			Path: path.String(),
			Hash: HashInfo{
				Algorithm: algorithm,
				Value:     hash,
			},
		},
	}
}

// unmarshalHashManifest unmarshals the JSON content into a HashManifest and handles any parsing errors.
func unmarshalHashManifest(content []byte) (HashManifest, error) {
	var manifest HashManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		switch e := err.(type) {
		case *json.SyntaxError:
			return HashManifest{}, fmt.Errorf("%w: invalid JSON syntax at offset %d", ErrInvalidManifestFormat, e.Offset)
		case *json.UnmarshalTypeError:
			return HashManifest{}, fmt.Errorf("%w: invalid type for field %s", ErrInvalidManifestFormat, e.Field)
		default:
			return HashManifest{}, fmt.Errorf("%w: %v", ErrJSONParseError, err)
		}
	}
	return manifest, nil
}

// validateHashManifest validates the content of manifest file
func validateHashManifest(manifest HashManifest, algoName string, targetPath common.ResolvedPath) error {
	// Version validation
	if manifest.Version != HashManifestVersion {
		return fmt.Errorf("%w: version %s", ErrUnsupportedVersion, manifest.Version)
	}

	// Format validation
	if manifest.Format != HashManifestFormat {
		return fmt.Errorf("%w: format %s", ErrInvalidManifestFormat, manifest.Format)
	}

	// File path validation
	if manifest.File.Path == "" {
		return fmt.Errorf("%w: empty file path", ErrInvalidManifestFormat)
	}

	// Path match confirmation
	if manifest.File.Path != targetPath.String() {
		return fmt.Errorf("%w: path mismatch", ErrHashCollision)
	}

	// Hash algorithm validation
	if manifest.File.Hash.Algorithm != algoName {
		return fmt.Errorf("%w: algorithm mismatch", ErrInvalidManifestFormat)
	}

	// Hash value validation
	if manifest.File.Hash.Value == "" {
		return fmt.Errorf("%w: empty hash value", ErrInvalidManifestFormat)
	}

	// Timestamp validation
	if manifest.Timestamp.IsZero() {
		return fmt.Errorf("%w: zero timestamp", ErrInvalidTimestamp)
	}

	return nil
}
