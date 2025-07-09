// Package common provides shared interfaces and utilities used across the runner packages.
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
	CreateTempDir(prefix string) (string, error)

	// MkdirAll creates a directory path recursively
	MkdirAll(path string, perm os.FileMode) error

	// RemoveAll removes a directory and all its contents
	RemoveAll(path string) error

	// Remove removes a single file or empty directory
	Remove(path string) error

	// Stat returns file information
	Stat(path string) (fs.FileInfo, error)

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
func (fs *DefaultFileSystem) CreateTempDir(prefix string) (string, error) {
	return os.MkdirTemp("", prefix)
}

// MkdirAll creates a directory path recursively
func (fs *DefaultFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// RemoveAll removes a directory and all its contents
func (fs *DefaultFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Remove removes a single file or empty directory
func (fs *DefaultFileSystem) Remove(path string) error {
	return os.Remove(path)
}

// Stat returns file information
func (fs *DefaultFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

// FileExists checks if a file or directory exists
func (fs *DefaultFileSystem) FileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// IsDir checks if the path is a directory
func (fs *DefaultFileSystem) IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}