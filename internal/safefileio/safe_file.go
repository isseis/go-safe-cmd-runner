// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
//
// Platform-specific implementations:
//   - Linux: see safe_file_linux.go (uses openat2 with fallback to portable method)
//   - Others: see safe_file_nonlinux.go (uses portable method only)
package safefileio

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
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
	io.Seeker   // Required for file offset operations (seek/read from specific positions)
	io.ReaderAt // Required for debug/elf.NewFile and similar operations
	Chmod(mode os.FileMode) error
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

	if err := validateOpenPerm(perm); err != nil {
		return nil, err
	}

	return fs.safeOpenFileInternal(absPath, flag, perm)
}

// validateOpenPerm rejects a permission value that carries bits outside
// os.ModePerm, so that every mode SafeOpenFile hands to open(2) is nine POSIX
// permission bits and nothing else, on both routes.
//
// Go's os.FileMode encodes setuid, setgid, sticky and the file-type bits in
// its own high bits (1<<19 and above) rather than at the POSIX ones, so such a
// value delivers a mode the caller did not mean. Discarding those bits with
// perm.Perm() would hide the caller's mistake instead of surfacing it.
//
// This does not contradict groupmembership.MaxAllowedReadPerms permitting
// setuid and setgid: that limit constrains a file already on disk, this one
// constrains the value handed to open(2).
func validateOpenPerm(perm os.FileMode) error {
	if perm&^os.ModePerm != 0 {
		return fmt.Errorf("%w: %v", ErrUnsupportedFileMode, perm)
	}
	return nil
}

// Remove removes the named file or (empty) directory
func (fs *osFS) Remove(name string) error {
	return os.Remove(name)
}

// AtomicMoveFile atomically moves a file from source to destination with secure permissions.
// Path resolution is intentionally limited to filepath.Abs (no EvalSymlinks) so that symlinks
// in srcPath and dstPath's parent remain visible to the security checks in atomicMoveFileCore
// (SafeOpenFile via openat2 RESOLVE_NO_SYMLINKS for the source, ensureParentDirsNoSymlinks
// for the destination parent).
func (fs *osFS) AtomicMoveFile(srcPath, dstPath string, requiredPerm os.FileMode) error {
	absSrc, err := filepath.Abs(srcPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}
	absDst, err := filepath.Abs(dstPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}
	return atomicMoveFileCore(absSrc, absDst, requiredPerm, fs)
}

// SafeWriteFileOverwrite writes a file safely, allowing overwrite of existing files.
// It uses openat2 with RESOLVE_NO_SYMLINKS when available for atomic symlink-safe operations,
// eliminating TOCTOU (Time-of-Check Time-of-Use) race conditions completely.
// On systems without openat2, it falls back to path verification before opening the file.
//
// filePath must be created with common.NewResolvedPathParentOnly. A path created with
// common.NewResolvedPath would resolve the leaf symlink, bypassing leaf-symlink detection,
// so this function rejects it and returns ErrInvalidFilePath.
//
// Note: The filepath parameter is intentionally not restricted to a safe directory as the
// function is designed to work with any valid file path while maintaining security.
func SafeWriteFileOverwrite(filePath common.ResolvedPath, content []byte, perm os.FileMode) (err error) {
	return safeWriteFileOverwriteWithFS(filePath, content, perm, defaultFS)
}

// safeWriteFileOverwriteWithFS is the internal implementation that accepts a FileSystem for testing
func safeWriteFileOverwriteWithFS(filePath common.ResolvedPath, content []byte, perm os.FileMode, fs FileSystem) (err error) {
	return safeWriteFileCommon(filePath, content, perm, fs)
}

