package security

import (
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// shouldSkipHashValidation determines whether to skip hash validation
func shouldSkipHashValidation(cmdPath string, globalConfig *runnertypes.GlobalConfig) bool {
	if globalConfig == nil {
		return false // Validate all files when no config provided
	}

	if !globalConfig.SkipStandardPaths {
		return false // Validate all files when SkipStandardPaths=false
	}

	return isStandardDirectory(cmdPath) // Skip only standard directories
}

// validateFileHash performs file hash validation using filevalidator
func validateFileHash(cmdPath string, hashDir string) error {
	if hashDir == "" {
		// If no hash directory provided, skip validation
		return nil
	}

	// Create filevalidator instance
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	if err != nil {
		return fmt.Errorf("%w: failed to create file validator: %v", ErrHashValidationFailed, err)
	}

	// Perform hash validation using filevalidator
	if err := validator.Verify(cmdPath); err != nil {
		// Check if error is due to missing hash file (not necessarily a failure)
		if isHashFileNotFound(err) {
			// Hash file not found - this might be acceptable depending on policy
			// For now, we treat this as validation failure
			return fmt.Errorf("%w: no hash recorded for file: %s", ErrHashValidationFailed, cmdPath)
		}
		// Hash validation failed
		return fmt.Errorf("%w: %v", ErrHashValidationFailed, err)
	}

	return nil
}

// isHashFileNotFound checks if the error indicates a missing hash file
func isHashFileNotFound(err error) bool {
	if err == nil {
		return false
	}
	// Check for specific filevalidator errors indicating missing hash files
	return errors.Is(err, filevalidator.ErrHashFileNotFound)
}
