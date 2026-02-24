// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
//
// Platform-specific implementations:
//   - Linux: see safe_file_linux.go (uses openat2 with fallback to portable method)
//   - Others: see safe_file_nonlinux.go (uses portable method only)
package safefileio

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
)

// FileSystemConfig holds configuration for the file system operations
type FileSystemConfig struct {
	// DisableOpenat2 explicitly disables openat2 usage even if available
	DisableOpenat2 bool
}

// osFS implements FileSystem using the local disk
type osFS struct {
	openat2Available bool
	config           FileSystemConfig
	groupMembership  *groupmembership.GroupMembership
}

// NewFileSystem creates a new FileSystem with the given configuration
func NewFileSystem(config FileSystemConfig) FileSystem {
	fs := &osFS{
		config:          config,
		groupMembership: groupmembership.New(),
	}

	if !config.DisableOpenat2 {
		fs.openat2Available = isOpenat2Available()
	}

	return fs
}

// DefaultFileSystem is the default filesystem implementation
var defaultFS = NewFileSystem(FileSystemConfig{})

// FileSystem is an interface that abstracts secure file system operations
type FileSystem interface {
	// SafeOpenFile opens a file with security checks to prevent symlink attacks and TOCTOU race conditions
	SafeOpenFile(name string, flag int, perm os.FileMode) (File, error)
	// GetGroupMembership returns the GroupMembership instance for security checks
	GetGroupMembership() *groupmembership.GroupMembership
	// Remove removes the named file or (empty) directory
	Remove(name string) error
	// AtomicMoveFile atomically moves a file from source to destination with secure permissions
	AtomicMoveFile(srcPath, dstPath string, requiredPerm os.FileMode) error
}

// File is an interface that abstracts file operations
// The underlying *os.File implements all these interfaces.
type File interface {
	io.Reader
	io.Writer
	io.Seeker   // Required for VerifyFromHandle and similar operations
	io.ReaderAt // Required for debug/elf.NewFile and similar operations
	Close() error
	Stat() (os.FileInfo, error)
	Truncate(size int64) error
}

// IsOpenat2Available returns true if openat2 is available and enabled
func (fs *osFS) IsOpenat2Available() bool {
	return fs.openat2Available
}

// GetGroupMembership returns the GroupMembership instance for security checks
func (fs *osFS) GetGroupMembership() *groupmembership.GroupMembership {
	return fs.groupMembership
}

func (fs *osFS) SafeOpenFile(name string, flag int, perm os.FileMode) (File, error) {
	absPath, err := filepath.Abs(name)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	return fs.safeOpenFileInternal(absPath, flag, perm)
}

// Remove removes the named file or (empty) directory
func (fs *osFS) Remove(name string) error {
	return os.Remove(name)
}

// AtomicMoveFile atomically moves a file from source to destination with secure permissions
func (fs *osFS) AtomicMoveFile(srcPath, dstPath string, requiredPerm os.FileMode) error {
	return safeAtomicMoveFileWithFS(srcPath, dstPath, requiredPerm, fs)
}

// SafeWriteFile writes a file safely after validating the path and checking file properties.
// It uses openat2 with RESOLVE_NO_SYMLINKS when available for atomic symlink-safe operations,
// eliminating TOCTOU (Time-of-Check Time-of-Use) race conditions completely.
// On systems without openat2, it falls back to path verification before opening the file.
//
// Note: The filepath parameter is intentionally not restricted to a safe directory as the
// function is designed to work with any valid file path while maintaining security.
func SafeWriteFile(filePath string, content []byte, perm os.FileMode) (err error) {
	return safeWriteFileWithFS(filePath, content, perm, defaultFS)
}

// SafeWriteFileOverwrite writes a file safely, allowing overwrite of existing files.
// It uses openat2 with RESOLVE_NO_SYMLINKS when available for atomic symlink-safe operations,
// eliminating TOCTOU (Time-of-Check Time-of-Use) race conditions completely.
// On systems without openat2, it falls back to path verification before opening the file.
//
// Note: The filepath parameter is intentionally not restricted to a safe directory as the
// function is designed to work with any valid file path while maintaining security.
func SafeWriteFileOverwrite(filePath string, content []byte, perm os.FileMode) (err error) {
	return safeWriteFileOverwriteWithFS(filePath, content, perm, defaultFS)
}