// atomicMoveFileCore is the shared implementation for osFS.AtomicMoveFile.
// absSrc and absDst must be absolute paths. Symlinks in the paths are detected and
// rejected here by SafeOpenFile (openat2 RESOLVE_NO_SYMLINKS) and ensureParentDirsNoSymlinks.
func atomicMoveFileCore(absSrc, absDst string, requiredPerm os.FileMode, fs FileSystem) error {
	// Pre-validate requested permissions
	if err := fs.GetGroupMembership().ValidateRequestedPermissions(requiredPerm, groupmembership.FileOpWrite); err != nil {
		return err
	}

	// Open the source file safely BEFORE changing permissions.
	// SafeOpenFile uses openat2(RESOLVE_NO_SYMLINKS) which rejects symlinks,
	// ensuring we have a handle to the real file and not a symlink target.
	srcFile, err := fs.SafeOpenFile(absSrc, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open source file safely: %w", err)
	}
	defer func() {
		if closeErr := srcFile.Close(); closeErr != nil {
			slog.Warn("error closing source file", slog.Any("error", closeErr))
		}
	}()

	// Set secure permissions via the opened file handle (fchmod).
	// This avoids the TOCTOU race where os.Chmod follows symlinks and could
	// modify permissions on an unintended target before symlink checks run.
	if err := srcFile.Chmod(requiredPerm); err != nil {
		return fmt.Errorf("failed to set secure permissions on source: %w", err)
	}

	// Validate source file properties
	if err := canSafelyAccessFile(srcFile, absSrc, groupmembership.FileOpRead, fs.GetGroupMembership()); err != nil {
		return fmt.Errorf("source file validation failed: %w", err)
	}

	// Ensure destination parent directories are safe
	if err := ensureParentDirsNoSymlinks(absDst); err != nil {
		return fmt.Errorf("destination parent directory unsafe: %w", err)
	}

	// Move the verified source inode into place. moveFileAnchored anchors the
	// move to srcFile's fd (on Linux) rather than re-resolving absSrc by path
	// name, so a replacement of absSrc between SafeOpenFile above and this
	// call cannot cause a different inode to be moved (see safe_file_linux.go).
	if err := moveFileAnchored(srcFile, absSrc, absDst); err != nil {
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
func safeWriteFileCommon(filePath common.ResolvedPath, content []byte, perm os.FileMode, fs FileSystem) (err error) {
	absPath := filePath.String()
	if absPath == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidFilePath)
	}
	// Require NewResolvedPathParentOnly so the leaf-symlink position is not pre-resolved;
	// SafeOpenFile (openat2 RESOLVE_NO_SYMLINKS) can then detect and reject a symlink at the leaf.
	if !filePath.IsParentOnly() {
		return fmt.Errorf("%w: filePath must be created with NewResolvedPathParentOnly", ErrInvalidFilePath)
	}

	// Pre-validate requested permissions for write operation
	if err := fs.GetGroupMembership().ValidateRequestedPermissions(perm, groupmembership.FileOpWrite); err != nil {
		return err
	}

	file, err := fs.SafeOpenFile(absPath, os.O_WRONLY|os.O_CREATE, perm)
	if err != nil {
		return err
	}

	defer func() {
		closeErr := file.Close()
		if closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// Validate the file is a regular file (not a device, pipe, etc.)
	if err := canSafelyAccessFile(file, absPath, groupmembership.FileOpWrite, fs.GetGroupMembership()); err != nil {
		return err
	}

	// Truncate after permission check to ensure content is written to an empty file
	if err := file.Truncate(0); err != nil {
		return fmt.Errorf("failed to truncate %s: %w", absPath, err)
	}

	if _, err = file.Write(content); err != nil {
		return fmt.Errorf("failed to write to %s: %w", absPath, err)
	}

	return nil
}

// ensureParentDirsNoSymlinks checks if any component of the path is a symlink
// by traversing the directory hierarchy step-by-step using opendir(2) equivalent.
//
// Exception: OS-managed symlinks on an explicit allowlist (e.g. /tmp ->
// /private/tmp on macOS) are followed after verifying the target matches the
// expected destination. All other symlinks are rejected to prevent
// symlink-redirect attacks where an attacker substitutes a directory component
// with a symlink to an arbitrary target.
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
			// Allow only well-known OS-managed symlinks whose target matches the
			// expected value in the allowlist (e.g. /tmp -> /private/tmp on macOS).
			// All other symlinks — including unexpected root-owned ones — are rejected.
			if !common.IsAllowedOSManagedSymlink(currentPath) {
				return fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
			}
			// Resolve the OS-managed symlink so subsequent components are
			// evaluated against the real path.
			resolved, err := filepath.EvalSymlinks(currentPath)
			if err != nil {
				return fmt.Errorf("failed to resolve OS symlink %s: %w", currentPath, err)
			}
			currentPath = resolved
			continue
		}

		// Ensure it's a directory (except for the last component which might not exist yet)
		if !fi.IsDir() {
			return fmt.Errorf("%w: not a directory: %s", ErrInvalidFilePath, currentPath)
		}
	}

	return nil
}

// maxTempNameAttempts bounds retries when a randomly generated temporary name
// -- a hard link name in moveFileAnchored, or a temporary file name in the
// write path -- collides with an existing entry (EEXIST). With tmpNameRandBytes
// of entropy, a real collision is astronomically unlikely; this only guards
// against pathological environments and keeps the loop from running forever.
//
// The fallback creation probe reuses the same bound for the same reason: there,
// what it survives is a counterparty repeatedly deleting and recreating the
// target between the probe and the reopen.
const maxTempNameAttempts = 10

