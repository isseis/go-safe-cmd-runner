// Package common provides shared interfaces and utilities used across the runner packages.

//nolint:revive // common is an appropriate name for shared utilities package
package common

import (
	"io/fs"
	"os"
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
