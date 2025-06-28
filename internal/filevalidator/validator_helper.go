// Package filevalidator provides functionality for file validation and verification.
package filevalidator

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// safeReadFile reads a file safely after validating the path and checking file properties
func safeReadFile(filePath string) ([]byte, error) {
	// Clean the path to prevent directory traversal
	cleanPath := filepath.Clean(filePath)

	// Get the absolute path to ensure we can properly check for directory traversal
	abspath, err := filepath.Abs(cleanPath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// Verify the path is absolute
	if !filepath.IsAbs(cleanPath) {
		return nil, fmt.Errorf("%w: path is not absolute: %s", ErrInvalidFilePath, cleanPath)
	}

	// Verify the resolved path matches the cleaned path to prevent symlink attacks
	resolvedPath, err := filepath.EvalSymlinks(abspath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve file path: %w", err)
	}
	if resolvedPath != abspath {
		return nil, fmt.Errorf("%w: resolved path does not match: %s", ErrInvalidFilePath, resolvedPath)
	}

	// Verify the file exists and is accessible
	fileInfo, err := os.Stat(abspath)
	if err != nil {
		return nil, fmt.Errorf("failed to access file: %w", err)
	}

	// Ensure it's a regular file (not a directory, symlink, etc.)
	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, abspath)
	}

	// Open the file with read-only flag
	file, err := os.Open(abspath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Use a helper function to handle the deferred close with error checking
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = fmt.Errorf("error closing file: %w", closeErr)
		}
	}()

	// Read the file contents
	content, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return content, nil
}
