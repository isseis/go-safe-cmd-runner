// Package common provides shared interfaces and utilities used across the runner packages.
//
//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Error definitions for static error handling
var (
	ErrEmptyPath = errors.New("path cannot be empty")
)

// FileSystem defines the interface for file system operations
// This interface allows for easy mocking in tests and provides a consistent API
// for file operations across all packages.
type FileSystem interface {
	// CreateTempDir creates a temporary directory with the given prefix
	CreateTempDir(dir string, prefix string) (string, error)

	// TempDir returns the default directory for temporary files
	TempDir() string

	// RemoveAll removes a directory and all its contents
	RemoveAll(path string) error

	// Remove removes a single file or empty directory
	Remove(path string) error

	// Lstat returns file information
	Lstat(path string) (fs.FileInfo, error)

	// FileExists checks if a file or directory exists
	FileExists(path string) (bool, error)

	// IsDir checks if the path is a directory
	IsDir(path string) (bool, error)

	// CreateTemp creates a temporary file with the given prefix in the specified directory
	CreateTemp(dir string, pattern string) (*os.File, error)

	// MkdirAll creates a directory and all necessary parents with the specified permissions
	MkdirAll(path string, perm os.FileMode) error
}

// DefaultFileSystem implements FileSystem using standard os package functions
type DefaultFileSystem struct{}

// NewDefaultFileSystem creates a new DefaultFileSystem
func NewDefaultFileSystem() *DefaultFileSystem {
	return &DefaultFileSystem{}
}

// CreateTempDir creates a temporary directory with the given prefix
func (fs *DefaultFileSystem) CreateTempDir(dir string, prefix string) (string, error) {
	return os.MkdirTemp(dir, prefix)
}

// TempDir returns the default directory for temporary files
func (fs *DefaultFileSystem) TempDir() string {
	return os.TempDir()
}

// RemoveAll removes a directory and all its contents
func (fs *DefaultFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Remove removes a single file or empty directory
func (fs *DefaultFileSystem) Remove(path string) error {
	return os.Remove(path)
}

// Lstat returns file information
func (fs *DefaultFileSystem) Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

// FileExists checks if a file or directory exists
func (fs *DefaultFileSystem) FileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// IsDir checks if the path is a directory
func (fs *DefaultFileSystem) IsDir(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// CreateTemp creates a temporary file with the given prefix in the specified directory
func (fs *DefaultFileSystem) CreateTemp(dir string, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

// MkdirAll creates a directory and all necessary parents with the specified permissions
func (fs *DefaultFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// ResolvedPath represents a file path that has been resolved to an absolute path
// with all symbolic links evaluated. It can only be created via constructors,
// ensuring that the path is always in a normalized form.
type ResolvedPath struct {
	path string
}

// NewResolvedPath creates a ResolvedPath for an existing file or directory.
// It resolves the path to an absolute path and evaluates all symbolic links.
// Returns ErrEmptyPath if the path is empty, or any error from Abs/EvalSymlinks.
func NewResolvedPath(path string) (ResolvedPath, error) {
	if path == "" {
		return ResolvedPath{}, ErrEmptyPath
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ResolvedPath{}, err
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return ResolvedPath{}, err
	}
	return ResolvedPath{path: resolvedPath}, nil
}

// String returns the resolved path as a string.
func (p ResolvedPath) String() string {
	return p.path
}

// ContainsPathTraversalSegment checks if a path contains ".." as a distinct path segment
// This avoids false positives for legitimate filenames that contain ".." (e.g., "archive..zip")
func ContainsPathTraversalSegment(path string) bool {
	// Split the path into segments and check if any segment is ".."
	segments := strings.Split(path, string(filepath.Separator))
	return slices.Contains(segments, "..")
}
