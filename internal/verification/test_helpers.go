//go:build test

package verification

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/require"
)

// TestOption is a function type for configuring Manager instances for testing
type TestOption func(*managerInternalOptions)

// WithFS sets a custom file system for testing
func WithFS(fs common.FileSystem) TestOption {
	return func(opts *managerInternalOptions) {
		opts.fs = fs
	}
}

// WithFileValidatorDisabled disables file validation for testing
func WithFileValidatorDisabled() TestOption {
	return func(opts *managerInternalOptions) {
		opts.fileValidatorEnabled = false
	}
}

// WithFileValidatorEnabled enables file validation for testing
func WithFileValidatorEnabled() TestOption {
	return func(opts *managerInternalOptions) {
		opts.fileValidatorEnabled = true
	}
}

// WithTestingSecurityLevel sets the security level to relaxed for testing
func WithTestingSecurityLevel() TestOption {
	return func(opts *managerInternalOptions) {
		opts.securityLevel = SecurityLevelRelaxed
	}
}

// WithSkipHashDirectoryValidation skips hash directory validation for testing
func WithSkipHashDirectoryValidation() TestOption {
	return func(opts *managerInternalOptions) {
		opts.skipHashDirectoryValidation = true
	}
}

// WithPathResolver sets a custom path resolver for testing
func WithPathResolver(pathResolver *PathResolver) TestOption {
	return func(opts *managerInternalOptions) {
		opts.customPathResolver = pathResolver
	}
}

// WithDryRunMode enables dry-run mode for testing
func WithDryRunMode() TestOption {
	return func(opts *managerInternalOptions) {
		opts.isDryRun = true
	}
}

// NewManagerForTest creates a new verification manager for testing with a custom hash directory
// This API allows custom hash directories for testing purposes and uses relaxed security constraints
func NewManagerForTest(hashDir string, options ...TestOption) (*Manager, error) {
	// Log testing manager creation for audit trail
	slog.Info("Testing verification manager created",
		"api", "NewManagerForTest",
		"hash_directory", hashDir,
		"security_level", "relaxed")

	// Start with default testing options
	internalOpts := newInternalOptions()
	internalOpts.creationMode = CreationModeTesting
	internalOpts.securityLevel = SecurityLevelRelaxed
	internalOpts.skipHashDirectoryValidation = true
	// Keep fileValidatorEnabled = true by default for proper testing

	// Apply user-provided options
	for _, opt := range options {
		opt(internalOpts)
	}

	// Convert to InternalOption array
	internalOptions := []InternalOption{
		withCreationMode(internalOpts.creationMode),
		withSecurityLevel(internalOpts.securityLevel),
	}

	if internalOpts.skipHashDirectoryValidation {
		internalOptions = append(internalOptions, withSkipHashDirectoryValidationInternal())
	}

	if !internalOpts.fileValidatorEnabled {
		internalOptions = append(internalOptions, withFileValidatorDisabledInternal())
	}

	if internalOpts.fs != nil {
		internalOptions = append(internalOptions, withFSInternal(internalOpts.fs))
	}

	if internalOpts.customPathResolver != nil {
		internalOptions = append(internalOptions, withCustomPathResolverInternal(internalOpts.customPathResolver))
	}

	if internalOpts.isDryRun {
		internalOptions = append(internalOptions, withDryRunModeInternal())
	}

	// Create manager with testing constraints (allows custom hash directory)
	return newManagerInternal(hashDir, internalOptions...)
}

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
