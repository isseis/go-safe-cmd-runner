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
