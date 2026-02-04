package filevalidator

import (
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// PrivilegedFileValidator provides secure file operations with privilege escalation support.
// It encapsulates the FileSystem dependency to avoid global state and enable
// safe parallel testing without race conditions.
type PrivilegedFileValidator struct {
	fs safefileio.FileSystem
}

// NewPrivilegedFileValidator creates a new PrivilegedFileValidator with an optional custom FileSystem.
// If fs is nil, a default FileSystem is created.
func NewPrivilegedFileValidator(fs safefileio.FileSystem) *PrivilegedFileValidator {
	if fs == nil {
		fs = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	}
	return &PrivilegedFileValidator{fs: fs}
}

// DefaultPrivilegedFileValidator returns a PrivilegedFileValidator instance with the default FileSystem.
func DefaultPrivilegedFileValidator() *PrivilegedFileValidator {
	return NewPrivilegedFileValidator(nil)
}

// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them.
// This function uses safefileio for secure file access, preventing symlink attacks and
// TOCTOU race conditions even during privilege escalation.
//
// Returns safefileio.File which implements io.Reader, io.Writer, io.Seeker, and io.ReaderAt.
func (pfv *PrivilegedFileValidator) OpenFileWithPrivileges(filepath string, privManager runnertypes.PrivilegeManager) (safefileio.File, error) {
	// Attempt normal access with standard privileges first using safefileio
	// This prevents symlink attacks and TOCTOU race conditions
	file, err := pfv.fs.SafeOpenFile(filepath, os.O_RDONLY, 0)
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

	// Use safefileio even during privilege escalation for full TOCTOU protection
	var privilegedFile safefileio.File
	privErr := privManager.WithPrivileges(runnertypes.ElevationContext{
		Operation: runnertypes.OperationFileValidation,
		FilePath:  filepath,
	}, func() error {
		var openErr error
		privilegedFile, openErr = pfv.fs.SafeOpenFile(filepath, os.O_RDONLY, 0)
		return openErr
	})

	if privErr != nil {
		return nil, fmt.Errorf("failed to open file %s with privileges: %w", filepath, privErr)
	}

	return privilegedFile, nil
}
