// Package tempdir provides functionality for managing temporary directories
// including creation, cleanup, and lifecycle management.
package tempdir

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// Error definitions for the tempdir package
var (
	// ErrTempDirNotFound is returned when a requested temporary directory is not found
	ErrTempDirNotFound = errors.New("temporary directory not found")
	// ErrCleanupFailed is returned when temporary directory cleanup fails
	ErrCleanupFailed = errors.New("temporary directory cleanup failed")
)

// TempDirManager handles temporary directory lifecycle management
//
//nolint:revive // TempDirManager is intentionally explicit to avoid confusion with other managers
type TempDirManager struct {
	tempDirs map[string]bool // directory path -> managed flag
	mu       sync.RWMutex
	baseDir  string
	fs       common.FileSystem
}

// NewTempDirManager creates a new temporary directory manager
func NewTempDirManager(baseDir string) *TempDirManager {
	return NewTempDirManagerWithFS(baseDir, common.NewDefaultFileSystem())
}

// NewTempDirManagerWithFS creates a new temporary directory manager with a custom FileSystem
func NewTempDirManagerWithFS(baseDir string, fs common.FileSystem) *TempDirManager {
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	return &TempDirManager{
		tempDirs: make(map[string]bool),
		baseDir:  baseDir,
		fs:       fs,
	}
}

// CreateTempDir creates a temporary directory for a command and returns its path
func (m *TempDirManager) CreateTempDir(commandName string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create the directory with a meaningful prefix
	prefix := fmt.Sprintf("tempdir_%s_", commandName)
	tempDirPath, err := m.fs.CreateTempDir(m.baseDir, prefix)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Store the path as managed
	m.tempDirs[tempDirPath] = true
	return tempDirPath, nil
}

// CleanupTempDir cleans up a specific temporary directory
func (m *TempDirManager) CleanupTempDir(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.tempDirs[path] {
		return fmt.Errorf("%w: %s", ErrTempDirNotFound, path)
	}

	err := m.fs.RemoveAll(path)
	if err != nil {
		return fmt.Errorf("%w: failed to cleanup temp dir %s: %v", ErrCleanupFailed, path, err)
	}

	delete(m.tempDirs, path)
	return nil
}

// CleanupAll cleans up all managed temporary directories
func (m *TempDirManager) CleanupAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error
	for path := range m.tempDirs {
		err := m.fs.RemoveAll(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to cleanup %s: %w", path, err))
		} else {
			delete(m.tempDirs, path)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("%w: %w", ErrCleanupFailed, errors.Join(errs...))
	}
	return nil
}
