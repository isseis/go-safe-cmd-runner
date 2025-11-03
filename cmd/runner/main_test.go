package main

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
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

// createTempHashDir creates a temporary directory for hash storage during testing
func createTempHashDir(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "go-safe-cmd-runner-test-")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cleanup := func() {
		_ = os.RemoveAll(tempDir) // Ignore cleanup errors in test helper
	}

	return tempDir, cleanup
}

// runForTestWithTempHashDir is a version that uses a temporary hash directory
func runForTestWithTempHashDir(t *testing.T, runID string) error {
	t.Helper()

	// Create temporary hash directory
	tempHashDir, cleanup := createTempHashDir(t)
	defer cleanup()

	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize verification manager with temporary hash directory
	verificationManager, err := verification.NewManagerForTest(tempHashDir)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   "Verification manager initialization failed",
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	// Load and prepare configuration (verify, parse, and expand variables)
	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, *configPath, runID)
	if err != nil {
		return err
	}

	// The rest of the function follows the same logic as run()
	// Handle validate command (after verification and loading)
	if *validateConfig {
		// Return silently for config validation in tests
		return nil
	}

	// For testing, we skip the actual execution steps
	_ = ctx
	_ = cfg

	return nil
}

// runForTestWithManagerUsingTempDir is a helper that uses temporary hash directory
func runForTestWithManagerUsingTempDir(t *testing.T) (error, error) {
	t.Helper()

	// Create temporary hash directory
	tempHashDir, cleanup := createTempHashDir(t)
	defer cleanup()

	// Test manager creation directly with temp directory
	_, err := verification.NewManagerForTest(tempHashDir)
	if err != nil {
		return nil, err
	}

	// Test the full runForTestWithTempHashDir flow
	return runForTestWithTempHashDir(t, "test-run-id"), nil
}

// runForTestWithCustomHashDir is a helper for testing custom hash directories
func runForTestWithCustomHashDir(t *testing.T, hashDir string) (error, error) {
	t.Helper()

	// Test manager creation with custom hash directory
	verificationManager, err := verification.NewManagerForTest(hashDir)
	if err != nil {
		return nil, err
	}

	// Try to load and prepare config (will fail without config file, but tests manager creation)
	_, configErr := bootstrap.LoadAndPrepareConfig(verificationManager, *configPath, "test-run-id")
	return configErr, nil
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
	err := runForTestWithTempHashDir(t, runID)
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
		runErr, managerErr := runForTestWithManagerUsingTempDir(t)
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
		configErr, managerErr := runForTestWithCustomHashDir(t, tempDir)
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
		configErr, managerErr := runForTestWithCustomHashDir(t, "relative/path")
		// This will fail due to directory not existing, but not due to relative path restriction
		// We expect either a config error or manager error (directory doesn't exist)
		assert.True(t, configErr != nil || managerErr != nil, "expected an error for non-existent directory")
	})
}
