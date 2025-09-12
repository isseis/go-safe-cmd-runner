package bootstrap

import (
	"fmt"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	runnererrors "github.com/isseis/go-safe-cmd-runner/internal/runner/errors"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/hashdir"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// InitializeVerificationManager creates and configures the verification manager
func InitializeVerificationManager(hashDirectory *string, defaultHashDirectory, runID string) (*verification.Manager, error) {
	// Use secure hash directory validation with priority-based resolution
	hashDir, err := hashdir.GetWithValidation(hashDirectory, defaultHashDirectory)
	if err != nil {
		// Extract the failed path from HashDirectoryError for better logging
		failedPath := hashDir // fallback to returned hashDir
		if hashDirErr, ok := err.(*hashdir.HashDirectoryError); ok {
			failedPath = hashDirErr.Path
		}

		classifiedErr := runnererrors.ClassifyVerificationError(
			runnererrors.ErrorTypeHashDirectoryValidation,
			runnererrors.ErrorSeverityCritical,
			"Hash directory validation failed",
			failedPath,
			err,
		)
		runnererrors.LogClassifiedError(classifiedErr)

		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Hash directory validation failed: %v", err),
			Component: "file",
			RunID:     runID,
		}
	}

	slog.Info("Hash directory validation completed successfully",
		"hash_directory", hashDir,
		"run_id", runID)

	// Initialize privilege manager
	logger := slog.Default()
	privMgr := privilege.NewManager(logger)

	// Initialize verification manager with privilege support
	verificationManager, err := verification.NewManagerWithOpts(
		hashDir,
		verification.WithPrivilegeManager(privMgr),
	)
	if err != nil {
		classifiedErr := runnererrors.ClassifyVerificationError(
			runnererrors.ErrorTypeHashDirectoryValidation,
			runnererrors.ErrorSeverityCritical,
			"Failed to initialize verification manager",
			hashDir, // hashDir is valid here since GetWithValidation succeeded
			err,
		)
		runnererrors.LogClassifiedError(classifiedErr)
		return nil, fmt.Errorf("failed to initialize verification: %w", err)
	}

	return verificationManager, nil
}
