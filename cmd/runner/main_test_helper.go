//go:build test

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

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
		_ = os.RemoveAll(tempDir) // Ignore cleanup errors in test helper
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

	// Phase 2: Load and prepare configuration (verify, parse, and expand variables)
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

	// For testing, we skip the actual execution phases
	_ = ctx
	_ = cfg

	return nil
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

	// Try to load and prepare config (will fail without config file, but tests manager creation)
	_, configErr := bootstrap.LoadAndPrepareConfig(verificationManager, *configPath, "test-run-id")
	return configErr, nil
}
