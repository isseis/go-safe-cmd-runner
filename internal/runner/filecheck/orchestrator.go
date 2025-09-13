// Package filecheck provides high-level file verification orchestration.
package filecheck

import (
	"fmt"
	"log/slog"

	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	runnererrors "github.com/isseis/go-safe-cmd-runner/internal/runner/errors"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// PerformConfigFileVerification verifies configuration file integrity
func PerformConfigFileVerification(verificationManager *verification.Manager, configPath, runID string) error {
	if err := verificationManager.VerifyConfigFile(configPath); err != nil {
		// Create classified error for config verification failure
		classifiedErr := runnererrors.ClassifyVerificationError(
			runnererrors.ErrorTypeConfigVerification,
			runnererrors.ErrorSeverityCritical,
			fmt.Sprintf("Config file verification failed: %s", configPath),
			configPath,
			err,
		)
		runnererrors.LogClassifiedError(classifiedErr)

		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Config verification failed: %v", err),
			Component: "verification",
			RunID:     runID,
		}
	}

	slog.Info("Config file verification completed successfully",
		"config_path", configPath,
		"run_id", runID)
	return nil
}

// PerformEnvironmentFileVerification verifies environment file integrity
func PerformEnvironmentFileVerification(verificationManager *verification.Manager, envFilePath, runID string) error {
	if envFilePath == "" {
		slog.Debug("No environment file specified, skipping verification", "run_id", runID)
		return nil
	}

	if err := verificationManager.VerifyEnvironmentFile(envFilePath); err != nil {
		// Environment file verification failure is CRITICAL - terminate execution for security
		classifiedErr := runnererrors.ClassifyVerificationError(
			runnererrors.ErrorTypeEnvironmentVerification,
			runnererrors.ErrorSeverityCritical,
			fmt.Sprintf("Environment file verification failed: %s", envFilePath),
			envFilePath,
			err,
		)
		runnererrors.LogClassifiedError(classifiedErr)

		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Environment file verification failed: %v", err),
			Component: "verification",
			RunID:     runID,
		}
	}

	slog.Info("Environment file verification completed successfully",
		"env_file", envFilePath,
		"run_id", runID)
	return nil
}

// PerformFileVerification verifies configuration, environment, and global files
func PerformFileVerification(verificationManager *verification.Manager, cfg *runnertypes.Config, configPath, envFileToLoad, runID string) error {
	// Verify configuration file integrity - CRITICAL
	if err := PerformConfigFileVerification(verificationManager, configPath, runID); err != nil {
		return err
	}

	// Verify environment file integrity - CRITICAL
	if err := PerformEnvironmentFileVerification(verificationManager, envFileToLoad, runID); err != nil {
		return err
	}

	// Verify global files - CRITICAL: Program must exit if global verification fails
	result, err := verificationManager.VerifyGlobalFiles(&cfg.Global)
	if err != nil {
		classifiedErr := runnererrors.ClassifyVerificationError(
			runnererrors.ErrorTypeGlobalVerification,
			runnererrors.ErrorSeverityCritical,
			"Global files verification failed - terminating for security",
			"", // No single file path for global verification
			err,
		)
		runnererrors.LogClassifiedError(classifiedErr)

		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Global files verification failed: %v", err),
			Component: "verification",
			RunID:     runID,
		}
	}

	// Log global verification results
	if result.TotalFiles > 0 {
		slog.Info("Global files verification completed successfully",
			"verified", result.VerifiedFiles,
			"skipped", len(result.SkippedFiles),
			"duration_ms", result.Duration.Milliseconds(),
			"run_id", runID)
	}

	return nil
}
