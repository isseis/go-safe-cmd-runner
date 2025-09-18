package filevalidator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

const (
	// hashDirectoryPermissions defines the permissions for hash directories
	// 0o750 = owner: read/write/execute, group: read/execute, others: none
	hashDirectoryPermissions = 0o750
)

// Error definitions for static error handling
var (
	ErrPrivilegeManagerNotAvailable    = errors.New("privilege manager not available")
	ErrPrivilegedExecutionNotSupported = errors.New("privileged execution not supported")
)

// FileValidator interface defines the basic file validation methods
type FileValidator interface {
	Record(filePath string, force bool) (string, error)
	Verify(filePath string) error
	VerifyWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) error
	VerifyAndRead(filePath string) ([]byte, error)
	VerifyAndReadWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) ([]byte, error)
}

// HashFilePathGetter is an interface for getting the path where the hash for a file would be stored.
// This is used to test file validation logic for handling hash collisions.
type HashFilePathGetter interface {
	// GetHashFilePath returns the path where the given file's hash would be stored.
	GetHashFilePath(hashAlgorithm HashAlgorithm, hashDir string, filePath common.ResolvedPath) (string, error)
}

// GetHashFilePath returns the path where the hash for the given file would be stored.
func (v *Validator) GetHashFilePath(filePath common.ResolvedPath) (string, error) {
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
	return newValidator(algorithm, hashDir, NewProductionHashFilePathGetter())
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
	info, err := os.Lstat(hashDir)
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
// If force is true, existing hash files for the same file path will be overwritten.
// Hash collisions (different file paths with same hash) always return an error regardless of force.
func (v *Validator) Record(filePath string, force bool) (string, error) {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return "", err
	}

	// Calculate the hash of the file
	hash, err := v.calculateHash(targetPath.String())
	if err != nil {
		return "", fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Get the path for the hash file
	hashFilePath, err := v.GetHashFilePath(targetPath)
	if err != nil {
		return "", err
	}

	// Ensure the directory exists with restrictive permissions
	if err := os.MkdirAll(filepath.Dir(hashFilePath), hashDirectoryPermissions); err != nil {
		return "", fmt.Errorf("failed to create hash directory: %w", err)
	}

	// Check if the hash file already exists and contains a different path
	if existingContent, err := safefileio.SafeReadFile(hashFilePath); err == nil {
		// Parse the existing content as manifest
		existingManifest, err := unmarshalHashManifest(existingContent)
		if err != nil {
			return "", err
		}

		// If the paths don't match, it's a hash collision - always return error
		if existingManifest.File.Path != targetPath.String() {
			return "", fmt.Errorf("%w: hash collision detected between %s and %s",
				ErrHashCollision, existingManifest.File.Path, targetPath)
		}

		// If we get here, the file already exists with the same path
		// If force is false, we should not overwrite it
		if !force {
			return "", fmt.Errorf("hash file already exists for %s: %w", targetPath, ErrHashFileExists)
		}
		// If force is true, we continue and overwrite it
	} else if !os.IsNotExist(err) {
		// Return error if it's not a "not exist" error
		return "", fmt.Errorf("failed to check existing hash file: %w", err)
	}

	// Create manifest hash file
	manifest := createHashManifest(targetPath, hash, v.algorithm.Name())

	err = v.writeHashManifest(hashFilePath, manifest, force)
	if err != nil {
		return "", fmt.Errorf("failed to write hash manifest: %w", err)
	}

	return hashFilePath, nil
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
	actualHash, err := v.calculateHash(targetPath.String())
	if os.IsNotExist(err) {
		return err
	}
	if err != nil {
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

// readAndParseHashFile reads and parses a hash file, returning the file path and hash value.
// It returns an error if the file cannot be read, the JSON is invalid, or the hash file format is incorrect.
func (v *Validator) readAndParseHashFile(targetPath common.ResolvedPath) (string, string, error) {
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

	// Parse and validate the hash file content
	return v.parseAndValidateHashFile(hashFileContent, targetPath)
}

// validatePath validates and normalizes the given file path.
func validatePath(filePath string) (common.ResolvedPath, error) {
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

	return common.NewResolvedPath(resolvedPath)
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

// parseAndValidateHashFile parses and validates a JSON hash file content and returns the path and hash
func (v *Validator) parseAndValidateHashFile(content []byte, targetPath common.ResolvedPath) (string, string, error) {
	// Parse manifest format
	manifest, err := unmarshalHashManifest(content)
	if err != nil {
		return "", "", err
	}

	// Validate the hash file against the target path
	if err := validateHashManifest(manifest, v.algorithm.Name(), targetPath); err != nil {
		return "", "", err
	}

	return manifest.File.Path, manifest.File.Hash.Value, nil
}

// writeHashManifest writes a hash manifest in JSON format with options
func (v *Validator) writeHashManifest(filePath string, manifest HashManifest, force bool) error {
	// Marshal to JSON manifest with indentation
	jsonData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal manifest: %w", err)
	}

	// Add newline
	jsonData = append(jsonData, '\n')

	// Write to file
	if force {
		return safefileio.SafeWriteFileOverwrite(filePath, jsonData, 0o644)
	}
	return safefileio.SafeWriteFile(filePath, jsonData, 0o644)
}

// VerifyFromHandle verifies a file's hash using an already opened file handle
func (v *Validator) VerifyFromHandle(file *os.File, targetPath common.ResolvedPath) error {
	// Calculate hash directly from file handle (normal privilege)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek file to start: %w", err)
	}
	actualHash, err := v.algorithm.Sum(file)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	// Read recorded hash (normal privilege)
	_, expectedHash, err := v.readAndParseHashFile(targetPath)
	if err != nil {
		return err
	}

	// Compare hashes
	if expectedHash != actualHash {
		return ErrMismatch
	}

	return nil
}

// VerifyWithPrivileges verifies a file's integrity using privilege escalation
// This method assumes that normal verification has already failed with a permission error
func (v *Validator) VerifyWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) error {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return err
	}

	// Check if privilege manager is available
	if privManager == nil {
		return fmt.Errorf("failed to verify file %s: %w", targetPath, ErrPrivilegeManagerNotAvailable)
	}

	// Check if privilege escalation is supported
	if !privManager.IsPrivilegedExecutionSupported() {
		return fmt.Errorf("failed to verify file %s: %w", targetPath, ErrPrivilegedExecutionNotSupported)
	}

	// Open file with privileges
	file, openErr := OpenFileWithPrivileges(targetPath.String(), privManager)
	if openErr != nil {
		return fmt.Errorf("failed to open file with privileges: %w", openErr)
	}
	defer func() {
		_ = file.Close() // Ignore close error
	}()

	// Verify using the opened file handle
	return v.VerifyFromHandle(file, targetPath)
}