// tmpNameRandBytes is the number of random bytes used to build a temporary
// name (see randomTempName).
const tmpNameRandBytes = 12

// randomTempName returns a name unlikely to collide with an existing directory
// entry. The caller supplies the prefix, which marks the entry as
// safefileio-internal state if it is ever left behind and says which operation
// left it.
func randomTempName(prefix string) (string, error) {
	var b [tmpNameRandBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("failed to generate random temporary name: %w", err)
	}
	return prefix + hex.EncodeToString(b[:]), nil
}

// verifySameFile checks that the directory entry currently at path still refers
// to the same inode as the already-open file. It uses Lstat (rather than Stat)
// on path so that a symlink swapped in at path is detected as a mismatch
// instead of being followed.
//
// This narrows the window in which a caller acts on the wrong inode, but it
// cannot close it: the check and the operation that follows it (an unlink, a
// rename) are separate system calls, so the entry can still be replaced in
// between. How much is at stake in that window depends on how the caller names
// its target. A caller working relative to an already-open directory fd can
// only have the final name swapped, the parent being pinned to an inode; a
// caller working from a path name is additionally exposed to its parent
// components being replaced -- which, on the fallback path, is the very thing
// that has just been found untrustworthy.
//
// This check is not redundant with directory-level protections elsewhere in the
// codebase (e.g. the world-writable-directory rejection in
// internal/security/toctou.go): those run well before this point (e.g. before
// the wrapped command runs), so this is the only defense operating at the
// moment of the operation itself.
func verifySameFile(file File, path string) error {
	fdInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("failed to fstat open file descriptor: %w", err)
	}
	fdStat, ok := fdInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: unsupported file info type for fd", ErrUnsupportedFileHandle)
	}

	var pathStat syscall.Stat_t
	if err := syscall.Lstat(path, &pathStat); err != nil {
		return fmt.Errorf("failed to lstat path %q: %w", path, err)
	}

	if fdStat.Dev != pathStat.Dev || fdStat.Ino != pathStat.Ino {
		return fmt.Errorf("%w: path %q no longer refers to the expected inode", ErrSourceIdentityMismatch, path)
	}

	return nil
}

// withCloseError appends a close failure to a warning's attributes, and only a
// failure: an operator reading these records to decide whether a destination
// needs quarantining learns nothing from a close_error that is nil every time.
func withCloseError(closeErr error, attrs ...any) []any {
	if closeErr == nil {
		return attrs
	}
	return append(attrs, slog.Any("close_error", closeErr))
}

