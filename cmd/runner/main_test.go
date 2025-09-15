//go:build test

package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/hashdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestFlags initializes the command-line flags for testing and returns a cleanup function
func setupTestFlags() func() {
	// Save original command line arguments and flag.CommandLine
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine

	// Create new flag set with ExitOnError handling
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Initialize all flags - match the original flags from main.go (excluding removed hash-directory flag)
	configPath = flag.String("config", "", "path to config file")
	logLevel = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	logDir = flag.String("log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	dryRun = flag.Bool("dry-run", false, "print commands without executing them")
	dryRunFormat = flag.String("dry-run-format", "text", "dry-run output format (text, json)")
	dryRunDetail = flag.String("dry-run-detail", "detailed", "dry-run detail level (summary, detailed, full)")
	validateConfig = flag.Bool("validate", false, "validate configuration file and exit")
	runID = flag.String("run-id", "", "unique identifier for this execution run (auto-generates ULID if not provided)")
	forceInteractive = flag.Bool("interactive", false, "force interactive mode with colored output (overrides environment detection)")
	forceQuiet = flag.Bool("quiet", false, "force non-interactive mode (disables colored output)")

	// Return cleanup function to restore original state
	return func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}
}

func TestConfigPathRequired(t *testing.T) {
	// Setup test flags
	cleanup := setupTestFlags()
	defer cleanup()

	// Test args without --config (hash directory is now set automatically to default)
	os.Args = []string{"runner"}

	// Parse flags
	flag.Parse()

	// Test runForTest() function (test-specific version)
	runID := "test-run-id"
	err := runForTest(runID)
	if err == nil {
		t.Error("expected error when --config is not provided")
	}

	// Check if the error is a PreExecutionError with the correct type
	var preExecErr *logging.PreExecutionError
	if !errors.As(err, &preExecErr) {
		t.Errorf("expected PreExecutionError, got: %T (error: %v)", err, err)
		return
	}

	if preExecErr.Type != logging.ErrorTypeRequiredArgumentMissing {
		t.Errorf("expected ErrorTypeRequiredArgumentMissing, got: %v (message: %s)", preExecErr.Type, preExecErr.Message)
	}
}

func TestNewManagerProduction(t *testing.T) {
	t.Run("creates manager with default hash directory", func(t *testing.T) {
		// Production manager creation should use default hash directory
		runErr, managerErr := runForTestWithManager()
		if managerErr != nil {
			t.Fatalf("manager creation should not fail: %v", managerErr)
		}
		if runErr != nil {
			// In tests, we expect this to fail due to missing config file
			assert.Contains(t, runErr.Error(), "config")
		}
	})
}

// Define a static error for testing
var errTestUnderlying = errors.New("underlying error")

// TestHashDirectoryError tests the hashdir.HashDirectoryError type
func TestHashDirectoryError(t *testing.T) {
	t.Run("error creation and behavior", func(t *testing.T) {
		err := &hashdir.HashDirectoryError{
			Type:  hashdir.HashDirectoryErrorTypeRelativePath,
			Path:  "relative/path",
			Cause: errTestUnderlying,
		}

		assert.Contains(t, err.Error(), "relative/path")
		assert.True(t, errors.Is(err, errTestUnderlying))
	})
}

// TestNewManagerForTestValidation tests the testing API validation
func TestNewManagerForTestValidation(t *testing.T) {
	t.Run("valid custom hash directory", func(t *testing.T) {
		// Create temporary directory for testing
		tempDir := t.TempDir()

		// This should work since we're in a test file
		configErr, managerErr := runForTestWithCustomHashDir(tempDir)
		if managerErr != nil {
			t.Fatalf("manager creation should not fail: %v", managerErr)
		}
		if configErr != nil {
			// We expect config errors, not manager creation errors
			assert.Contains(t, configErr.Error(), "config")
		}
	})

	t.Run("relative path allowed in testing", func(t *testing.T) {
		// Custom hash directories (even relative ones) are allowed in testing mode
		configErr, managerErr := runForTestWithCustomHashDir("relative/path")
		// This will fail due to directory not existing, but not due to relative path restriction
		// We expect either a config error or manager error (directory doesn't exist)
		assert.True(t, configErr != nil || managerErr != nil, "expected an error for non-existent directory")
	})
}

// TestValidateHashDirectorySecurely tests security validation of hash directories
func TestValidateHashDirectorySecurely(t *testing.T) {
	t.Run("valid absolute path", func(t *testing.T) {
		tempDir := t.TempDir()
		result, err := hashdir.ValidateSecurely(tempDir)
		assert.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	t.Run("relative path should fail", func(t *testing.T) {
		result, err := hashdir.ValidateSecurely("relative/path")
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *hashdir.HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, hashdir.HashDirectoryErrorTypeRelativePath, hashDirErr.Type)
	})

	t.Run("non-existent directory should fail", func(t *testing.T) {
		nonExistentPath := "/non/existent/directory"
		result, err := hashdir.ValidateSecurely(nonExistentPath)
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *hashdir.HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, hashdir.HashDirectoryErrorTypeNotFound, hashDirErr.Type)
	})

	t.Run("file instead of directory should fail", func(t *testing.T) {
		tempFile, err := os.CreateTemp("", "test_file")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())
		tempFile.Close()

		result, err := hashdir.ValidateSecurely(tempFile.Name())
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *hashdir.HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, hashdir.HashDirectoryErrorTypeNotDirectory, hashDirErr.Type)
	})

	t.Run("symlink attack prevention", func(t *testing.T) {
		// Create a target directory
		targetDir := t.TempDir()

		// Create a symlink pointing to the target
		symlinkPath := filepath.Join(t.TempDir(), "symlink")
		err := os.Symlink(targetDir, symlinkPath)
		require.NoError(t, err)

		result, err := hashdir.ValidateSecurely(symlinkPath)
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *hashdir.HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, hashdir.HashDirectoryErrorTypeSymlinkAttack, hashDirErr.Type)
	})
}
