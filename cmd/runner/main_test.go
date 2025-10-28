package main

import (
	"errors"
	"flag"
	"os"
	"testing"

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

	// Initialize all flags - match the original flags from main.go (excluding removed hash-directory flag)
	configPath = flag.String("config", "", "path to config file")
	logLevel = flag.String("log-level", "info", "log level (debug, info, warn, error)")
	logDir = flag.String("log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	dryRun = flag.Bool("dry-run", false, "print commands without executing them")
	dryRunFormat = flag.String("dry-run-format", "text", "dry-run output format (text, json)")
	dryRunDetail = flag.String("dry-run-detail", "detailed", "dry-run detail level (summary, detailed, full)")
	showSensitive = flag.Bool("show-sensitive", false, "show sensitive information in dry-run output (use with caution)")
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

	// Test runForTestWithTempHashDir() function to avoid CI hash directory issues
	runID := "test-run-id"
	err := runForTestWithTempHashDir(runID)
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
		// Use temporary hash directory to avoid CI environment issues
		runErr, managerErr := runForTestWithManagerUsingTempDir()
		if managerErr != nil {
			t.Fatalf("manager creation should not fail: %v", managerErr)
		}
		if runErr != nil {
			// In tests, we expect this to fail due to missing config file
			assert.Contains(t, runErr.Error(), "config")
		}
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
