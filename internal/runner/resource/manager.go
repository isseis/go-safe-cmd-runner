// Package resource provides functionality for managing resources like
// temporary directories, file cleanup, and resource lifecycle management.
package resource

import (
	"errors"
	"fmt"
	"os"
	"sync"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// Error definitions for the resource package
var (
	// ErrTempDirNotFound is returned when a requested temporary directory is not found
	ErrTempDirNotFound = errors.New("temporary directory not found")
	// ErrCleanupFailed is returned when temporary directory cleanup fails
	ErrCleanupFailed = errors.New("temporary directory cleanup failed")
)

// Manager handles temporary directory lifecycle management
type Manager struct {
	tempDirs map[string]bool // directory path -> managed flag
	mu       sync.RWMutex
	baseDir  string
	fs       common.FileSystem
}

// NewManager creates a new resource manager
func NewManager(baseDir string) *Manager {
	return NewManagerWithFS(baseDir, common.NewDefaultFileSystem())
}

// NewManagerWithFS creates a new resource manager with a custom FileSystem
func NewManagerWithFS(baseDir string, fs common.FileSystem) *Manager {
	if baseDir == "" {
		baseDir = os.TempDir()
	}
	return &Manager{
		tempDirs: make(map[string]bool),
		baseDir:  baseDir,
		fs:       fs,
	}
}

// CreateTempDir creates a temporary directory for a command and returns its path
func (m *Manager) CreateTempDir(commandName string) (string, error) {
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
func (m *Manager) CleanupTempDir(path string) error {
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
func (m *Manager) CleanupAll() error {
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
		return fmt.Errorf("%w: %d temp dirs failed to cleanup", ErrCleanupFailed, len(errs))
	}

	return nil
}

// IsTempDirManaged checks if a given path is managed by this manager
func (m *Manager) IsTempDirManaged(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.tempDirs[path]
}
