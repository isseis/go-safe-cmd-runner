package output

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// File operation errors
var (
	ErrNotDirectory = errors.New("path exists but is not a directory")
	ErrNotFile      = errors.New("path is a directory, not a file")
)

// SafeFileManager implements FileManager interface using both safefileio and common FileSystem interfaces
type SafeFileManager struct {
	safeFS   safefileio.FileSystem // For security-critical file operations
	commonFS common.FileSystem     // For general file system operations
}

// NewSafeFileManager creates a new SafeFileManager with default configurations
func NewSafeFileManager() *SafeFileManager {
	return &SafeFileManager{
		safeFS:   safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
		commonFS: common.NewDefaultFileSystem(),
	}
}

// NewSafeFileManagerWithFS creates a new SafeFileManager with custom FileSystem implementations
// This constructor is useful for testing with mock implementations
func NewSafeFileManagerWithFS(safeFS safefileio.FileSystem, commonFS common.FileSystem) *SafeFileManager {
	return &SafeFileManager{
		safeFS:   safeFS,
		commonFS: commonFS,
	}
}

// CreateTempFile creates a temporary file for output capture with secure permissions (0600)
func (f *SafeFileManager) CreateTempFile(dir string, pattern string) (*os.File, error) {
	// Use commonFS.CreateTemp for consistent file system operations and testability
	// This automatically creates files with 0600 permissions and provides race-condition-free temporary file creation
	tempFile, err := f.commonFS.CreateTemp(dir, pattern)
	if err != nil {
		return nil, fmt.Errorf("failed to create temporary file: %w", err)
	}

	return tempFile, nil
}

// WriteToTemp writes data to temporary file
func (f *SafeFileManager) WriteToTemp(file *os.File, data []byte) (int, error) {
	n, err := file.Write(data)
	if err != nil {
		return n, fmt.Errorf("failed to write to temporary file: %w", err)
	}

	return n, nil
}

// MoveToFinal atomically moves temp file to final location using safefileio
func (f *SafeFileManager) MoveToFinal(tempPath, finalPath string) error {
	// Ensure the directory exists for the final path
	finalDir := filepath.Dir(finalPath)
	if err := f.EnsureDirectory(finalDir); err != nil {
		return fmt.Errorf("failed to ensure directory for final path: %w", err)
	}

	// Use safeFS.AtomicMoveFile for secure atomic file moving
	// This provides protection against TOCTOU attacks and ensures 0600 permissions
	const secureFilePermission = 0o600
	if err := f.safeFS.AtomicMoveFile(tempPath, finalPath, secureFilePermission); err != nil {
		return fmt.Errorf("failed to move to final path %s: %w", finalPath, err)
	}

	return nil
}

// EnsureDirectory ensures directory exists with proper permissions (0750)
func (f *SafeFileManager) EnsureDirectory(path string) error {
	// Use common.FileSystem for directory operations

	// Check if path exists using common.FileSystem
	exists, err := f.commonFS.FileExists(path)
	if err != nil {
		return fmt.Errorf("failed to check if path exists %s: %w", path, err)
	}

	if exists {
		// Check if it's actually a directory
		isDir, err := f.commonFS.IsDir(path)
		if err != nil {
			return fmt.Errorf("failed to check if path is directory %s: %w", path, err)
		}
		if !isDir {
			return fmt.Errorf("path %s exists but is not a directory: %w", path, ErrNotDirectory)
		}
		// Directory already exists
		return nil
	}

	// Directory doesn't exist, create it using commonFS.MkdirAll
	const secureDirPermission = 0o750 // More restrictive than 0755 for security
	if err := f.commonFS.MkdirAll(path, secureDirPermission); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	return nil
}

// RemoveTemp removes temporary file - idempotent operation
func (f *SafeFileManager) RemoveTemp(path string) error {
	// Use common.FileSystem for file removal operations

	// Check if the path exists
	exists, err := f.commonFS.FileExists(path)
	if err != nil {
		return fmt.Errorf("failed to check if file exists %s: %w", path, err)
	}

	if !exists {
		// File doesn't exist - this is fine for idempotent operation
		return nil
	}

	// Check if it's a directory (error case)
	isDir, err := f.commonFS.IsDir(path)
	if err != nil {
		return fmt.Errorf("failed to check if path is directory %s: %w", path, err)
	}
	if isDir {
		return fmt.Errorf("path %s: %w", path, ErrNotFile)
	}

	// Remove the file using common.FileSystem
	if err := f.commonFS.Remove(path); err != nil {
		return fmt.Errorf("failed to remove temporary file %s: %w", path, err)
	}

	return nil
}
