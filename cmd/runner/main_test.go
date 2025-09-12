package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
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

	// Initialize all flags - match the original flags from main.go
	configPath = flag.String("config", "", "path to config file")
	envFile = flag.String("env-file", "", "path to environment file")
	logLevel = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	logDir = flag.String("log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	dryRun = flag.Bool("dry-run", false, "print commands without executing them")
	dryRunFormat = flag.String("dry-run-format", "text", "dry-run output format (text, json)")
	dryRunDetail = flag.String("dry-run-detail", "detailed", "dry-run detail level (summary, detailed, full)")
	hashDirectory = flag.String("hash-directory", "", "directory containing hash files (default: "+cmdcommon.DefaultHashDirectory+")")
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

	// Test args without --config
	os.Args = []string{"runner"}

	// Test run() function
	runID := "test-run-id"
	err := run(runID)
	if err == nil {
		t.Error("expected error when --config is not provided")
	}

	// Check if the error is a PreExecutionError with the correct type
	var preExecErr *logging.PreExecutionError
	if !errors.As(err, &preExecErr) {
		t.Errorf("expected PreExecutionError, got: %T", err)
		return
	}

	if preExecErr.Type != logging.ErrorTypeRequiredArgumentMissing {
		t.Errorf("expected ErrorTypeRequiredArgumentMissing, got: %v", preExecErr.Type)
	}
}

func TestGetHashDir(t *testing.T) {
	t.Run("default configuration", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Reset flags to defaults
		os.Args = []string{"runner"}
		flag.Parse()

		assert.Equal(t, cmdcommon.DefaultHashDirectory, getHashDir())
	})

	t.Run("empty hash directory in command line uses default", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Args = []string{"runner"}
		flag.Parse()

		assert.Equal(t, cmdcommon.DefaultHashDirectory, getHashDir(), "should use default hash directory when empty string is provided")
	})

	t.Run("custom hash directory via command line", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Args = []string{"runner", "--hash-directory", "/custom/path"}
		flag.Parse()

		assert.Equal(t, "/custom/path", getHashDir())
	})

	t.Run("command line takes precedence over default", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Args = []string{"runner", "--hash-directory", "/custom/path"}
		flag.Parse()

		assert.Equal(t, "/custom/path", getHashDir())
	})
}

// Define a static error for testing
var errTestUnderlying = errors.New("underlying error")

// TestHashDirectoryError tests the HashDirectoryError type
func TestHashDirectoryError(t *testing.T) {
	t.Run("error creation and behavior", func(t *testing.T) {
		err := &HashDirectoryError{
			Type:  HashDirectoryErrorTypeRelativePath,
			Path:  "relative/path",
			Cause: errTestUnderlying,
		}

		assert.Contains(t, err.Error(), "relative/path")
		assert.True(t, errors.Is(err, errTestUnderlying))
	})
}

// TestGetHashDirectoryWithValidation tests hash directory validation and resolution
func TestGetHashDirectoryWithValidation(t *testing.T) {
	t.Run("command line argument - valid absolute path", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Create temporary directory for testing
		tempDir := t.TempDir()
		os.Args = []string{"runner", "--hash-directory", tempDir}
		flag.Parse()

		result, err := getHashDirectoryWithValidation()
		assert.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	t.Run("command line argument - relative path should fail", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Args = []string{"runner", "--hash-directory", "relative/path"}
		flag.Parse()

		result, err := getHashDirectoryWithValidation()
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, HashDirectoryErrorTypeRelativePath, hashDirErr.Type)
		assert.Equal(t, "relative/path", hashDirErr.Path)
	})

	t.Run("command line argument - non-existent directory should fail", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		nonExistentPath := "/non/existent/directory"
		os.Args = []string{"runner", "--hash-directory", nonExistentPath}
		flag.Parse()

		result, err := getHashDirectoryWithValidation()
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, HashDirectoryErrorTypeNotFound, hashDirErr.Type)
		assert.Equal(t, nonExistentPath, hashDirErr.Path)
	})

	t.Run("environment variable fallback", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Create temporary directory for testing
		tempDir := t.TempDir()
		t.Setenv("HASH_DIRECTORY", tempDir)

		os.Args = []string{"runner"}
		flag.Parse()

		result, err := getHashDirectoryWithValidation()
		assert.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	t.Run("default value fallback", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Args = []string{"runner"}
		flag.Parse()

		// Create default hash directory for testing
		defaultDir := cmdcommon.DefaultHashDirectory
		if !filepath.IsAbs(defaultDir) {
			cwd, _ := os.Getwd()
			defaultDir = filepath.Join(cwd, defaultDir)
		}
		err := os.MkdirAll(defaultDir, 0o755)
		require.NoError(t, err)
		defer os.RemoveAll(defaultDir)

		result, err := getHashDirectoryWithValidation()
		assert.NoError(t, err)
		assert.Equal(t, defaultDir, result)
	})
}

// TestValidateHashDirectorySecurely tests security validation of hash directories
func TestValidateHashDirectorySecurely(t *testing.T) {
	t.Run("valid absolute path", func(t *testing.T) {
		tempDir := t.TempDir()
		result, err := validateHashDirectorySecurely(tempDir)
		assert.NoError(t, err)
		assert.Equal(t, tempDir, result)
	})

	t.Run("relative path should fail", func(t *testing.T) {
		result, err := validateHashDirectorySecurely("relative/path")
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, HashDirectoryErrorTypeRelativePath, hashDirErr.Type)
	})

	t.Run("non-existent directory should fail", func(t *testing.T) {
		nonExistentPath := "/non/existent/directory"
		result, err := validateHashDirectorySecurely(nonExistentPath)
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, HashDirectoryErrorTypeNotFound, hashDirErr.Type)
	})

	t.Run("file instead of directory should fail", func(t *testing.T) {
		tempFile, err := os.CreateTemp("", "test_file")
		require.NoError(t, err)
		defer os.Remove(tempFile.Name())
		tempFile.Close()

		result, err := validateHashDirectorySecurely(tempFile.Name())
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, HashDirectoryErrorTypeNotDirectory, hashDirErr.Type)
	})

	t.Run("symlink attack prevention", func(t *testing.T) {
		// Create a target directory
		targetDir := t.TempDir()

		// Create a symlink pointing to the target
		symlinkPath := filepath.Join(t.TempDir(), "symlink")
		err := os.Symlink(targetDir, symlinkPath)
		require.NoError(t, err)

		result, err := validateHashDirectorySecurely(symlinkPath)
		assert.Error(t, err)
		assert.Empty(t, result)

		var hashDirErr *HashDirectoryError
		require.True(t, errors.As(err, &hashDirErr))
		assert.Equal(t, HashDirectoryErrorTypeSymlinkAttack, hashDirErr.Type)
	})
}
