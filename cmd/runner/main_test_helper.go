//go:build test

package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/hashdir"
)

// runForTest is a test-only version of run() that uses custom hash directories
// This function should ONLY be called from tests to avoid security issues
func runForTest(runID string) error {
	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Phase 1: Get validated hash directory (using secure validation)
	validatedHashDir, err := hashdir.GetWithValidation(hashDirectory, cmdcommon.DefaultHashDirectory)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   "Hash directory validation failed",
			Component: "file",
			RunID:     runID,
			Err:       err,
		}
	}

	// Phase 2: Initialize verification manager with validated hash directory FOR TESTING
	verificationManager, err := bootstrap.InitializeVerificationManagerForTest(validatedHashDir)
	if err != nil {
		return err
	}

	// Phase 3: Verify and load configuration atomically (to prevent TOCTOU attacks)
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
