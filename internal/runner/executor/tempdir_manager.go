package executor

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

const (
	// tempDirPermissions is the permission mode for temporary directories (owner read/write/execute only)
	tempDirPermissions = 0o700
)

// ErrTempDirAlreadyCreated is returned when Create() is called more than once
// on the same TempDirManager instance.
var ErrTempDirAlreadyCreated = errors.New("temporary directory has already been created for this TempDirManager instance; Create() can only be called once")

// TempDirManager manages the lifecycle of a temporary directory for a group
type TempDirManager interface {
	// Create creates a temporary directory
	// In dry-run mode, returns a virtual path without creating the directory
	Create() (string, error)

	// Cleanup removes the temporary directory
	// In dry-run mode, logs the operation without removing the directory
	Cleanup() error

	// Path returns the path of the temporary directory
	Path() string
}

// DefaultTempDirManager is the default implementation of TempDirManager
type DefaultTempDirManager struct {
	groupName   string
	isDryRun    bool
	tempDirPath string
}

// NewTempDirManager creates a new TempDirManager instance
func NewTempDirManager(groupName string, isDryRun bool) TempDirManager {
	return &DefaultTempDirManager{
		groupName: groupName,
		isDryRun:  isDryRun,
	}
}

// Create creates a temporary directory
func (m *DefaultTempDirManager) Create() (string, error) {
	if m.tempDirPath != "" {
		return "", ErrTempDirAlreadyCreated
	}

	if m.isDryRun {
		// Generate virtual path in dry-run mode
		timestamp := time.Now().Format("20060102150405")
		tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("scr-%s-dryrun-%s", m.groupName, timestamp))
		m.tempDirPath = tempDir
		slog.Info("[DRY-RUN] Would create temporary directory", slog.String("group", m.groupName), slog.String("path", tempDir))
		return tempDir, nil
	}

	// Normal mode: create actual directory
	prefix := fmt.Sprintf("scr-%s-", m.groupName)
	tempDir, err := os.MkdirTemp(os.TempDir(), prefix)
	if err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %w", err)
	}

	// Security: ensure strict 0700 permissions
	// #nosec G302 - 0700 is intentional for temporary working directories to allow execution
	if err := os.Chmod(tempDir, tempDirPermissions); err != nil {
		_ = os.RemoveAll(tempDir) // Best effort cleanup
		return "", fmt.Errorf("failed to set permissions on temporary directory: %w", err)
	}

	m.tempDirPath = tempDir
	slog.Info("Created temporary directory", slog.String("group", m.groupName), slog.String("path", tempDir))
	return tempDir, nil
}

// Cleanup removes the temporary directory
func (m *DefaultTempDirManager) Cleanup() error {
	if m.tempDirPath == "" {
		return nil
	}

	if m.isDryRun {
		// Dry-run mode: log only
		slog.Debug("[DRY-RUN] Would delete temporary directory", slog.String("path", m.tempDirPath))
		m.tempDirPath = ""
		return nil
	}

	// Normal mode: actually remove the directory
	err := os.RemoveAll(m.tempDirPath)
	if err != nil {
		// Return wrapped error; caller decides whether this is fatal
		slog.Error("Failed to cleanup temporary directory", slog.String("path", m.tempDirPath), slog.Any("error", err))
		return fmt.Errorf("failed to cleanup temporary directory '%s': %w", m.tempDirPath, err)
	}

	slog.Debug("Cleaned up temporary directory", slog.String("path", m.tempDirPath))
	m.tempDirPath = ""
	return nil
}

// Path returns the path of the temporary directory
func (m *DefaultTempDirManager) Path() string {
	return m.tempDirPath
}
