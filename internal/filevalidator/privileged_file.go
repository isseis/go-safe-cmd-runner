package filevalidator

import (
	"errors"
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them
// This function uses the existing privilege management infrastructure
func OpenFileWithPrivileges(filepath string, privManager runnertypes.PrivilegeManager) (*os.File, error) {
	// Attempt normal access with standard privileges first
	file, err := os.Open(filepath) //nolint:gosec // filepath is validated by caller
	if err == nil {
		return file, nil
	}

	// If the error is not a permission error, privilege escalation won't resolve it
	if !os.IsPermission(err) {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, err)
	}

	// Return an error if PrivilegeManager is not provided
	if privManager == nil {
		return nil, fmt.Errorf("failed to open file %s: %w", filepath, fmt.Errorf("%w: %w", runnertypes.ErrPrivilegedExecutionNotAvailable, err))
	}

	// Check if privilege escalation is supported
	if !privManager.IsPrivilegedExecutionSupported() {
		return nil, fmt.Errorf("privileged execution not supported for file %s: %w: %v", filepath, privilege.ErrPrivilegedExecutionNotSupported, err)
	}

	var privilegedFile *os.File
	privErr := privManager.WithPrivileges(runnertypes.ElevationContext{
		Operation: runnertypes.OperationFileValidation,
		FilePath:  filepath,
	}, func() error {
		var openErr error
		privilegedFile, openErr = os.Open(filepath) //nolint:gosec // filepath is validated by caller
		return openErr
	})

	if privErr != nil {
		return nil, fmt.Errorf("failed to open file %s with privileges: %w", filepath, privErr)
	}

	return privilegedFile, nil
}

// IsPrivilegeError checks if error is a privilege-related error
// This function now uses the existing privilege management error handling
func IsPrivilegeError(err error) bool {
	return errors.Is(err, runnertypes.ErrPrivilegedExecutionNotAvailable)
}