// SafeAtomicMoveFile atomically moves a file from source to destination with secure permissions.
// It uses rename(2) system call for atomic operation and validates both source and destination
// files using safefileio security checks. The source file permissions are set to requiredPerm
// before the move operation.
//
// This function provides protection against symlink attacks, TOCTOU race conditions, and
// ensures the destination file has the required permissions and security properties.
func SafeAtomicMoveFile(srcPath, dstPath string, requiredPerm os.FileMode) error {
	return safeAtomicMoveFileWithFS(srcPath, dstPath, requiredPerm, defaultFS)
}

// safeWriteFileOverwriteWithFS is the internal implementation that accepts a FileSystem for testing
func safeWriteFileOverwriteWithFS(filePath string, content []byte, perm os.FileMode, fs FileSystem) (err error) {
	return safeWriteFileCommon(filePath, content, perm, fs, os.O_WRONLY|os.O_CREATE)
}

// safeWriteFileWithFS is the internal implementation that accepts a FileSystem for testing
func safeWriteFileWithFS(filePath string, content []byte, perm os.FileMode, fs FileSystem) (err error) {
	return safeWriteFileCommon(filePath, content, perm, fs, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
}

// safeAtomicMoveFileWithFS is the internal implementation that accepts a FileSystem for testing
func safeAtomicMoveFileWithFS(srcPath, dstPath string, requiredPerm os.FileMode, fs FileSystem) error {
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	absDst, err := filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// Pre-validate requested permissions
	if err := fs.GetGroupMembership().ValidateRequestedPermissions(requiredPerm, groupmembership.FileOpWrite); err != nil {
		return err
	}

	// Set secure permissions on source file before move
	if err := os.Chmod(absSrc, requiredPerm); err != nil {
		return fmt.Errorf("failed to set secure permissions on source: %w", err)
	}

	// Validate source file through safefileio
	srcFile, err := fs.SafeOpenFile(absSrc, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open source file safely: %w", err)
	}
	defer func() {
		if closeErr := srcFile.Close(); closeErr != nil {
			slog.Warn("error closing source file", slog.Any("error", closeErr))
		}
	}()

	// Validate source file properties
	if err := canSafelyAccessFile(srcFile, absSrc, groupmembership.FileOpRead, fs.GetGroupMembership()); err != nil {
		return fmt.Errorf("source file validation failed: %w", err)
	}

	// Ensure destination parent directories are safe
	if err := ensureParentDirsNoSymlinks(absDst); err != nil {
		return fmt.Errorf("destination parent directory unsafe: %w", err)
	}

	// Perform atomic rename
	if err := os.Rename(absSrc, absDst); err != nil {
		return fmt.Errorf("atomic move failed: %w", err)
	}

	// Validate destination file after move
	dstFile, err := fs.SafeOpenFile(absDst, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open destination file safely: %w", err)
	}
	defer func() {
		if closeErr := dstFile.Close(); closeErr != nil {
			slog.Warn("error closing destination file", slog.Any("error", closeErr))
		}
	}()

	// Final validation of destination file
	if err := canSafelyAccessFile(dstFile, absDst, groupmembership.FileOpWrite, fs.GetGroupMembership()); err != nil {
		return fmt.Errorf("destination file validation failed: %w", err)
	}

	return nil
}

// safeWriteFileCommon contains the common logic for safe file writing operations
func safeWriteFileCommon(filePath string, content []byte, perm os.FileMode, fs FileSystem, flags int) (err error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// Pre-validate requested permissions for write operation
	if err := fs.GetGroupMembership().ValidateRequestedPermissions(perm, groupmembership.FileOpWrite); err != nil {
		return err
	}

	// Track whether file was created by this function
	// Only set to true when creating a new file (O_EXCL), not when truncating an existing file
	fileCreated := false

	// Use the FileSystem interface consistently for both testing and production
	file, err := fs.SafeOpenFile(absPath, flags, perm)
	if err != nil {
		return err
	}
	// File was successfully opened/created - only mark as created if using O_EXCL (new file)
	if flags&os.O_EXCL != 0 {
		fileCreated = true
	}

	// Ensure the file is closed, and remove it if validation or writing fails
	defer func() {
		closeErr := file.Close()

		// If there was an error during validation or writing, remove the file
		if err != nil && fileCreated {
			if removeErr := fs.Remove(absPath); removeErr != nil {
				slog.Warn("failed to remove file after error",
					slog.String("path", absPath),
					slog.Any("original_error", err),
					slog.Any("remove_error", removeErr))
			}
		}

		// Report close error if no other error occurred
		if closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// Validate the file is a regular file (not a device, pipe, etc.)
	if err := canSafelyAccessFile(file, absPath, groupmembership.FileOpWrite, fs.GetGroupMembership()); err != nil {
		return err
	}

	// Truncate the file after permission check to ensure content is written to an empty file
	// For O_EXCL (new file), this is a no-op but harmless
	// For overwrite mode, this ensures the file is truncated only after permission validation
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate %s: %w", absPath, err)
	}

	// Write the content
	if _, err = file.Write(content); err != nil {
		return fmt.Errorf("failed to write to %s: %w", absPath, err)
	}

	return nil
}

// ensureParentDirsNoSymlinks checks if any component of the path is a symlink
// by traversing the directory hierarchy step-by-step using opendir(2) equivalent.
func ensureParentDirsNoSymlinks(absPath string) error {
	// Get the directory of the file
	dir := filepath.Dir(absPath)

	components := splitPathComponents(dir)

	// Start from the root and traverse step by step
	// Note: filepath.VolumeName(dir) + string(os.PathSeparator) ensures correct root path on both Unix and Windows.
	// For example, on Windows: VolumeName("C:\\Users") + "\\" = "C:\\"
	currentPath := filepath.VolumeName(dir) + string(os.PathSeparator)

	for _, component := range components {
		currentPath = filepath.Join(currentPath, component)

		// Use os.Lstat to check if the current component is a symlink
		// This doesn't follow symlinks, making it safe
		fi, err := os.Lstat(currentPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Directory doesn't exist yet, which is fine for creation
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", currentPath, err)
		}

		// Check if it's a symlink
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
		}

		// Ensure it's a directory (except for the last component which might not exist yet)
		if !fi.IsDir() {
			return fmt.Errorf("%w: not a directory: %s", ErrInvalidFilePath, currentPath)
		}
	}

	return nil
}

