package filevalidator

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
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

// GetHashFilePath returns the path where the hash for the given file would be stored.
func (v *Validator) GetHashFilePath(filePath common.ResolvedPath) (string, error) {
	return v.hashFilePathGetter.GetHashFilePath(v.hashDir, filePath)
}

// GetStore returns the underlying fileanalysis.Store.
// This is useful for accessing syscall analysis results stored alongside hashes.
func (v *Validator) GetStore() *fileanalysis.Store {
	return v.store
}

// Validator provides functionality to record and verify file hashes.
// It should be instantiated using the New function.
type Validator struct {
	algorithm               HashAlgorithm
	hashDir                 string
	hashFilePathGetter      common.HashFilePathGetter
	privilegedFileValidator *PrivilegedFileValidator

	// store is the unified analysis store for FileAnalysisRecord format.
	store *fileanalysis.Store
}

// New initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
// The hash directory is created automatically if it does not exist.
// This constructor uses the FileAnalysisRecord format for storing hash and analysis results.
// The analysis store preserves existing fields (e.g., SyscallAnalysis) when updating hashes.
func New(algorithm HashAlgorithm, hashDir string) (*Validator, error) {
	// Resolve to absolute path early, consistent with newValidator behavior.
	var err error
	hashDir, err = filepath.Abs(hashDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for hash directory: %w", err)
	}

	hashFilePathGetter := NewHybridHashFilePathGetter()

	// Create analysis store first — this creates the directory if it doesn't exist.
	store, err := fileanalysis.NewStore(hashDir, hashFilePathGetter)
	if err != nil {
		return nil, fmt.Errorf("failed to create analysis store: %w", err)
	}

	// Now create the validator — the directory is guaranteed to exist.
	v, err := newValidator(algorithm, hashDir, hashFilePathGetter)
	if err != nil {
		return nil, err
	}
	v.store = store

	return v, nil
}

// newValidator initializes and returns a new Validator with the specified hash algorithm and hash directory.
// Returns an error if the algorithm is nil or if the hash directory cannot be accessed.
func newValidator(algorithm HashAlgorithm, hashDir string, hashFilePathGetter common.HashFilePathGetter) (*Validator, error) {
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
		algorithm:               algorithm,
		hashDir:                 hashDir,
		hashFilePathGetter:      hashFilePathGetter,
		privilegedFileValidator: DefaultPrivilegedFileValidator(),
	}, nil
}

// Record calculates the hash of the file at filePath and saves it to the hash directory.
// The hash file is named using a URL-safe Base64 encoding of the file path.
// If force is true, existing hash files for the same file path will be overwritten.
// Records are stored by file path (FileAnalysisRecord format), so identical content in
// different files does not cause a collision error.
// Existing fields (e.g., SyscallAnalysis) in the record are preserved when updating.
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

	return v.saveHash(targetPath, hash, hashFilePath, force)
}

// saveHash saves the hash using FileAnalysisRecord format.
// This format preserves existing fields (e.g., SyscallAnalysis) when updating.
func (v *Validator) saveHash(filePath common.ResolvedPath, hash, hashFilePath string, force bool) (string, error) {
	// Check for existing record
	_, err := v.store.Load(filePath)
	if err == nil {
		// Record exists
		if !force {
			return "", fmt.Errorf("hash file already exists for %s: %w", filePath, ErrHashFileExists)
		}
	} else if !errors.Is(err, fileanalysis.ErrRecordNotFound) {
		// For errors other than "not found", we proceed to Update only if the error is a
		// schema mismatch or corruption error that can be handled there. Otherwise, we fail.
		var schemaErr *fileanalysis.SchemaVersionMismatchError
		var corruptedErr *fileanalysis.RecordCorruptedError
		canProceedToUpdate := errors.As(err, &schemaErr) || errors.As(err, &corruptedErr)
		if !canProceedToUpdate {
			return "", fmt.Errorf("failed to check existing record: %w", err)
		}
	}

	// Use Update to preserve existing fields (e.g., SyscallAnalysis)
	contentHash := fmt.Sprintf("%s:%s", v.algorithm.Name(), hash)
	err = v.store.Update(filePath, func(record *fileanalysis.Record) error {
		record.ContentHash = contentHash
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to update analysis record: %w", err)
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

	return v.verifyHash(targetPath, actualHash)
}

// verifyHash verifies the hash using FileAnalysisRecord format.
func (v *Validator) verifyHash(filePath common.ResolvedPath, actualHash string) error {
	record, err := v.store.Load(filePath)
	if err != nil {
		if errors.Is(err, fileanalysis.ErrRecordNotFound) {
			return ErrHashFileNotFound
		}
		return fmt.Errorf("failed to load analysis record: %w", err)
	}

	// ContentHash is in prefixed format "sha256:<hex>"
	expectedHash := fmt.Sprintf("%s:%s", v.algorithm.Name(), actualHash)
	if record.ContentHash != expectedHash {
		return ErrMismatch
	}

	return nil
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

// VerifyFromHandle verifies a file's hash using an already opened file handle.
// The file parameter must implement io.ReadSeeker (satisfied by *os.File and safefileio.File).
func (v *Validator) VerifyFromHandle(file io.ReadSeeker, targetPath common.ResolvedPath) error {
	// Calculate hash directly from file handle (normal privilege)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to seek file to start: %w", err)
	}
	actualHash, err := v.algorithm.Sum(file)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}

	return v.verifyHash(targetPath, actualHash)
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
	file, openErr := v.privilegedFileValidator.OpenFileWithPrivileges(targetPath.String(), privManager)
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

	if verifyErr := v.verifyHash(targetPath, actualHash); verifyErr != nil {
		return nil, verifyErr
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
		file, openErr := v.privilegedFileValidator.OpenFileWithPrivileges(targetPath.String(), privManager)
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
