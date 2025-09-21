package output

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// File operation errors
var (
	ErrNotDirectory            = errors.New("path exists but is not a directory")
	ErrNotFile                 = errors.New("path is a directory, not a file")
	ErrDirectoryPermissionMode = errors.New("directory permission mode exceeds security requirement")
)

// SafeFileManager implements FileManager interface using safefileio for secure file operations
type SafeFileManager struct {
	fs safefileio.FileSystem
}

// NewSafeFileManager creates a new SafeFileManager with default safefileio configuration
func NewSafeFileManager() *SafeFileManager {
	return &SafeFileManager{
		fs: safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
	}
}

// CreateTempFile creates a temporary file for output capture with secure permissions (0600)
func (f *SafeFileManager) CreateTempFile(dir string, pattern string) (*os.File, error) {
	// Use os.CreateTemp which automatically creates files with 0600 permissions
	// and provides race-condition-free temporary file creation
	tempFile, err := os.CreateTemp(dir, pattern)
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

	// Use safefileio.SafeAtomicMoveFile for secure atomic file moving
	// This provides protection against TOCTOU attacks and ensures 0600 permissions
	const secureFilePermission = 0o600
	if err := safefileio.SafeAtomicMoveFile(tempPath, finalPath, secureFilePermission); err != nil {
		return fmt.Errorf("failed to move to final path %s: %w", finalPath, err)
	}

	return nil
}

// EnsureDirectory ensures directory exists with proper permissions (0755)
func (f *SafeFileManager) EnsureDirectory(path string) error {
	// Check if path exists and is a file (error case)
	if stat, err := os.Stat(path); err == nil {
		if !stat.IsDir() {
			return fmt.Errorf("path %s exists but is not a directory: %w", path, ErrNotDirectory)
		}
		// Directory already exists
		return nil
	} else if !os.IsNotExist(err) {
		// Some other error occurred
		return fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	// Directory doesn't exist, create it
	const secureDirPermission = 0o750 // More restrictive than 0755 for security
	if err := os.MkdirAll(path, secureDirPermission); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	return nil
}

// RemoveTemp removes temporary file - idempotent operation
func (f *SafeFileManager) RemoveTemp(path string) error {
	// Check if the path exists
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist - this is fine for idempotent operation
			return nil
		}
		return fmt.Errorf("failed to stat file %s: %w", path, err)
	}

	// Check if it's a directory (error case)
	if stat.IsDir() {
		return fmt.Errorf("path %s: %w", path, ErrNotFile)
	}

	// Remove the file
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to remove temporary file %s: %w", path, err)
	}

	return nil
}
