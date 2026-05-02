package security

import (
	"errors"
	"fmt"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

// validateFileHash verifies cmdPath against a SHA256 hash stored under hashDir.
// The caller is responsible for ensuring hashDir is non-empty before calling.
func validateFileHash(cmdPath string, hashDir string) error {
	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir)
	if err != nil {
		return fmt.Errorf("hash validation failed to initialize validator: %w", err)
	}

	if err := validator.Verify(cmdPath); err != nil {
		if isHashFileNotFound(err) {
			return fmt.Errorf("%w: no hash recorded for file: %s", ErrHashValidationFailed, cmdPath)
		}
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