// verifyAndReadContent performs the common verification and reading logic
// readContent should return the file content and any read error
func (v *Validator) verifyAndReadContent(targetPath common.ResolvedPath, readContent func() ([]byte, error)) ([]byte, error) {
	// Read file content
	content, err := readContent()
	if err != nil {
		return nil, err
	}

	// Calculate hash of the content we just read
	actualHash, err := v.algorithm.Sum(bytes.NewReader(content))
	if err != nil {
		return nil, err
	}

	// Get expected hash
	_, expectedHash, err := v.readAndParseHashFile(targetPath)
	if err != nil {
		return nil, err
	}

	// Compare hashes
	if expectedHash != actualHash {
		return nil, ErrMismatch
	}

	return content, nil
}

// VerifyAndRead atomically verifies file integrity and returns its content to prevent TOCTOU attacks
func (v *Validator) VerifyAndRead(filePath string) ([]byte, error) {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return nil, err
	}

	// Use common verification logic with normal file reading
	return v.verifyAndReadContent(targetPath, func() ([]byte, error) {
		content, err := safefileio.SafeReadFile(targetPath.String())
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return content, nil
	})
}

// VerifyAndReadWithPrivileges atomically verifies file integrity and returns its content using privileged access
func (v *Validator) VerifyAndReadWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) ([]byte, error) {
	// Validate the file path
	targetPath, err := validatePath(filePath)
	if err != nil {
		return nil, err
	}

	// Check if privilege manager is available
	if privManager == nil {
		return nil, fmt.Errorf("failed to verify and read file %s: %w", targetPath, ErrPrivilegeManagerNotAvailable)
	}

	// Check if privilege escalation is supported
	if !privManager.IsPrivilegedExecutionSupported() {
		return nil, fmt.Errorf("failed to verify and read file %s: %w", targetPath, ErrPrivilegedExecutionNotSupported)
	}

	// Use common verification logic with privileged file reading
	return v.verifyAndReadContent(targetPath, func() ([]byte, error) {
		// Open file with privileges
		file, openErr := OpenFileWithPrivileges(targetPath.String(), privManager)
		if openErr != nil {
			return nil, fmt.Errorf("failed to open file with privileges: %w", openErr)
		}
		defer func() {
			_ = file.Close() // Ignore close error
		}()

		// Read content from the opened file handle
		content, err := io.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file content: %w", err)
		}
		return content, nil
	})
}
