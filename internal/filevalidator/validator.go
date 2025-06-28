package filevalidator

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Validator provides functionality to record and verify file hashes.
// It should be instantiated using the New function.
type Validator struct {
	algorithm HashAlgorithm
	hashDir   string
}

// New initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
// The testing.T parameter is optional and used only for testing.
func New(algorithm HashAlgorithm, hashDir string) (*Validator, error) {
	if algorithm == nil {
		return nil, ErrNilAlgorithm
	}

	// Clean and make the path absolute
	hashDir, err := filepath.Abs(filepath.Clean(hashDir))
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
		algorithm: algorithm,
		hashDir:   hashDir,
	}, nil
}

// Record calculates the hash of the file at filePath and saves it to the hash directory.
// The hash file is named using a URL-safe Base64 encoding of the file path.
func (v *Validator) Record(filePath string) error {
	// Validate the file path
	targetPath, err := v.validatePath(filePath)
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
	if err := os.MkdirAll(filepath.Dir(hashFilePath), 0o755); err != nil {
		return fmt.Errorf("failed to create hash directory: %w", err)
	}

	// Check if the hash file already exists and contains a different path
	if existingContent, err := safeReadFile(hashFilePath); err == nil {
		// File exists, check the recorded path
		parts := strings.SplitN(string(existingContent), "\n", 2)
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
	return os.WriteFile(hashFilePath, []byte(fmt.Sprintf("%s\n%s", targetPath, hash)), 0o644)
}

// Verify checks if the file at filePath matches its recorded hash.
// Returns ErrMismatch if the hashes don't match, or ErrHashFileNotFound if no hash is recorded.
func (v *Validator) Verify(filePath string) error {
	// Validate the file path
	targetPath, err := v.validatePath(filePath)
	if err != nil {
		return err
	}

	// Get the path to the hash file
	hashFilePath, err := v.GetHashFilePath(targetPath)
	if err != nil {
		return err
	}

	// Read the stored hash file
	hashFileContent, err := safeReadFile(hashFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ErrHashFileNotFound
		}
		return fmt.Errorf("failed to read hash file: %w", err)
	}

	// Parse the hash file content (format: "filepath\nhash")
	parts := strings.SplitN(string(hashFileContent), "\n", 2)
	if len(parts) != 2 {
		return fmt.Errorf("%w: expected 'path\nhash', got %d parts", ErrInvalidHashFileFormat, len(parts))
	}

	// Check if the recorded path matches the current file path
	recordedPath := parts[0]
	if recordedPath == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidHashFileFormat)
	}
	if recordedPath != targetPath {
		return fmt.Errorf("%w: recorded path '%s' does not match current path '%s'", ErrHashCollision, recordedPath, targetPath)
	}

	expectedHash := parts[1]

	// Calculate the current hash
	actualHash, err := v.calculateHash(targetPath)
	if err != nil {
		return fmt.Errorf("failed to calculate file hash: %w", err)
	}

	// Compare the hashes
	if expectedHash != actualHash {
		return ErrMismatch
	}

	return nil
}

// GetHashFilePath returns the path where the hash for the given file would be stored.
func (v *Validator) GetHashFilePath(filePath string) (string, error) {
	if v.algorithm == nil {
		return "", ErrNilAlgorithm
	}

	if filePath == "" {
		return "", ErrInvalidFilePath
	}

	encodedPath, err := encodePath(filePath)
	if err != nil {
		return "", err
	}

	// Create a hash of the encoded path to handle long paths and special characters
	h := sha256.Sum256([]byte(encodedPath))
	hashStr := base64.URLEncoding.EncodeToString(h[:])

	// Use the first 12 characters of the hash as the filename
	return filepath.Join(v.hashDir, hashStr[:12]+"."+v.algorithm.Name()), nil
}

// GetTargetFilePath decodes the original file path from a hash file path.
func (v *Validator) GetTargetFilePath(hashFilePath string) (string, error) {
	if v.algorithm == nil {
		return "", ErrNilAlgorithm
	}

	// Ensure the hash file is within the hash directory
	hashFilePath, err := filepath.Abs(filepath.Clean(hashFilePath))
	if err != nil {
		return "", fmt.Errorf("invalid hash file path: %w", err)
	}

	relPath, err := filepath.Rel(v.hashDir, hashFilePath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		return "", fmt.Errorf("hash file is not within the hash directory: %w", ErrInvalidFilePath)
	}

	// Extract the hash from the filename
	ext := "." + v.algorithm.Name()
	if !strings.HasSuffix(relPath, ext) {
		return "", fmt.Errorf("invalid hash file extension: %w", ErrInvalidFilePath)
	}

	// Read the hash file content
	content, err := os.ReadFile(hashFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read hash file: %w", err)
	}

	// The first line of the hash file contains the original file path
	parts := strings.SplitN(string(content), "\n", 2)
	if len(parts) < 1 {
		return "", ErrInvalidHashFileFormat
	}

	return parts[0], nil
}

// validatePath validates and normalizes the given file path.
func (v *Validator) validatePath(filePath string) (string, error) {
	if filePath == "" {
		return "", ErrInvalidFilePath
	}

	// Clean and make the path absolute
	abspath, err := filepath.Abs(filepath.Clean(filePath))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// Check if the path is a symlink
	info, err := os.Lstat(abspath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", os.ErrNotExist, abspath)
		}
		return "", fmt.Errorf("failed to access file: %w", err)
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return "", ErrIsSymlink
	}

	return abspath, nil
}

// calculateHash calculates the hash of the file at the given path.
func (v *Validator) calculateHash(filePath string) (string, error) {
	// Clean and validate the file path
	cleanPath := filepath.Clean(filePath)
	if cleanPath != filePath {
		return "", fmt.Errorf("%w: %s", ErrSuspiciousFilePath, filePath)
	}

	file, err := os.Open(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			// Log the error but don't fail the operation
			// as the file was successfully read
			_ = fmt.Errorf("failed to close file: %w", err)
		}
	}()

	return v.algorithm.Sum(file)
}

// encodePath encodes a file path to a URL-safe base64 string.
func encodePath(path string) (string, error) {
	// Clean the path to remove any relative components
	cleanPath := filepath.Clean(path)

	// Convert to absolute path to ensure consistency
	abspath, err := filepath.Abs(cleanPath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Encode the path using URL-safe base64 without padding
	encoded := base64.URLEncoding.EncodeToString([]byte(abspath))
	return strings.TrimRight(encoded, "="), nil
}
