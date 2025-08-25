package security

import (
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

// shouldSkipHashValidation determines whether to skip hash validation
func shouldSkipHashValidation(cmdPath string, skipStandardPaths bool) bool {
	if !skipStandardPaths {
		return false // Validate all files when SkipStandardPaths=false
	}

	return isStandardDirectory(cmdPath) // Skip only standard directories
}

// validateFileHash performs file hash validation using provided validator
func validateFileHash(cmdPath string, hashDir string) error {
	// Fallback to creating validator (for backward compatibility)
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	if err != nil {
		return fmt.Errorf("hash validation failed to initialize validator: %w", err)
	}

	// Perform hash validation using filevalidator
	if err := validator.Verify(cmdPath); err != nil {
		// Check if error is due to missing hash file (not necessarily a failure)
		if isHashFileNotFound(err) {
			// Hash file not found. The current security policy is to treat this as
			// a validation failure.
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
