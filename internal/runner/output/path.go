package output

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Path validation errors
var (
	ErrEmptyPath                = errors.New("output path is empty")
	ErrWorkDirRequired          = errors.New("work directory is required for relative path")
	ErrPathTraversalAbsolute    = errors.New("path traversal detected in absolute path")
	ErrPathTraversalRelative    = errors.New("path traversal detected in relative path")
	ErrPathEscapesWorkDirectory = errors.New("relative path escapes work directory")
)

// DefaultPathValidator provides basic path validation without comprehensive security checks
// Security validation is handled separately by the SecurityValidator
type DefaultPathValidator struct{}

// NewDefaultPathValidator creates a new DefaultPathValidator
func NewDefaultPathValidator() *DefaultPathValidator {
	return &DefaultPathValidator{}
}

// ValidateAndResolvePath validates and resolves an output path
// This performs basic path validation and path traversal prevention
// Additional security checks (symlink detection, etc.) are performed by SecurityValidator
func (v *DefaultPathValidator) ValidateAndResolvePath(outputPath, workDir string) (string, error) {
	// Trim whitespace and check for empty path
	trimmedPath := strings.TrimSpace(outputPath)
	if trimmedPath == "" {
		return "", ErrEmptyPath
	}

	if filepath.IsAbs(trimmedPath) {
		return v.validateAbsolutePath(trimmedPath)
	}

	return v.validateRelativePath(trimmedPath, workDir)
}

// validateAbsolutePath validates and cleans an absolute path
func (v *DefaultPathValidator) validateAbsolutePath(path string) (string, error) {
	// Check for path traversal patterns
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("%w: %s", ErrPathTraversalAbsolute, path)
	}

	// Clean the path to remove redundant separators and resolve . components
	cleanPath := filepath.Clean(path)
	return cleanPath, nil
}

// validateRelativePath validates and resolves a relative path within workDir
func (v *DefaultPathValidator) validateRelativePath(path, workDir string) (string, error) {
	if workDir == "" {
		return "", ErrWorkDirRequired
	}

	// First check for explicit ".." in the path before any processing
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("%w: %s", ErrPathTraversalRelative, path)
	}

	// Join with work directory and clean
	fullPath := filepath.Join(workDir, path)
	cleanPath := filepath.Clean(fullPath)

	// Ensure the result is still within the work directory
	cleanWorkDir := filepath.Clean(workDir)

	// Use filepath.Rel to check if the path escapes the work directory
	relPath, err := filepath.Rel(cleanWorkDir, cleanPath)
	if err != nil || strings.HasPrefix(relPath, "..") || relPath == ".." {
		return "", fmt.Errorf("%w: %s", ErrPathEscapesWorkDirectory, path)
	}

	return cleanPath, nil
}