// splitPathComponents splits the given directory path into its components from root to target directory
// and returns them as a slice of strings in order.
// Example: "/home/user/docs" becomes ["home", "user", "docs"].
func splitPathComponents(dir string) []string {
	// Note: For efficiency, we append each new element to the end of the slice during traversal (O(1)
	// per append), and then reverse the slice once at the end. This avoids the O(n^2) behavior of
	// prepending to the front of the slice in a loop.
	components := []string{}
	current := dir
	for {
		parent := filepath.Dir(current)
		if parent == current {
			// Reached root directory
			break
		}

		components = append(components, filepath.Base(current))
		current = parent
	}

	// Reverse the slice to get the correct order (root to target)
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}
	return components
}

// MaxFileSize is the maximum allowed file size for SafeReadFile (128 MB)
const MaxFileSize = 128 * 1024 * 1024

// SafeReadFile reads a file safely after validating the path and checking file properties.
// It enforces a maximum file size of MaxFileSize to prevent memory exhaustion attacks.
// It uses openat2 with RESOLVE_NO_SYMLINKS when available for atomic symlink-safe operations.
func SafeReadFile(filePath string) ([]byte, error) {
	return SafeReadFileWithFS(filePath, defaultFS)
}

// SafeReadFileWithFS is the internal implementation that accepts a FileSystem for testing
func SafeReadFileWithFS(filePath string, fs FileSystem) ([]byte, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// Use the FileSystem interface consistently for both testing and production
	file, err := fs.SafeOpenFile(absPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.Warn("error closing file", slog.Any("error", closeErr))
		}
	}()

	return readFileContent(file, absPath, fs)
}

// readFileContent reads and validates the content of an already opened file
func readFileContent(file File, filePath string, fs FileSystem) ([]byte, error) {
	fileInfo, err := canSafelyReadFromFile(file, filePath, fs.GetGroupMembership())
	if err != nil {
		return nil, err
	}

	if fileInfo.Size() > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	// Use io.ReadAll with LimitReader for consistent behavior across implementations
	content, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if int64(len(content)) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	return content, nil
}

// getFileStatInfo retrieves file statistics and validates that the file is a regular file.
// This helper function performs common validation steps used by multiple functions.
//
// Parameters:
//   - file: The file to examine
//   - filePath: The file path for error reporting
//
// Returns:
//   - os.FileInfo: File information if validation passes
//   - *syscall.Stat_t: System-specific file statistics
//   - error: Validation error if the file is invalid
func getFileStatInfo(file File, filePath string) (os.FileInfo, *syscall.Stat_t, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, filePath)
	}

	// Get file stat info for UID/GID
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, nil, fmt.Errorf("%w: failed to get file stat info", ErrInvalidFilePath)
	}

	return fileInfo, stat, nil
}

