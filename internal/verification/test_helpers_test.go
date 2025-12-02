//go:build test

package verification

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// setupLogCapture configures a logger that captures log output to a buffer
// and returns the buffer and a cleanup function
func setupLogCapture(t *testing.T) (*strings.Builder, func()) {
	t.Helper()
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	originalLogger := slog.Default()
	slog.SetDefault(logger)
	return &logBuffer, func() { slog.SetDefault(originalLogger) }
}

// setupHashDir creates a hash directory and returns its path
func setupHashDir(t *testing.T, tmpDir string) string {
	t.Helper()
	hashDir := filepath.Join(tmpDir, "hashes")
	err := os.MkdirAll(hashDir, 0o755) // #nosec G301 -- Test code: directory permissions are appropriate for test environment
	require.NoError(t, err)
	return hashDir
}

// createTestFile creates a test file with the given content and returns its path
func createTestFile(t *testing.T, dir, filename string, content []byte) string {
	t.Helper()
	filePath := filepath.Join(dir, filename)
	err := os.WriteFile(filePath, content, 0o644) // #nosec G306 -- Test code: file permissions are appropriate for test environment
	require.NoError(t, err)
	return filePath
}

// createDryRunManager creates a manager configured for dry-run mode testing
func createDryRunManager(t *testing.T, hashDir string) *Manager {
	t.Helper()
	manager, err := NewManagerForTest(hashDir,
		WithDryRunMode(),
		WithSkipHashDirectoryValidation())
	require.NoError(t, err)
	return manager
}

// setupDryRunTest sets up a complete dry-run test environment
// Returns: tmpDir, hashDir, logBuffer, cleanup function
func setupDryRunTest(t *testing.T) (string, string, *strings.Builder, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	hashDir := setupHashDir(t, tmpDir)
	logBuffer, cleanupLog := setupLogCapture(t)
	return tmpDir, hashDir, logBuffer, cleanupLog
}
