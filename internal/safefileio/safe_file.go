// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
package safefileio

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
)

// openat2 constants for RESOLVE flags
const (
	// ResolveNoSymlinks disallows resolution of symbolic links
	ResolveNoSymlinks = 0x04
	// AtFdcwd represents the current working directory
	AtFdcwd = -0x64
	// SysOpenat2 is the system call number for openat2 on Linux
	SysOpenat2 = 437
)

// FileOperation represents the type of file operation being performed
type FileOperation int

const (
	// FileOpRead indicates a read operation
	FileOpRead FileOperation = iota
	// FileOpWrite indicates a write operation
	FileOpWrite
)

const (
	// maxAllowedPermsRead defines the maximum allowed file permissions for read operations
	// rwxr-xr-x with setuid/setgid allowed
	maxAllowedPermsRead = 0o4755
	// maxAllowedPermsWrite defines the maximum allowed file permissions for write operations
	// rw-r--r-- (more restrictive for write operations)
	maxAllowedPermsWrite = 0o644
	// groupWritePermission represents the group write bit (020)
	groupWritePermission = 0o020
	// allPermissionBits represents all possible permission and special bits
	allPermissionBits = 0o7777
)

// openHow struct for openat2 system call
type openHow struct {
	flags   uint64
	mode    uint64
	resolve uint64
}

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

const testFilePerm = 0o600 // Read/write for owner only

// isOpenat2Available checks if openat2 system call is available and working
func isOpenat2Available() bool {
	// Check if we're on Linux
	if runtime.GOOS != "linux" {
		return false
	}

	// Create a temporary directory for testing
	testDir, err := os.MkdirTemp("", "openat2test")
	if err != nil {
		return false
	}
	defer func() {
		if err := os.RemoveAll(testDir); err != nil {
			slog.Warn("failed to remove test directory", "error", err, "path", testDir)
		}
	}()

	testFile := filepath.Join(testDir, "testfile")
	how := openHow{
		flags:   uint64(os.O_CREATE | os.O_RDWR | os.O_EXCL),
		mode:    testFilePerm, // #nosec G302 - file permissions are appropriate for test file
		resolve: ResolveNoSymlinks,
	}

	// Test openat2 with actual file operations
	fd, err := openat2(AtFdcwd, testFile, &how)
	// Clean up the test file
	if fd >= 0 {
		_ = syscall.Close(fd)
	}
	_ = os.Remove(testFile)

	return err == nil
}

// openat2 wraps the openat2 system call
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
	pathBytes, err := syscall.BytePtrFromString(pathname)
	if err != nil {
		return -1, err
	}

	fd, _, errno := syscall.Syscall6(
		SysOpenat2,
		uintptr(dirfd),
		// #nosec G103 - uintptr conversion is required for syscall interface
		uintptr(unsafe.Pointer(pathBytes)),
		// #nosec G103 - uintptr conversion is required for syscall interface
		uintptr(unsafe.Pointer(how)),
		unsafe.Sizeof(*how),
		0, 0,
	)

	if errno != 0 {
		return -1, errno
	}

	return int(fd), nil
}

// FileSystem is an interface that abstracts secure file system operations
type FileSystem interface {
	// SafeOpenFile opens a file with security checks to prevent symlink attacks and TOCTOU race conditions
	SafeOpenFile(name string, flag int, perm os.FileMode) (File, error)
	// GetGroupMembership returns the GroupMembership instance for security checks
	GetGroupMembership() *groupmembership.GroupMembership
}

