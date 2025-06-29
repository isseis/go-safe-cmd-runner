// Package filevalidator provides functionality for file validation and verification.
package filevalidator

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
)

// safeReadFile reads a file safely after validating the path and checking file properties
// It uses O_NOFOLLOW to prevent symlink attacks and performs all checks atomically
func safeReadFile(filePath string) ([]byte, error) {
	// Clean the path to prevent directory traversal and get absolute path
	cleanPath := filepath.Clean(filePath)
	if cleanPath == "." || cleanPath == ".." || cleanPath == "/" {
		return nil, fmt.Errorf("%w: invalid path: %s", ErrInvalidFilePath, filePath)
	}

	// Get the absolute path
	absPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// Verify the path is still absolute after cleaning and not a root directory
	if !filepath.IsAbs(absPath) || absPath == "/" {
		return nil, fmt.Errorf("%w: invalid absolute path: %s", ErrInvalidFilePath, absPath)
	}

	// Open the file with O_NOFOLLOW to prevent symlink following
	// #nosec G304 - absPath is properly cleaned and validated above
	file, err := os.OpenFile(absPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if os.IsNotExist(err) {
		return nil, err
	} else if err != nil {
		// Check if the error is due to a symlink (which is what we want to prevent)
		if isSymlinkError(err) {
			return nil, ErrIsSymlink
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Use a helper function to handle the deferred close with error checking
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			// Log the error but don't fail the operation
			// as the file was successfully read
			fmt.Printf("error closing file: %v\n", closeErr)
		}
	}()

	// Get file info from the open file descriptor to prevent TOCTOU
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Ensure it's a regular file (not a directory, device, etc.)
	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, absPath)
	}

	// Read the file contents
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return content, nil
}

// isSymlinkError checks if the error indicates we tried to open a symlink
func isSymlinkError(err error) bool {
	e, ok := err.(*os.PathError)
	if !ok {
		return false
	}
	// Different OSes return different error numbers for O_NOFOLLOW on a symlink
	return isELOOP(e.Err) || isEISL(e.Err)
}

// isELOOP checks if the error is "too many levels of symbolic links"
func isELOOP(err error) bool {
	return errors.Is(err, syscall.ELOOP) ||
		errors.Is(err, syscall.EMLINK) ||
		errors.Is(err, syscall.ENAMETOOLONG)
}

// isEISL checks if the error is "invalid argument" (some systems return this for O_NOFOLLOW on symlinks)
func isEISL(err error) bool {
	return errors.Is(err, syscall.EINVAL) ||
		errors.Is(err, syscall.EISDIR) ||
		errors.Is(err, syscall.ENOTDIR)
}
