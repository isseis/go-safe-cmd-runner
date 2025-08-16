package main

import (
	"errors"
	"flag"
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/stretchr/testify/assert"
)

// setupTestFlags initializes the command-line flags for testing and returns a cleanup function
func setupTestFlags() func() {
	// Save original command line arguments and flag.CommandLine
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine

	// Create new flag set with ExitOnError handling
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Initialize all flags
	configPath = flag.String("config", "", "path to config file")
	envFile = flag.String("env-file", "", "path to environment file")
	logLevel = flag.String("log-level", "", "log level (debug, info, warn, error)")
	dryRun = flag.Bool("dry-run", false, "print commands without executing them")
	hashDirectory = flag.String("hash-directory", cmdcommon.DefaultHashDirectory, "directory containing hash files")

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

	if preExecErr.Type != logging.ErrorTypeInvalidArguments {
		t.Errorf("expected ErrorTypeInvalidArguments, got: %v", preExecErr.Type)
	}
}

func TestGetHashDir(t *testing.T) {
	// Clear environment variables at start
	oldEnvHashDir := os.Getenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")
	defer func() {
		os.Setenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY", oldEnvHashDir)
	}()

	t.Run("default configuration", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Clear environment variables
		os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		// Reset flags to defaults
		os.Args = []string{"runner"}
		flag.Parse()

		assert.Equal(t, cmdcommon.DefaultHashDirectory, getHashDir())
	})

	t.Run("empty hash directory in environment uses default", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Setenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY", "")
		defer os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner"}
		flag.Parse()

		assert.Equal(t, cmdcommon.DefaultHashDirectory, getHashDir(), "should use default hash directory when empty string is provided")
	})

	t.Run("custom hash directory via command line", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Clear environment variables
		os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner", "--hash-directory", "/custom/path"}
		flag.Parse()

		assert.Equal(t, "/custom/path", getHashDir())
	})

	t.Run("custom hash directory via environment variable", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		// Clear command line flags
		hashDirectory = new(string)
		os.Setenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY", "/env/path")
		defer os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner"}
		flag.Parse()

		assert.Equal(t, "/env/path", getHashDir())
	})

	t.Run("command line takes precedence over environment", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Setenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY", "/env/path")
		defer os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner", "--hash-directory", "/custom/path"}
		flag.Parse()

		assert.Equal(t, "/custom/path", getHashDir())
	})

	t.Run("empty command line hash directory uses environment", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Setenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY", "/env/path")
		defer os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner", "--hash-directory="} // Empty value
		flag.Parse()

		assert.Equal(t, "/env/path", getHashDir(), "should use environment variable when command line value is empty")
	})

	t.Run("empty environment hash directory uses default", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Setenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY", "")
		defer os.Unsetenv("GO_SAFE_CMD_RUNNER_HASH_DIRECTORY")

		os.Args = []string{"runner"}
		flag.Parse()

		assert.Equal(t, cmdcommon.DefaultHashDirectory, getHashDir(), "should use default when environment variable is empty")
	})
}