// File is an interface that abstracts file operations
type File interface {
	io.Reader
	io.Writer
	Close() error
	Stat() (os.FileInfo, error)
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
	return fs.safeOpenFileInternal(name, flag, perm)
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
	return safeWriteFileCommon(filePath, content, perm, fs, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
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
	if err := validateRequestedPermissions(requiredPerm, FileOpWrite); err != nil {
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
			slog.Warn("error closing source file", "error", closeErr)
		}
	}()

	// Validate source file properties
	if err := canSafelyWriteToFile(srcFile, absSrc, FileOpRead, fs.GetGroupMembership()); err != nil {
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
			slog.Warn("error closing destination file", "error", closeErr)
		}
	}()

	// Final validation of destination file
	if err := canSafelyWriteToFile(dstFile, absDst, FileOpWrite, fs.GetGroupMembership()); err != nil {
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
	if err := validateRequestedPermissions(perm, FileOpWrite); err != nil {
		return err
	}

	// Use the FileSystem interface consistently for both testing and production
	file, err := fs.SafeOpenFile(absPath, flags, perm)
	if err != nil {
		return err
	}

	// Ensure the file is closed on error
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// Validate the file is a regular file (not a device, pipe, etc.)
	if err := canSafelyWriteToFile(file, absPath, FileOpWrite, fs.GetGroupMembership()); err != nil {
		return err
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
			slog.Warn("error closing file", "error", closeErr)
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

// canSafelyWriteToFile checks if the current user can safely write to a file by validating
// file permissions, ownership, and group membership in a unified security check.
//
// This function performs comprehensive security validation:
//   - Verifies the file is a regular file
//   - Prevents world-writable files (security risk)
//   - For group-writable files, ensures the current user is the file owner or the only group member
//   - Validates file permissions against the specified operation type
//
// Parameters:
//   - file: The opened file to validate
//   - filePath: The file path (for error messages)
//   - operation: The intended file operation (read/write)
//   - groupMembership: Group membership checker for security validation
//
// Returns:
//   - error: Validation error if the file cannot be safely written to
func canSafelyWriteToFile(file File, filePath string, operation FileOperation, groupMembership *groupmembership.GroupMembership) error {
	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, filePath)
	}

	// Get file stat info for UID/GID
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: failed to get file stat info", ErrInvalidFilePath)
	}

	// Use unified permission and ownership check
	canSafelyWrite, err := groupMembership.CanCurrentUserSafelyWriteFile(stat.Uid, stat.Gid, fileInfo.Mode())
	if err != nil {
		return fmt.Errorf("%w: %s - %w", ErrInvalidFilePermissions, filePath, err)
	}
	if !canSafelyWrite {
		return fmt.Errorf("%w: %s - current user cannot safely write to this file",
			ErrInvalidFilePermissions, filePath)
	}

	perm := fileInfo.Mode().Perm()

	// Select maximum allowed permissions based on operation type
	var maxAllowedPerms os.FileMode
	switch operation {
	case FileOpRead:
		maxAllowedPerms = maxAllowedPermsRead
	case FileOpWrite:
		maxAllowedPerms = maxAllowedPermsWrite
	default:
		return fmt.Errorf("%w: unknown operation type", ErrInvalidFileOperation)
	}

	// Check other disallowed bits (excluding group writable which we handled above)
	disallowedBits := perm &^ (maxAllowedPerms | groupWritePermission)
	if disallowedBits != 0 {
		return fmt.Errorf("%w: file %s has permissions %o with disallowed bits %o, maximum allowed is %o (plus group writable under conditions)",
			ErrInvalidFilePermissions, filePath, perm, disallowedBits, maxAllowedPerms)
	}

	return nil
}

// canSafelyReadFromFile checks if the current user can safely read from a file with
// more relaxed permissions compared to write operations.
//
// This function performs read-specific security validation:
//   - Verifies the file is a regular file
//   - Prevents world-writable files (security risk)
//   - For group-writable files, uses more relaxed group membership checks
//   - Allows broader permission range (up to 0o4755 including setuid/setgid)
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
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, filePath)
	}

	// Get file stat info for UID/GID
	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("%w: failed to get file stat info", ErrInvalidFilePath)
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

	// No additional permission checks needed - CanCurrentUserSafelyReadFile handles all validation

	return fileInfo, nil
}

// validateRequestedPermissions validates the requested permissions before file creation/modification
func validateRequestedPermissions(perm os.FileMode, operation FileOperation) error {
	// Select maximum allowed permissions based on operation type
	var maxAllowedPerms os.FileMode
	switch operation {
	case FileOpRead:
		maxAllowedPerms = maxAllowedPermsRead
	case FileOpWrite:
		maxAllowedPerms = maxAllowedPermsWrite
	default:
		return fmt.Errorf("%w: unknown file operation", ErrInvalidFilePath)
	}

	// Check if requested permissions exceed the maximum allowed
	// Use full mode to include setuid/setgid/sticky bits, not just Perm()
	fullMode := perm & allPermissionBits // Include all permission and special bits
	disallowedBits := fullMode &^ (maxAllowedPerms | groupWritePermission)
	if disallowedBits != 0 {
		return fmt.Errorf("%w: requested permissions %o exceed maximum allowed %o for %v operation",
			ErrInvalidFilePermissions, fullMode, maxAllowedPerms, operation)
	}

	return nil
}

// safeOpenFileInternal is the internal implementation of safeOpenFile
func (fs *osFS) safeOpenFileInternal(filePath string, flag int, perm os.FileMode) (*os.File, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	if fs.openat2Available {
		// Use openat2 with RESOLVE_NO_SYMLINKS for atomic operation
		how := openHow{
			// #nosec G115 - flag conversion is intentional and safe within valid flag range
			flags:   uint64(flag),
			mode:    uint64(perm),
			resolve: ResolveNoSymlinks,
		}

		fd, err := openat2(AtFdcwd, absPath, &how)
		if err != nil {
			// Check for specific errors
			if errno, ok := err.(syscall.Errno); ok {
				switch errno {
				case syscall.ELOOP:
					return nil, ErrIsSymlink
				case syscall.EEXIST:
					return nil, ErrFileExists
				case syscall.ENOENT:
					return nil, os.ErrNotExist // Return standard not exist error
				}
			}
			return nil, fmt.Errorf("failed to open file: %w", err)
		}

		return os.NewFile(uintptr(fd), absPath), nil
	}

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
		if os.IsNotExist(err) {
			return nil, err // Return the original error for file not found
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Detect symlink attack after ensureParentDirNoSymlinks call above.
	if err := ensureParentDirsNoSymlinks(absPath); err != nil {
		return nil, err
	}

	return file, nil
}
