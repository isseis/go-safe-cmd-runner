// Package logging provides safe file operations and utilities for the logging framework.
package logging

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/oklog/ulid/v2"
)

// Common errors
var (
	ErrEmptyLogDirectory = errors.New("log directory cannot be empty")

	// File permissions constants
	logDirPerm  os.FileMode = 0o750
	logFilePerm os.FileMode = 0o600
)

// SafeFileOpener handles safe file operations with symlink protection
type SafeFileOpener struct {
	fs safefileio.FileSystem
}

// NewSafeFileOpener creates a new SafeFileOpener using the safefileio package
func NewSafeFileOpener() *SafeFileOpener {
	return &SafeFileOpener{
		fs: safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
	}
}

// OpenFile safely opens a file using the existing safefileio package
// Returns an io.WriteCloser that can be used with slog.NewJSONHandler
func (s *SafeFileOpener) OpenFile(path string, flag int, perm os.FileMode) (io.WriteCloser, error) {
	// Ensure the directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, logDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Use the existing safefileio package for secure file operations
	file, err := s.fs.SafeOpenFile(path, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s safely: %w", path, err)
	}

	return file, nil
}

// GenerateRunID generates a new ULID for run identification
func GenerateRunID() string {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
}

// ValidateLogDir ensures the log directory is safe and accessible
func ValidateLogDir(dir string) error {
	if dir == "" {
		return ErrEmptyLogDirectory
	}

	// Check if directory exists or can be created
	if err := os.MkdirAll(dir, logDirPerm); err != nil {
		return fmt.Errorf("cannot create log directory %s: %w", dir, err)
	}

	// Check if we can write to the directory using safefileio
	testFile := filepath.Join(dir, ".write_test")
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	f, err := fs.SafeOpenFile(testFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, logFilePerm)
	if err != nil {
		return fmt.Errorf("cannot write to log directory %s: %w", dir, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close test file: %w", err)
	}
	if err := os.Remove(testFile); err != nil {
		return fmt.Errorf("failed to remove test file: %w", err)
	}

	return nil
}
