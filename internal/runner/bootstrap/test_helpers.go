//go:build test

package bootstrap

import (
	"fmt"
	"log/slog"

	runnererrors "github.com/isseis/go-safe-cmd-runner/internal/runner/runerrors"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// InitializeVerificationManagerForTest creates and configures a verification manager for testing
// with a custom hash directory path. This function should ONLY be used in tests.
// WARNING: This function bypasses production security constraints and should never be used
// in production code.
func InitializeVerificationManagerForTest(validatedHashDir string) (*verification.Manager, error) {
	slog.Info("Initializing test verification manager with custom hash directory",
		"hash_directory", validatedHashDir)

	// Use the verified hash directory for testing
	verificationManager, err := verification.NewManagerForTest(validatedHashDir)
	if err != nil {
		classifiedErr := runnererrors.ClassifyVerificationError(
			runnererrors.ErrorTypeHashDirectoryValidation,
			runnererrors.ErrorSeverityCritical,
			"Failed to initialize test verification manager",
			validatedHashDir,
			err,
		)
		runnererrors.LogClassifiedError(classifiedErr)
		return nil, fmt.Errorf("failed to initialize verification: %w", err)
	}

	return verificationManager, nil
}
