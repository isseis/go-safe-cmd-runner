package filevalidator

import (
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// defaultFS is the default FileSystem used by OpenFileWithPrivileges.
// It can be overridden for testing via SetFileSystemForTesting.
var defaultFS safefileio.FileSystem = safefileio.NewFileSystem(safefileio.FileSystemConfig{})

// SetFileSystemForTesting allows tests to inject a mock FileSystem.
// This should only be used in tests.
func SetFileSystemForTesting(fs safefileio.FileSystem) {
	defaultFS = fs
}

// ResetFileSystemForTesting resets the FileSystem to the default implementation.
// This should be called in test cleanup.
func ResetFileSystemForTesting() {
	defaultFS = safefileio.NewFileSystem(safefileio.FileSystemConfig{})
}

// OpenFileWithPrivileges opens a file with elevated privileges and immediately restores them.
// This function uses safefileio for secure file access, preventing symlink attacks and
// TOCTOU race conditions even during privilege escalation.
//
// Returns safefileio.File which implements io.Reader, io.Writer, io.Seeker, and io.ReaderAt.
func OpenFileWithPrivileges(filepath string, privManager runnertypes.PrivilegeManager) (safefileio.File, error) {
	return OpenFileWithPrivilegesFS(filepath, privManager, defaultFS)
}

// OpenFileWithPrivilegesFS opens a file with elevated privileges using the provided FileSystem.
// This allows for dependency injection in tests.
func OpenFileWithPrivilegesFS(filepath string, privManager runnertypes.PrivilegeManager, fs safefileio.FileSystem) (safefileio.File, error) {
	// Attempt normal access with standard privileges first using safefileio
	// This prevents symlink attacks and TOCTOU race conditions
	file, err := fs.SafeOpenFile(filepath, os.O_RDONLY, 0)
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
		privilegedFile, openErr = fs.SafeOpenFile(filepath, os.O_RDONLY, 0)
		return openErr
	})

	if privErr != nil {
		return nil, fmt.Errorf("failed to open file %s with privileges: %w", filepath, privErr)
	}

	return privilegedFile, nil
}
