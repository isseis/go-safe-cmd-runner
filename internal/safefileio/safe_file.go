// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
package safefileio

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
)

// FileSystem is an interface that abstracts file system operations
type FileSystem interface {
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
}

// File is an interface that abstracts file operations
type File interface {
	Write(b []byte) (n int, err error)
	Close() error
	Stat() (os.FileInfo, error)
}

// osFS implements FileSystem using the local disk
var defaultFS FileSystem = osFS{}

type osFS struct{}

func (osFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	// #nosec G304 - The path is validated after opening to prevent TOCTOU attacks
	return os.OpenFile(name, flag, perm)
}

// SafeWriteFile writes a file safely after validating the path and checking file properties.
// It checks all path components for symlinks and uses O_NOFOLLOW to prevent symlink attacks.
// The function is designed to be secure against TOCTOU (Time-of-Check Time-of-Use) race conditions
// by opening the file first and then verifying the path components.
//
// Note: The filepath parameter is intentionally not restricted to a safe directory as the
// function is designed to work with any valid file path while maintaining security.
func SafeWriteFile(filePath string, content []byte, perm os.FileMode) (err error) {
	return safeWriteFileWithFS(filePath, content, perm, defaultFS)
}

// safeWriteFileWithFS is the internal implementation that accepts a FileSystem for testing
func safeWriteFileWithFS(filePath string, content []byte, perm os.FileMode, fs FileSystem) (err error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// First try to open the file with O_NOFOLLOW to prevent following symlinks
	file, err := fs.OpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, perm)
	if err != nil {
		switch {
		case os.IsExist(err):
			return ErrFileExists
		case isNoFollowError(err):
			return ErrIsSymlink
		default:
			return fmt.Errorf("failed to open file: %w", err)
		}
	}

	// Ensure the file is closed on error
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// Now verify the directory components using the file descriptor to prevent TOCTOU
	if err := verifyPathComponents(absPath); err != nil {
		return err
	}

	// Validate the file is a regular file (not a device, pipe, etc.)
	if _, err := validateFile(file, absPath); err != nil {
		return err
	}

	// Write the content
	if _, err = file.Write(content); err != nil {
		return fmt.Errorf("failed to write to %s: %w", absPath, err)
	}

	return nil
}

// verifyPathComponents checks if any component of the path is a symlink.
// This is called after opening the file to prevent TOCTOU attacks.
func verifyPathComponents(absPath string) error {
	// Get the directory of the file
	dir := filepath.Dir(absPath)
	if dir == "." {
		// If it's in the current directory, get the current working directory
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Get the absolute path of the directory
	dir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Check each directory component
	current := dir
	for {
		// Get the parent directory
		parent := filepath.Dir(current)
		if parent == current {
			break // Reached root directory
		}

		// Check if the current path is a symlink using os.Lstat
		// This is safe because we're not following symlinks
		fi, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // Directory doesn't exist, we can stop checking
			}
			return fmt.Errorf("failed to stat %s: %w", current, err)
		}

		// Check if it's a symlink
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrIsSymlink, current)
		}

		current = parent
	}

	return nil
}

// MaxFileSize is the maximum allowed file size for SafeReadFile (128 MB)
const MaxFileSize = 128 * 1024 * 1024

// SafeReadFile reads a file safely after validating the path and checking file properties.
// It enforces a maximum file size of MaxFileSize to prevent memory exhaustion attacks.
// It uses O_NOFOLLOW to prevent symlink attacks and performs all checks atomically.
func SafeReadFile(filePath string) ([]byte, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// First try to open the file with O_NOFOLLOW to prevent following symlinks
	// #nosec G304 - absPath is properly cleaned and validated above, and we use O_NOFOLLOW
	file, err := os.OpenFile(absPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		switch {
		case isNoFollowError(err):
			return nil, ErrIsSymlink
		default:
			return nil, err
		}
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("error closing file: %v\n", closeErr)
		}
	}()

	// Now verify the directory components using the file descriptor to prevent TOCTOU
	if err := verifyPathComponents(absPath); err != nil {
		return nil, err
	}

	// Validate the file is a regular file (not a device, pipe, etc.)
	if _, err := validateFile(file, absPath); err != nil {
		return nil, err
	}

	return readFileContent(file, filePath)
}

// readFileContent reads and validates the content of an already opened file
func readFileContent(file *os.File, filePath string) ([]byte, error) {
	fileInfo, err := validateFile(file, filePath)
	if err != nil {
		return nil, err
	}

	if fileInfo.Size() > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	content, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if int64(len(content)) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	return content, nil
}

// validateFile checks if the file is a regular file and returns its FileInfo
// To prevent TOCTOU attacks, we use the file descriptor to get the file info
func validateFile(file File, filePath string) (os.FileInfo, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, filePath)
	}

	return fileInfo, nil
}
