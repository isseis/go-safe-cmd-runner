package bootstrap

import (
	"fmt"
	"log/slog"

	runnererrors "github.com/isseis/go-safe-cmd-runner/internal/runner/errors"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// InitializeVerificationManager creates and configures the verification manager
// with an already validated hash directory path
func InitializeVerificationManager(validatedHashDir, runID string) (*verification.Manager, error) {
	slog.Info("Initializing verification manager with validated hash directory",
		"hash_directory", validatedHashDir,
		"run_id", runID)

	// SECURITY: Remove runtime testing detection to prevent security bypass
	// Testing should use build tags or separate test functions instead

	// Note: In production, we always use the default hash directory
	// The validatedHashDir parameter is ignored for security
	verificationManager, err := verification.NewManager()
	if err != nil {
		classifiedErr := runnererrors.ClassifyVerificationError(
			runnererrors.ErrorTypeHashDirectoryValidation,
			runnererrors.ErrorSeverityCritical,
			"Failed to initialize verification manager",
			validatedHashDir,
			err,
		)
		runnererrors.LogClassifiedError(classifiedErr)
		return nil, fmt.Errorf("failed to initialize verification: %w", err)
	}

	return verificationManager, nil
}
