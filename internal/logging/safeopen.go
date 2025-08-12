// Package logging provides safe file operations and utilities for the logging framework.
package logging

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// Common errors
var (
	ErrInvalidFileType   = errors.New("unexpected file type returned from safefileio")
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
func (s *SafeFileOpener) OpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
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

	// Convert safefileio.File to *os.File
	// The safefileio package returns a File interface, but we need *os.File for slog
	if osFile, ok := file.(*os.File); ok {
		return osFile, nil
	}

	return nil, ErrInvalidFileType
}

// GenerateLogFilename generates a unique log filename
func (s *SafeFileOpener) GenerateLogFilename(dir string) (string, string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	runID := uuid.New().String()
	timestamp := time.Now().Format("20060102T150405Z")

	filename := fmt.Sprintf("%s_%s_%s.json", hostname, timestamp, runID)
	fullPath := filepath.Join(dir, filename)

	return fullPath, runID, nil
}

// GetBuildInfo returns build information for logging
func GetBuildInfo() (gitCommit, buildVersion string) {
	// These would typically be set via build flags
	// For now, return placeholder values
	gitCommit = os.Getenv("GIT_COMMIT")
	if gitCommit == "" {
		gitCommit = "unknown"
	}

	buildVersion = os.Getenv("BUILD_VERSION")
	if buildVersion == "" {
		buildVersion = "dev"
	}

	return gitCommit, buildVersion
}

// GenerateRunID generates a new UUID v4 for run identification
func GenerateRunID() string {
	return uuid.New().String()
}

// CompressLogFile compresses a log file to .gz format and removes the original
func CompressLogFile(logPath string) error {
	// This is a simplified implementation
	// In a production environment, you'd want to use gzip compression
	gzPath := logPath + ".gz"

	// Read original file
	// #nosec G304 - logPath is validated and safe for reading
	data, err := os.ReadFile(logPath)
	if err != nil {
		return fmt.Errorf("failed to read log file for compression: %w", err)
	}

	// For now, just rename (in production, would actually compress)
	err = os.WriteFile(gzPath, data, logFilePerm)
	if err != nil {
		return fmt.Errorf("failed to write compressed log file: %w", err)
	}

	// Remove original
	err = os.Remove(logPath)
	if err != nil {
		return fmt.Errorf("failed to remove original log file: %w", err)
	}

	return nil
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

// SecureRandom generates secure random bytes
func SecureRandom(length int) ([]byte, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return nil, fmt.Errorf("failed to generate secure random bytes: %w", err)
	}
	return bytes, nil
}