// removeVerifiedFileByPath closes file and then removes path, but only once it
// has established that path still names the inode file refers to. If it does
// not, or if that cannot be determined, the entry is left alone: at the point
// this runs the path is already suspect, and deleting whatever happens to sit
// there now would turn a detected attack into a deletion the attacker chose.
//
// file is always closed, whether or not the removal happens.
//
// The returned error describes why the removal was skipped or failed -- or, if
// it did happen, why the close failed -- for a caller that wants to assert on
// it. Callers in a failure path must not return it in place of the failure that
// brought them here (see the design document's error handling section); this
// function records the reason itself.
func removeVerifiedFileByPath(file File, path string) error {
	verifyErr := verifySameFile(file, path)
	closeErr := file.Close()

	if verifyErr != nil {
		// The wording covers both reasons this branch is reached: the entry is
		// known to be a different inode, and the comparison could not be made
		// at all. Both leave the entry alone.
		slog.Warn("left a file in place after a failed open: could not confirm it still refers to the opened inode",
			withCloseError(closeErr,
				slog.String("path", path),
				slog.Any("error", verifyErr))...)
		return verifyErr
	}

	if err := os.Remove(path); err != nil {
		slog.Warn("failed to remove the file created by a failed open",
			withCloseError(closeErr,
				slog.String("path", path),
				slog.Any("error", err))...)
		return err
	}

	if closeErr != nil {
		slog.Warn("failed to close the file created by a failed open",
			slog.String("path", path),
			slog.Any("error", closeErr))
		return closeErr
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
func SafeReadFile(filePath common.ResolvedPath) ([]byte, error) {
	return SafeReadFileWithFS(filePath, defaultFS)
}

// SafeReadFileWithFS is the internal implementation that accepts a FileSystem for testing
func SafeReadFileWithFS(filePath common.ResolvedPath, fs FileSystem) ([]byte, error) {
	absPath := filePath.String()
	if absPath == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidFilePath)
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
// This helper performs common validation steps used by multiple functions.
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
// It verifies the file is a regular file and uses groupmembership to validate permissions for the operation.
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
// more relaxed permissions than write operations.
// It verifies the file is a regular file and uses groupmembership to validate read permissions.
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
//
// When the second verification fails the open is abandoned: the caller receives
// that failure and nothing else, and this function leaves behind neither the
// open fd nor a file it created along the way. What it must not do is delete
// whatever the path names at that moment -- the path has just been found
// untrustworthy -- so a file is removed only after removeVerifiedFileByPath has
// confirmed it is still the inode that was opened.
func safeOpenFileFallback(absPath string, flag int, perm os.FileMode) (*os.File, error) {
	// Prevent symlink attacks by ensuring parent directories are not symlinks.
	if err := ensureParentDirsNoSymlinks(absPath); err != nil {
		return nil, err
	}

	file, created, err := openNoFollowTrackingCreation(absPath, flag, perm)
	if err != nil {
		return nil, err
	}

	// Detect symlink attack after ensureParentDirNoSymlinks call above.
	if err := ensureParentDirsAfterOpen(absPath); err != nil {
		// Recorded independently of how the cleanup below turns out: a
		// component of the path changed while the file was being opened, which
		// is the signature of an attack in progress.
		slog.Warn("path check failed after opening the file; abandoning the open",
			slog.String("path", absPath),
			slog.Any("error", err))

		if created {
			// Any failure here is already recorded by the helper. Reporting it
			// instead of err would replace the attack signal with a detail of
			// the cleanup.
			_ = removeVerifiedFileByPath(file, absPath)
		} else if closeErr := file.Close(); closeErr != nil {
			slog.Warn("failed to close the file after abandoning the open",
				slog.String("path", absPath),
				slog.Any("error", closeErr))
		}
		return nil, err
	}

	return file, nil
}

// openNoFollowTrackingCreation opens absPath without following a symlink at the
// leaf, and reports whether this call is what brought the file into existence.
//
// O_CREATE alone does not say which happened, and the fd cannot be asked
// afterwards. So when the caller passes O_CREATE without O_EXCL, the open is
// split: first a probe with O_EXCL added, which succeeds only if the file was
// created here, and on EEXIST a reopen with O_CREATE dropped, which by
// definition did not create it. The reopen keeps O_NOFOLLOW, so a symlink at
// the leaf -- which is what EEXIST means when one is present -- is still
// rejected with ErrIsSymlink rather than read as "a regular file was already
// there".
//
// The EEXIST that drives that decision is internal to this function and is
// never returned: ErrFileExists is the answer to a caller that asked for
// O_EXCL itself.
//
// A counterparty deleting the file between the probe and the reopen turns the
// reopen into ENOENT. That is retried from the probe, up to
// maxTempNameAttempts, after which the ENOENT is returned; a caller is never
// told the open succeeded when it did not.
func openNoFollowTrackingCreation(absPath string, flag int, perm os.FileMode) (*os.File, bool, error) {
	if flag&os.O_CREATE == 0 || flag&os.O_EXCL != 0 {
		file, err := openNoFollow(absPath, flag, perm)
		if err != nil {
			return nil, false, err
		}
		// An O_CREATE|O_EXCL open that succeeded created the file; an open
		// without O_CREATE never does.
		return file, flag&os.O_CREATE != 0, nil
	}

	var lastErr error
	for range maxTempNameAttempts {
		file, err := openNoFollow(absPath, flag|os.O_EXCL, perm)
		if err == nil {
			return file, true, nil
		}
		if !errors.Is(err, ErrFileExists) {
			return nil, false, err
		}

		file, err = openNoFollow(absPath, flag&^os.O_CREATE, perm)
		if err == nil {
			return file, false, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, false, err
		}
		lastErr = err
	}

	// The entry kept disappearing between the probe and the reopen, which is
	// what a counterparty deleting and recreating it looks like from here.
	slog.Warn("gave up opening the file: it kept disappearing between the creation probe and the reopen",
		slog.String("path", absPath),
		slog.Int("attempts", maxTempNameAttempts),
		slog.Any("error", lastErr))

	// Wrapped rather than returned bare, so that this return is non-nil by
	// construction: handing back a nil error with a nil file would have every
	// caller dereference it. The last ENOENT is preserved, since a caller
	// distinguishing "not there" is right either way.
	return nil, false, fmt.Errorf("gave up opening %s after %d attempts: %w", absPath, maxTempNameAttempts, lastErr)
}

// openNoFollow opens absPath with O_NOFOLLOW added and maps the errno values
// this package gives sentinels to.
func openNoFollow(absPath string, flag int, perm os.FileMode) (*os.File, error) {
	// #nosec G304 - absPath is validated by the caller
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
	return file, nil
}