// canSafelyAccessFile checks if the current user can safely access a file by validating
// file permissions, ownership, and group membership in a unified security check.
//
// This function performs comprehensive security validation:
//   - Verifies the file is a regular file
//   - Uses groupmembership to validate write permissions
//
// Parameters:
//   - file: The opened file to validate
//   - filePath: The file path (for error messages)
//   - operation: The intended file operation (read/write)
//   - groupMembership: Group membership checker for security validation
//
// Returns:
//   - error: Validation error if the file cannot be safely accessed
func canSafelyAccessFile(file File, filePath string, operation groupmembership.FileOperation, groupMembership *groupmembership.GroupMembership) error {
	fileInfo, stat, err := getFileStatInfo(file, filePath)
	if err != nil {
		return err
	}

	// Use unified permission and ownership check based on operation type
	switch operation {
	case groupmembership.FileOpRead:
		canSafelyRead, err := groupMembership.CanCurrentUserSafelyReadFile(stat.Gid, fileInfo.Mode())
		if err != nil {
			return fmt.Errorf("%w: %s - %w", ErrInvalidFilePermissions, filePath, err)
		}
		if !canSafelyRead {
			return fmt.Errorf("%w: %s - current user cannot safely read from this file",
				ErrInvalidFilePermissions, filePath)
		}
	case groupmembership.FileOpWrite:
		canSafelyWrite, err := groupMembership.CanCurrentUserSafelyWriteFile(stat.Uid, stat.Gid, fileInfo.Mode())
		if err != nil {
			return fmt.Errorf("%w: %s - %w", ErrInvalidFilePermissions, filePath, err)
		}
		if !canSafelyWrite {
			return fmt.Errorf("%w: %s - current user cannot safely write to this file",
				ErrInvalidFilePermissions, filePath)
		}
	default:
		return fmt.Errorf("%w: unknown operation type", ErrInvalidFileOperation)
	}

	return nil
}

// canSafelyReadFromFile checks if the current user can safely read from a file with
// more relaxed permissions compared to write operations.
//
// This function performs read-specific security validation:
//   - Verifies the file is a regular file
//   - Uses groupmembership to validate read permissions
//
// Parameters:
//   - file: The opened file to validate
//   - filePath: The file path (for error messages)
//   - groupMembership: Group membership checker for security validation
//
// Returns:
//   - os.FileInfo: File information if validation passes
//   - error: Validation error if the file cannot be safely read from
func canSafelyReadFromFile(file File, filePath string, groupMembership *groupmembership.GroupMembership) (os.FileInfo, error) {
	fileInfo, stat, err := getFileStatInfo(file, filePath)
	if err != nil {
		return nil, err
	}

	// Use comprehensive read-specific permission check from groupmembership
	// This covers world-writable checks, group membership validation, and permission validation
	canSafelyRead, err := groupMembership.CanCurrentUserSafelyReadFile(stat.Gid, fileInfo.Mode())
	if err != nil {
		return nil, fmt.Errorf("%w: %s - %w", ErrInvalidFilePermissions, filePath, err)
	}
	if !canSafelyRead {
		return nil, fmt.Errorf("%w: %s - current user cannot safely read from this file",
			ErrInvalidFilePermissions, filePath)
	}

	return fileInfo, nil
}

// safeOpenFileFallback implements the fallback method for opening files without openat2.
// This method performs two-phase verification to detect symlink attacks:
// 1. Verify parent directories are not symlinks before opening
// 2. Verify again after opening to detect TOCTOU race conditions
func safeOpenFileFallback(absPath string, flag int, perm os.FileMode) (*os.File, error) {
	// Prevent symlink attacks by ensuring parent directories are not symlinks.
	if err := ensureParentDirsNoSymlinks(absPath); err != nil {
		return nil, err
	}

	// #nosec G304 - absPath is properly validated above
	file, err := os.OpenFile(absPath, flag|syscall.O_NOFOLLOW, perm)
	if err != nil {
		if os.IsExist(err) {
			return nil, ErrFileExists
		}
		if isNoFollowError(err) {
			return nil, ErrIsSymlink
		}
		return nil, err
	}

	// Detect symlink attack after ensureParentDirNoSymlinks call above.
	if err := ensureParentDirsNoSymlinks(absPath); err != nil {
		return nil, err
	}

	return file, nil
}
