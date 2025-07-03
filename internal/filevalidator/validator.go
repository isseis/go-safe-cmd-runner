package filevalidator

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// HashFilePathGetter is an interface for getting the path where the hash for a file would be stored.
// This is used to test file validation logic for handling hash collisions.
type HashFilePathGetter interface {
	// GetHashFilePath returns the path where the given file's hash would be stored.
	GetHashFilePath(hashAlgorithm HashAlgorithm, hashDir string, filePath string) (string, error)
}

// ProductionHashFilePathGetter is a concrete implementation of HashFilePathGetter.
type ProductionHashFilePathGetter struct{}

// GetHashFilePath returns the path where the given file's hash would be stored.
// This implementation uses a simple hash function to generate a hash file path.
func (p *ProductionHashFilePathGetter) GetHashFilePath(hashAlgorithm HashAlgorithm, hashDir string, filePath string) (string, error) {
	if hashAlgorithm == nil {
		return "", ErrNilAlgorithm
	}

	targetPath, err := validatePath(filePath)
	if err != nil {
		return "", err
	}

	h := sha256.Sum256([]byte(targetPath))
	hashStr := base64.URLEncoding.EncodeToString(h[:])

	return filepath.Join(hashDir, hashStr[:12]+"."+hashAlgorithm.Name()), nil
}

// GetHashFilePath returns the path where the hash for the given file would be stored.
func (v *Validator) GetHashFilePath(filePath string) (string, error) {
	return v.hashFilePathGetter.GetHashFilePath(v.algorithm, v.hashDir, filePath)
}

// Validator provides functionality to record and verify file hashes.
// It should be instantiated using the New function.
type Validator struct {
	algorithm          HashAlgorithm
	hashDir            string
	hashFilePathGetter HashFilePathGetter
}

// New initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
func New(algorithm HashAlgorithm, hashDir string) (*Validator, error) {
	return newValidator(algorithm, hashDir, &ProductionHashFilePathGetter{})
}

// newValidator initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
func newValidator(algorithm HashAlgorithm, hashDir string, hashFilePathGetter HashFilePathGetter) (*Validator, error) {
	if algorithm == nil {
		return nil, ErrNilAlgorithm
	}

	hashDir, err := filepath.Abs(hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for hash directory: %w", err)
	}

	// Ensure the hash directory exists and is a directory
	info, err := os.Stat(hashDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrHashDirNotExist, hashDir)
		}
		return nil, fmt.Errorf("failed to access hash directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s", ErrHashPathNotDir, hashDir)
	}

	return &Validator{
		algorithm:          algorithm,
		hashDir:            hashDir,
		hashFilePathGetter: hashFilePathGetter,
	}, nil
}

// Record calculates the hash of the file at filePath and saves it to the hash directory.
// The hash file is named using a URL-safe Base64 encoding of the file path.
func (v *Validator) Record(filePath string) error {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return err
	}

	// Calculate the hash of the file
	hash, err := v.calculateHash(targetPath)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Get the path for the hash file
	hashFilePath, err := v.GetHashFilePath(targetPath)
	if err != nil {
		return err
	}

	// Ensure the directory exists
	if err := os.MkdirAll(filepath.Dir(hashFilePath), 0o750); err != nil {
		return fmt.Errorf("failed to create hash directory: %w", err)
	}

	// Check if the hash file already exists and contains a different path
	if existingContent, err := safefileio.SafeReadFile(hashFilePath); err == nil {
		// File exists, check the recorded path
		parts := strings.SplitN(string(existingContent), "\n", 2) //nolint:mnd
		if len(parts) == 0 {
			return fmt.Errorf("%w: empty file", ErrInvalidHashFileFormat)
		}
		recordedPath := parts[0]
		if recordedPath != targetPath {
			return fmt.Errorf("%w: path '%s' conflicts with existing path '%s'", ErrHashCollision, targetPath, recordedPath)
		}
		// If we get here, the file exists and has the same path, so we can overwrite it
	} else if !os.IsNotExist(err) {
		// Return error if it's not a "not exist" error
		return fmt.Errorf("failed to check existing hash file: %w", err)
	}

	// Write the target path and hash to the hash file
	return safefileio.SafeWriteFile(hashFilePath, fmt.Appendf(nil, "%s\n%s", targetPath, hash), 0o640)
}

// GetHashAlgorithm returns the hash algorithm used by the validator.
func (v *Validator) GetHashAlgorithm() HashAlgorithm {
	return v.algorithm
}

// GetHashDir returns the directory for storing the hash files.
func (v *Validator) GetHashDir() string {
	return v.hashDir
}

// Verify checks if the file at filePath matches its recorded hash.
// Returns ErrMismatch if the hashes don't match, or ErrHashFileNotFound if no hash is recorded.
func (v *Validator) Verify(filePath string) error {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return err
	}

	// Calculate the current hash
	actualHash, err := v.calculateHash(targetPath)
	if os.IsNotExist(err) {
		return err
	} else if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	_, expectedHash, err := v.readAndParseHashFile(targetPath)
	if err != nil {
		return err
	}
	// Compare the hashes
	if expectedHash != actualHash {
		return ErrMismatch
	}

	return nil
}

func (v *Validator) readAndParseHashFile(targetPath string) (string, string, error) {
	// Get the path to the hash file
	hashFilePath, err := v.GetHashFilePath(targetPath)
	if err != nil {
		return "", "", err
	}

	// Read the stored hash file
	hashFileContent, err := safefileio.SafeReadFile(hashFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", ErrHashFileNotFound
		}
		return "", "", fmt.Errorf("failed to read hash file: %w", err)
	}

	// Parse the hash file content (format: "filepath\nhash")
	numLines := 2
	parts := strings.SplitN(string(hashFileContent), "\n", numLines)
	if len(parts) != numLines {
		return "", "", fmt.Errorf("%w: expected 'path\nhash', got %d parts", ErrInvalidHashFileFormat, len(parts))
	}

	// Check if the recorded path matches the current file path
	recordedPath := parts[0]
	if recordedPath == "" {
		return "", "", fmt.Errorf("%w: empty path", ErrInvalidHashFileFormat)
	}
	if recordedPath != targetPath {
		return "", "", fmt.Errorf("%w: recorded path '%s' does not match current path '%s'", ErrHashCollision, recordedPath, targetPath)
	}

	expectedHash := parts[1]
	return recordedPath, expectedHash, nil
}

// validatePath validates and normalizes the given file path.
func validatePath(filePath string) (string, error) {
	if filePath == "" {
		return "", safefileio.ErrInvalidFilePath
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", err
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return "", err
	}
	// check if resolvedPath is a regular file
	fileInfo, err := os.Lstat(resolvedPath)
	if err != nil {
		return "", err
	}
	if !fileInfo.Mode().IsRegular() {
		return "", fmt.Errorf("%w: not a regular file: %s", safefileio.ErrInvalidFilePath, resolvedPath)
	}
	return resolvedPath, nil
}

// calculateHash calculates the hash of the file at the given path.
// filePath must be validated by validatePath before calling this function.
func (v *Validator) calculateHash(filePath string) (string, error) {
	content, err := safefileio.SafeReadFile(filePath)
	if err != nil {
		return "", err
	}
	return v.algorithm.Sum(bytes.NewReader(content))
}
