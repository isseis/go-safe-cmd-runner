//go:build test

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// createTempHashDir creates a temporary directory for hash storage during testing
func createTempHashDir() (string, func(), error) {
	tempDir, err := os.MkdirTemp("", "go-safe-cmd-runner-test-")
	if err != nil {
		return "", nil, err
	}

	cleanup := func() {
		os.RemoveAll(tempDir)
	}

	return tempDir, cleanup, nil
}

// runForTestWithTempHashDir is a version that uses a temporary hash directory
func runForTestWithTempHashDir(runID string) error {
	// Create temporary hash directory
	tempHashDir, cleanup, err := createTempHashDir()
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   "Failed to create temporary hash directory",
			Component: "verification",
			RunID:     runID,
		}
	}
	defer cleanup()

	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Phase 1: Initialize verification manager with temporary hash directory
	verificationManager, err := verification.NewManagerForTest(tempHashDir)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   "Verification manager initialization failed",
			Component: "verification",
			RunID:     runID,
		}
	}

	// Phase 2: Verify and load configuration atomically (to prevent TOCTOU attacks)
	cfg, err := bootstrap.LoadConfig(verificationManager, *configPath, runID)
	if err != nil {
		return err
	}

	// The rest of the function follows the same logic as run()
	// Handle validate command (after verification and loading)
	if *validateConfig {
		// Return silently for config validation in tests
		return nil
	}

	// For testing, we skip the actual execution phases
	_ = ctx
	_ = cfg

	return nil
}

// runForTest is a test-only version of run() that uses the default verification manager
// This function should ONLY be called from tests to avoid security issues
func runForTest(runID string) error {
	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Phase 1: Initialize verification manager with default hash directory for testing
	verificationManager, err := verification.NewManagerForTest(cmdcommon.DefaultHashDirectory)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   "Verification manager initialization failed",
			Component: "verification",
			RunID:     runID,
		}
	}

	// Phase 2: Verify and load configuration atomically (to prevent TOCTOU attacks)
	cfg, err := bootstrap.LoadConfig(verificationManager, *configPath, runID)
	if err != nil {
		return err
	}

	// The rest of the function follows the same logic as run()
	// Handle validate command (after verification and loading)
	if *validateConfig {
		// Return silently for config validation in tests
		return nil
	}

	// For testing, we skip the actual execution phases
	_ = ctx
	_ = cfg

	return nil
}

// runForTestWithManager is a helper for testing manager creation
func runForTestWithManager() (error, error) {
	// Test manager creation directly
	_, err := verification.NewManagerForTest(cmdcommon.DefaultHashDirectory)
	if err != nil {
		return nil, err
	}

	// Test the full runForTest flow
	return runForTest("test-run-id"), nil
}

// runForTestWithManagerUsingTempDir is a helper that uses temporary hash directory
func runForTestWithManagerUsingTempDir() (error, error) {
	// Create temporary hash directory
	tempHashDir, cleanup, err := createTempHashDir()
	if err != nil {
		return nil, err
	}
	defer cleanup()

	// Test manager creation directly with temp directory
	_, err = verification.NewManagerForTest(tempHashDir)
	if err != nil {
		return nil, err
	}

	// Test the full runForTestWithTempHashDir flow
	return runForTestWithTempHashDir("test-run-id"), nil
}

// runForTestWithCustomHashDir is a helper for testing custom hash directories
func runForTestWithCustomHashDir(hashDir string) (error, error) {
	// Test manager creation with custom hash directory
	verificationManager, err := verification.NewManagerForTest(hashDir)
	if err != nil {
		return nil, err
	}

	// Try to load config (will fail without config file, but tests manager creation)
	_, configErr := bootstrap.LoadConfig(verificationManager, *configPath, "test-run-id")
	return configErr, nil
}
