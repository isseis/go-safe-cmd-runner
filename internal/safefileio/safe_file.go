// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
//
// Platform-specific implementations:
//   - Linux: see safe_file_linux.go (uses openat2 with fallback to portable method)
//   - Others: see safe_file_nonlinux.go (uses portable method only)
//
// # How strong the guarantees are depends on the route
//
// On the openat2 route the kernel resolves the path and opens it in a single
// system call with RESOLVE_NO_SYMLINKS, so there is no moment between checking
// a path and using it: the race this package defends against cannot occur.
//
// The fallback route -- taken whenever openat2 is unavailable (Linux 5.5 and
// earlier, every non-Linux platform) or has been switched off with
// FileSystemConfig.DisableOpenat2 -- is best-effort: it narrows the window in
// which a path component can be substituted, and detects a substitution that
// did occur, but it does not eliminate the window. Once a directory fd is held
// the window is closed for everything that follows.
//
// The production target for this project is therefore Linux 5.6 or later.
// Non-Linux platforms are for development and limited use, not for production.
// See docs/user/security-risk-assessment.md, "Assumptions and Limitations".
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
	"golang.org/x/sys/unix"
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
	return newOSFS(config)
}

// newOSFS is NewFileSystem keeping the concrete type, for the package-internal
// callers that need the directory-fd primitives (openDirNoSymlinks, openFileAt)
// the FileSystem interface deliberately does not expose.
func newOSFS(config FileSystemConfig) *osFS {
	fs := &osFS{
		config:          config,
		groupMembership: groupmembership.New(),
	}

	if !config.DisableOpenat2 {
		fs.openat2Available = isOpenat2Available()
	}

	return fs
}

// defaultFS is the default filesystem implementation
var defaultFS = newOSFS(FileSystemConfig{})

// FileSystem is an interface that abstracts secure file system operations
type FileSystem interface {
	// SafeOpenFile opens a file with security checks to prevent symlink attacks
	// and TOCTOU race conditions. See the package documentation for how strong
	// that protection is on each route.
	SafeOpenFile(name string, flag int, perm os.FileMode) (File, error)
	// GetGroupMembership returns the GroupMembership instance for security checks
	GetGroupMembership() *groupmembership.GroupMembership
	// AtomicMoveFile atomically moves a file from source to destination with
	// secure permissions. See the package documentation for how strong the
	// symlink and TOCTOU protection is on each route.
	//
	// The rename onto the destination is the point of no return: if a later
	// step fails, the returned error wraps ErrDestinationCommitted and the
	// destination holds the moved file, which the caller can tell apart with
	// errors.Is. Nothing is rolled back -- whatever was at the destination is
	// gone either way, and undoing the move would leave the caller with
	// neither version.
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
	// Sync must reach durable storage before the caller treats the content as
	// written: safeWriteFileCommon relies on it to guarantee that a crash
	// leaves the destination holding either the old content or the new one.
	Sync() error
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

// AtomicMoveFile atomically moves a file from source to destination with secure permissions.
// Path resolution is intentionally limited to filepath.Abs (no EvalSymlinks) so that symlinks
// in srcPath and dstPath's parent remain visible to the security checks in atomicMoveFileCore
// (openDirNoSymlinks for each parent directory, openFileAt for the source leaf).
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
//
// The content is written to a temporary file in the destination's own directory
// and renamed over the destination, so the destination is only ever the content
// it already had or the content of a completed write. See the package
// documentation for how strong the symlink and TOCTOU protection is on each
// route.
//
// A failure before the rename leaves the destination as it was; a failure after
// it leaves the destination holding the new content and returns an error
// wrapping ErrDestinationCommitted, which the caller can detect with errors.Is.
// Either may leave a temporary entry in the directory, whose path is recorded.
//
// filePath must be created with common.NewResolvedPathParentOnly. A path created with
// common.NewResolvedPath would resolve the leaf symlink, bypassing leaf-symlink detection,
// so this function rejects it and returns ErrInvalidFilePath.
//
// The path is deliberately not restricted to a safe directory.
func SafeWriteFileOverwrite(filePath common.ResolvedPath, content []byte, perm os.FileMode) error {
	return safeWriteFileOverwriteWithFS(filePath, content, perm, defaultFS)
}

// safeWriteFileOverwriteWithFS is the internal implementation that accepts a
// FileSystem for testing. The concrete type is required rather than the
// interface: the write works through the directory-fd primitives
// (openDirNoSymlinks, openFileAt), which the interface deliberately does not
// expose.
func safeWriteFileOverwriteWithFS(filePath common.ResolvedPath, content []byte, perm os.FileMode, fs *osFS) error {
	return safeWriteFileCommon(filePath, content, perm, fs)
}

// atomicMoveFileCore is the shared implementation for osFS.AtomicMoveFile.
// absSrc and absDst must be absolute paths. Symlinks in the paths are detected
// and rejected here by openDirNoSymlinks, which pins each parent directory to an
// inode, and by openFileAt, which rejects a symlink at the leaf.
//
// It opens and validates the source; moveOpenFileCore does everything from the
// first side effect onwards. Once both directory fds are held no path is
// resolved a second time.
func atomicMoveFileCore(absSrc, absDst string, requiredPerm os.FileMode, fs *osFS) error {
	if err := fs.GetGroupMembership().ValidateRequestedPermissions(requiredPerm, groupmembership.FileOpWrite); err != nil {
		return err
	}

	// This route does not go through SafeOpenFile either, so it too carries
	// SafeOpenFile's mode check, in the same order and for the same reasons;
	// see safeWriteFileCommon.
	if err := validateOpenPerm(requiredPerm); err != nil {
		return err
	}

	srcDir, srcName := filepath.Dir(absSrc), filepath.Base(absSrc)
	dstDir, dstName := filepath.Dir(absDst), filepath.Base(absDst)

	srcDirFd, err := fs.openDirNoSymlinks(srcDir)
	if err != nil {
		return fmt.Errorf("source parent directory unsafe: %w", err)
	}
	defer closeDirFd(srcDirFd, srcDir)

	dstDirFd, err := fs.openDirNoSymlinks(dstDir)
	if err != nil {
		return fmt.Errorf("destination parent directory unsafe: %w", err)
	}
	defer closeDirFd(dstDirFd, dstDir)

	// Opened relative to the directory fd just verified, so that neither the
	// parent nor the leaf can be substituted by a symlink.
	srcFile, err := fs.openFileAt(srcDirFd, srcName, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open source file safely: %w", err)
	}
	defer func() {
		if closeErr := srcFile.Close(); closeErr != nil {
			slog.Warn("error closing source file", slog.Any("error", closeErr))
		}
	}()

	// Validate the source before anything is changed about it: the mode this
	// check reads must be the caller's, not one this function has already
	// narrowed. A source it refuses is left exactly as it was found.
	if err := canSafelyAccessFile(srcFile, absSrc, subjectFileAtPath, groupmembership.FileOpRead, fs.GetGroupMembership()); err != nil {
		return fmt.Errorf("source file validation failed: %w", err)
	}

	return moveOpenFileCore(srcFile, srcDirFd, srcName, dstDirFd, dstName, absDst, requiredPerm, fs.GetGroupMembership())
}

const destinationCommittedMsg = "destination was replaced before the failure; it now holds the moved file"

var errRenameCommitted = errors.New("failure occurred after the rename replaced the destination")

// moveOpenFileCore moves the inode behind file to dstName under dstDirFd; the
// caller has already opened the source and validated it for reading.
//
// file must be the source's own fd, not a handle reopened by name: the read
// validation the caller ran does not look at the owner, so an attacker's file
// substituted at the source name would pass it just as well. dstPath is used
// for messages only -- nothing here resolves it.
//
// A failure once the rename has replaced the destination is wrapped in
// ErrDestinationCommitted; nothing is rolled back.
func moveOpenFileCore(file File, srcDirFd int, srcName string, dstDirFd int, dstName, dstPath string, requiredPerm os.FileMode, gm *groupmembership.GroupMembership) error {
	// fchmod rather than os.Chmod: the fd cannot be redirected by a symlink
	// swapped in at the source name.
	if err := file.Chmod(requiredPerm); err != nil {
		return fmt.Errorf("failed to set secure permissions on source: %w", err)
	}

	// The destination write policy is applied to this fd before the rename:
	// the inode is the same, so the verdict is the same, but a refusal no
	// longer arrives over a destination that has already been replaced.
	if err := canSafelyAccessFile(file, dstPath, subjectPendingDestination, groupmembership.FileOpWrite, gm); err != nil {
		return fmt.Errorf("destination file validation failed: %w", err)
	}

	if err := moveFileAnchored(file, srcDirFd, srcName, dstDirFd, dstName); err != nil {
		// Only the move knows which side of the rename it failed on; asking
		// the destination instead would read a name taken over right after our
		// rename as "nothing happened".
		if errors.Is(err, errRenameCommitted) {
			return commitFailure(fmt.Errorf("atomic move failed after the destination was replaced: %w", err), dstPath)
		}
		return fmt.Errorf("atomic move failed: %w", err)
	}

	// Only identity is left to establish; the permissions were checked on this
	// same inode's fd.
	if err := verifyMovedFile(file, dstDirFd, dstName); err != nil {
		return commitFailure(fmt.Errorf("destination identity check failed after the move: %w", err), dstPath)
	}

	return nil
}

// commitFailure wraps and logs a failure that happened after the rename had
// already replaced the destination.
func commitFailure(err error, dstPath string) error {
	slog.Warn(destinationCommittedMsg,
		slog.String("destination", dstPath),
		slog.Any("error", err))
	return fmt.Errorf("%w: %w", ErrDestinationCommitted, err)
}

// tempFilePrefixStem is carried by every directory entry this package creates
// for its own use, so an entry left behind by a crash is recognisable as this
// package's without knowing which operation left it.
const tempFilePrefixStem = ".safefileio-"

// tempWriteNamePrefix marks the temporary file safeWriteFileCommon writes the
// new content into before renaming it over the destination.
const tempWriteNamePrefix = tempFilePrefixStem + "write-" //nolint:gosec // G101: a directory-entry prefix, not a credential

// tempWritePerm is the mode the temporary file is created with. It is fixed
// rather than taken from the caller: the destination's final mode is set by
// moveOpenFileCore's fchmod once the content is complete, so opening the file
// any wider than the owner would only expose a half-written file.
const tempWritePerm os.FileMode = 0o600

const tempCloseFailedMsg = "failed to close the temporary file after the destination was replaced"

const tempFileLeftBehindMsg = "left the temporary file in place: the destination had already been replaced"

// safeWriteFileCommon contains the common logic for safe file writing
// operations. It writes content to a temporary file in the destination's own
// directory and renames it over the destination, so the destination is only
// ever the content it already had or the content of a completed write, never a
// truncated or half-written one.
//
// The temporary file is created, written, and moved entirely through the
// directory fd taken once at the start; no path is resolved a second time.
func safeWriteFileCommon(filePath common.ResolvedPath, content []byte, perm os.FileMode, fs *osFS) error {
	absPath := filePath.String()
	if absPath == "" {
		return fmt.Errorf("%w: empty path", ErrInvalidFilePath)
	}
	// Require NewResolvedPathParentOnly so the leaf-symlink position is not pre-resolved;
	// the destination probe below can then detect and reject a symlink at the leaf.
	if !filePath.IsParentOnly() {
		return fmt.Errorf("%w: filePath must be created with NewResolvedPathParentOnly", ErrInvalidFilePath)
	}

	if err := fs.GetGroupMembership().ValidateRequestedPermissions(perm, groupmembership.FileOpWrite); err != nil {
		return err
	}

	// This route no longer goes through SafeOpenFile, so it carries
	// SafeOpenFile's mode check itself. The check above cannot stand in for
	// it: it masks with 0o7777 and so reads os.ModeSetuid (1<<23) as a plain
	// 0o600. It runs second so that a mode carrying the POSIX setuid bit
	// (0o4000, which is inside that mask) is still reported as exceeding the
	// permission policy rather than as an unsupported mode.
	if err := validateOpenPerm(perm); err != nil {
		return err
	}

	dir, name := filepath.Dir(absPath), filepath.Base(absPath)

	dirFd, err := fs.openDirNoSymlinks(dir)
	if err != nil {
		return fmt.Errorf("destination parent directory unsafe: %w", err)
	}
	defer closeDirFd(dirFd, dir)

	if err := probeWriteDestination(fs, dirFd, name, absPath); err != nil {
		return err
	}

	tmpFile, tmpName, err := createTempFileInDir(fs, dirFd, tempWriteNamePrefix)
	if err != nil {
		// The caller has never seen the random temporary name, so the failure
		// is reported against the destination it asked for.
		return fmt.Errorf("failed to create a temporary file for %s: %w", absPath, err)
	}

	err = writeAndReplace(tmpFile, dirFd, tmpName, name, absPath, content, perm, fs.GetGroupMembership())
	switch {
	case err == nil:
		closeAfterCommit(tmpFile, absPath)
		// The content is durable, but the entry that publishes it is not until
		// the directory is flushed as well; a failure here is reported the same
		// way as any other post-rename failure, since the destination already
		// holds the new content.
		if syncErr := syncDirEntry(dirFd, dir); syncErr != nil {
			return commitFailure(syncErr, absPath)
		}
		return nil
	case errors.Is(err, ErrDestinationCommitted):
		// The temporary name has either been consumed by the move, or -- if
		// the move failed at removing the source after the rename -- is a
		// second link to the inode now published at the destination. Removing
		// it would be safe in that one case and impossible in the others, and
		// buys nothing over the record below, which lets an operator reconcile
		// a leftover entry with this run.
		closeAfterCommit(tmpFile, absPath)
		slog.Warn(tempFileLeftBehindMsg,
			slog.String("destination", absPath),
			slog.String("temp_file", filepath.Join(dir, tmpName)))
		return err
	default:
		// The destination has not been touched; take the temporary file back
		// out. removeVerifiedFileAt closes the file and records why it could
		// not remove it, so the caller still sees the failure that got here.
		_ = removeVerifiedFileAt(tmpFile, dirFd, dir, tmpName)
		return err
	}
}

// writeAndReplace fills the temporary file and moves it onto the destination
// name. Everything it does is reversible by removing the temporary file, right
// up to the rename inside moveOpenFileCore, which is what makes the caller's
// distinction between a committed and an uncommitted failure exact.
func writeAndReplace(tmpFile File, dirFd int, tmpName, dstName, dstPath string, content []byte, perm os.FileMode, gm *groupmembership.GroupMembership) error {
	if _, err := tmpFile.Write(content); err != nil {
		return fmt.Errorf("failed to write to %s: %w", dstPath, err)
	}

	// The content must be durable before the rename publishes it: without
	// this, a crash could leave the destination naming an inode whose blocks
	// were never written, which is neither the old content nor the new.
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to flush %s to disk: %w", dstPath, err)
	}

	return moveOpenFileCore(tmpFile, dirFd, tmpName, dirFd, dstName, dstPath, perm, gm)
}

// probeWriteDestination applies to an existing destination the checks that used
// to run on the destination fd the write itself opened. Without it a rename
// would silently replace a leaf symlink instead of refusing it, and an existing
// destination the caller may not write would be checked only after it had
// already been replaced.
//
// Opening read-only is what makes those checks possible on the inode rather
// than on a path name; a destination that is writable but not readable is
// refused here, where the previous O_WRONLY open accepted it.
func probeWriteDestination(fs *osFS, dirFd int, name, absPath string) error {
	file, err := fs.openFileAt(dirFd, name, os.O_RDONLY, 0)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Nothing is being replaced, so there is nothing to check.
			return nil
		}
		return fmt.Errorf("failed to open destination file safely: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			slog.Warn("error closing destination probe",
				slog.String("path", absPath),
				slog.Any("error", closeErr))
		}
	}()

	return canSafelyAccessFile(file, absPath, subjectFileAtPath, groupmembership.FileOpWrite, fs.GetGroupMembership())
}

// createTempFileInDir creates a file under a random name in the directory dirFd
// is open on, and returns it with the name it was created under. O_EXCL makes
// the creation itself the point at which the name is claimed, so the returned
// name always belongs to this call; a name already taken is retried under a new
// random one rather than opened.
func createTempFileInDir(fs *osFS, dirFd int, prefix string) (File, string, error) {
	for range maxTempNameAttempts {
		name, err := generateTempName(prefix)
		if err != nil {
			return nil, "", err
		}

		file, err := fs.openFileAt(dirFd, name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, tempWritePerm)
		switch {
		case err == nil:
			return file, name, nil
		case errors.Is(err, ErrFileExists):
			continue
		default:
			return nil, "", err
		}
	}

	return nil, "", fmt.Errorf("%w: after %d attempts", ErrTempNameExhausted, maxTempNameAttempts)
}

// closeAfterCommit closes a temporary file whose content is already at the
// destination. The failure is recorded rather than returned: the write did
// happen, and reporting an error over a destination that holds the new content
// would tell the caller the opposite of what is on disk (see the design
// document's error handling section).
func closeAfterCommit(file File, dstPath string) {
	if err := file.Close(); err != nil {
		slog.Warn(tempCloseFailedMsg,
			slog.String("destination", dstPath),
			slog.Any("error", err))
	}
}

// ensureParentDirsNoSymlinks checks that no component of absPath's parent
// directory is a symlink. It is the form used by callers that only need the
// verdict; see ensureDirNoSymlinks for the walk itself.
func ensureParentDirsNoSymlinks(absPath string) error {
	_, err := ensureDirNoSymlinks(filepath.Dir(absPath))
	return err
}

// ensureDirNoSymlinks checks if any component of dir is a symlink by
// traversing the directory hierarchy step-by-step using opendir(2) equivalent,
// and returns dir with any component it did follow replaced by its target.
//
// Exception: OS-managed symlinks on an explicit allowlist (e.g. /tmp ->
// /private/tmp on macOS) are followed after verifying the target matches the
// expected destination. All other symlinks are rejected to prevent
// symlink-redirect attacks where an attacker substitutes a directory component
// with a symlink to an arbitrary target.
//
// The returned path is what a caller must open: opening dir itself with
// O_NOFOLLOW fails whenever dir's own last component is one of those
// allowlisted symlinks, which would turn a supported layout into an error.
func ensureDirNoSymlinks(dir string) (string, error) {
	components := splitPathComponents(dir)

	// VolumeName keeps the root correct on Windows too: VolumeName("C:\\Users") + "\\" = "C:\\".
	currentPath := filepath.VolumeName(dir) + string(os.PathSeparator)

	for _, component := range components {
		currentPath = filepath.Join(currentPath, component)

		// Lstat, so the component being tested for is not itself followed.
		fi, err := os.Lstat(currentPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Directory doesn't exist yet, which is fine for creation
				continue
			}
			return "", fmt.Errorf("failed to stat %s: %w", currentPath, err)
		}

		if fi.Mode()&os.ModeSymlink != 0 {
			if !isAllowedOSManagedSymlink(currentPath) {
				return "", fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
			}
			// Resolve the OS-managed symlink so subsequent components are
			// evaluated against the real path.
			resolved, err := filepath.EvalSymlinks(currentPath)
			if err != nil {
				return "", fmt.Errorf("failed to resolve OS symlink %s: %w", currentPath, err)
			}
			currentPath = resolved
			continue
		}

		if !fi.IsDir() {
			return "", fmt.Errorf("%w: not a directory: %s", ErrInvalidFilePath, currentPath)
		}
	}

	return currentPath, nil
}

// openDirNoSymlinksFallback opens dir as a directory fd without following a
// symlink, for platforms and configurations without openat2.
//
// It opens the path ensureDirNoSymlinks resolved rather than dir itself: dir's
// own last component may be an allowlisted OS-managed symlink, which O_NOFOLLOW
// would reject.
//
// The check and the open are two steps, so a component can still be replaced in
// between; see the design document's residual-risk table. That window cannot be
// closed without openat2, but it can be detected: verifyDirAfterOpen re-runs the
// walk once the fd is held and abandons the open when the directory it landed on
// is not the one the first walk verified. This is the same two-phase shape
// safeOpenFileFallback uses for a file. Everything done relative to a returned
// fd is free of the window, the parent being pinned to an inode from here on.
func openDirNoSymlinksFallback(dir string) (int, error) {
	resolvedDir, err := ensureDirNoSymlinks(dir)
	if err != nil {
		return -1, err
	}

	fd, err := unix.Open(resolvedDir, unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC|dirAccessFlag, 0)
	if err != nil {
		return -1, fmt.Errorf("failed to open directory %s: %w", resolvedDir, mapDirOpenErrno(err, resolvedDir))
	}

	if err := verifyDirAfterOpen(fd, dir, resolvedDir); err != nil {
		slog.Warn(dirPostOpenCheckFailedMsg,
			slog.String("dir", dir),
			slog.Any("error", err))
		closeDirFd(fd, resolvedDir)
		return -1, err
	}
	return fd, nil
}

// dirPostOpenCheckFailedMsg marks the event that must be recorded whenever the
// second directory check fails, whether or not the close that follows succeeds.
const dirPostOpenCheckFailedMsg = "directory check failed after opening it; abandoning the open"

// verifyDirAfterOpen is the second phase of openDirNoSymlinksFallback: it
// confirms that the directory fd refers to what the first walk verified.
//
// Two things must hold. The walk must still find no symlink and resolve dir to
// the same path -- an intermediate component turned into a symlink is followed
// by unix.Open, which only refuses one at the leaf, so nothing else would notice
// it. And that path must still name the inode the fd holds, since the entry
// could have been replaced by another real directory, which no walk can see.
func verifyDirAfterOpen(fd int, dir, resolvedDir string) error {
	recheckedDir, err := ensureDirAfterOpen(dir)
	if err != nil {
		return err
	}
	if recheckedDir != resolvedDir {
		return fmt.Errorf("%w: %s resolved to %s, now to %s", ErrDirChangedDuringOpen, dir, resolvedDir, recheckedDir)
	}

	var fdStat unix.Stat_t
	if err := unix.Fstat(fd, &fdStat); err != nil {
		return fmt.Errorf("failed to fstat directory file descriptor for %s: %w", resolvedDir, err)
	}
	var pathStat unix.Stat_t
	if err := unix.Lstat(resolvedDir, &pathStat); err != nil {
		return fmt.Errorf("failed to lstat %s: %w", resolvedDir, err)
	}
	if fdStat.Dev != pathStat.Dev || fdStat.Ino != pathStat.Ino {
		return fmt.Errorf("%w: %s no longer refers to the opened directory", ErrDirChangedDuringOpen, resolvedDir)
	}
	return nil
}

// mapDirOpenErrno is mapOpenErrno for a directory open. O_DIRECTORY changes what
// the kernel reports for a symlink -- ENOTDIR on Linux, where the leaf being a
// symlink means it is not the directory that was asked for -- so that case is
// re-examined here rather than passed through as a bare errno, keeping the
// sentinel the same as on the openat2 route.
func mapDirOpenErrno(err error, dir string) error {
	if errors.Is(err, unix.ENOTDIR) {
		if fi, statErr := os.Lstat(dir); statErr == nil && fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrIsSymlink, dir)
		}
	}
	return mapOpenErrno(err)
}

// openFileAtFallback opens a single name relative to an already-open directory
// fd, for platforms and configurations without openat2. O_NOFOLLOW keeps a
// symlink at the leaf rejected rather than followed, and the name is a single
// component, so no path resolution beyond it takes place.
func openFileAtFallback(dirFd int, name string, flag int, perm os.FileMode) (*os.File, error) {
	if err := validateOpenAtName(name); err != nil {
		return nil, err
	}
	if err := validateOpenPerm(perm); err != nil {
		return nil, err
	}

	// #nosec G115 - openPermBits returns nine permission bits at most
	mode := uint32(openPermBits(flag, perm))
	fd, err := unix.Openat(dirFd, name, flag|unix.O_NOFOLLOW|unix.O_CLOEXEC, mode)
	if err != nil {
		return nil, mapOpenErrno(err)
	}
	return os.NewFile(uintptr(fd), name), nil //nolint:gosec // G115: fd is a valid file descriptor returned by the kernel
}

// openPermBits returns the mode to hand open(2), applying the same rule on
// every route: a mode only for an open that can bring an inode into existence,
// zero otherwise. Its callers reject a perm outside os.ModePerm first, so
// perm.Perm() here is a plain copy rather than a lossy narrowing.
func openPermBits(flag int, perm os.FileMode) os.FileMode {
	if !mayCreateFile(flag) {
		return 0
	}
	return perm.Perm()
}

// validateOpenAtName rejects anything that is not a single path component.
// Every operation anchored to a directory fd depends on the name naming an
// entry in that directory and nowhere else; a name carrying a separator, or
// "..", would resolve a path the caller never had checked, and an absolute name
// makes the *at family ignore the directory fd altogether.
//
// filepath.Base is not sufficient on its own: it maps "/" to itself.
func validateOpenAtName(name string) error {
	if name == "" || name == "." || name == ".." || filepath.IsAbs(name) || name != filepath.Base(name) {
		return fmt.Errorf("%w: %q is not a plain file name", ErrInvalidFilePath, name)
	}
	return nil
}

// mapOpenErrno translates the errno values an open can fail with into this
// package's sentinels, so that a caller sees the same error whichever route
// opened the file.
//
// The symlink case is decided by isNoFollowErrno rather than by testing ELOOP
// directly, because O_NOFOLLOW reports a symlink as EFTYPE on NetBSD.
func mapOpenErrno(err error) error {
	switch {
	case errors.Is(err, unix.EEXIST):
		return ErrFileExists
	case errors.Is(err, unix.ENOENT):
		return os.ErrNotExist
	case isNoFollowErrno(err):
		return ErrIsSymlink
	}
	return err
}

// mapRenameErrno keeps one rename failure reading the way it did when this
// package moved files with os.Rename: the destination already existing as a
// directory.
//
// The kernel reports that as EISDIR, but os.Rename lstats the destination first
// and substitutes EEXIST (see os.rename), which is what callers match on --
// internal/runner/base/output asks whether the final path is already taken.
// Both are kept in the chain, EEXIST for the question callers ask and EISDIR
// for the operator reading the message.
func mapRenameErrno(err error) error {
	if errors.Is(err, unix.EISDIR) {
		return fmt.Errorf("%w: destination exists as a directory: %w", unix.EEXIST, err)
	}
	return err
}

// closeDirFd releases a directory fd, recording a failure rather than
// reporting it: by the time this runs the caller's outcome is already decided,
// and a close failure on a directory opened read-only says nothing about it.
func closeDirFd(fd int, dir string) {
	if err := unix.Close(fd); err != nil {
		slog.Warn("failed to close directory file descriptor",
			slog.String("dir", dir),
			slog.Any("error", err))
	}
}

// dirSyncUnreadableMsg records a directory entry left un-fsynced because the
// directory cannot be opened for reading.
const dirSyncUnreadableMsg = "could not flush the destination directory: it is not readable"

// fsyncDirAt makes the directory entry the rename created durable. The
// temporary file's data is already fsynced before the rename, but the entry
// naming it is not: without this, a crash right after a successful write takes
// the destination back to its previous content -- or, on a first write, to not
// existing at all -- after the caller was told the write succeeded.
//
// dirFd itself cannot be fsynced: on Linux it is opened O_PATH (dirAccessFlag),
// which the kernel refuses every I/O operation on. The directory is reopened
// through dirFd under the single name ".", so what gets fsynced is the same
// inode the whole write was anchored to and no path is resolved a second time.
func fsyncDirAt(dirFd int, dir string) error {
	fd, err := unix.Openat(dirFd, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		if errors.Is(err, unix.EACCES) {
			// Read permission on the destination directory is exactly what
			// dirAccessFlag exists to avoid demanding, and a write-only drop
			// directory (0o733 and the like) has none. Losing the entry costs
			// durability, not the invariant -- the destination still reads as
			// its previous content -- so this is recorded rather than turned
			// into a failure for a write that did succeed.
			slog.Warn(dirSyncUnreadableMsg, slog.String("dir", dir))
			return nil
		}
		return fmt.Errorf("failed to open the destination directory %s for flushing: %w", dir, err)
	}
	defer closeDirFd(fd, dir)

	if err := unix.Fsync(fd); err != nil {
		return fmt.Errorf("failed to flush the destination directory %s: %w", dir, err)
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
	fdStat, err := fdStatOf(file)
	if err != nil {
		return err
	}

	var pathStat unix.Stat_t
	if err := unix.Lstat(path, &pathStat); err != nil {
		return fmt.Errorf("failed to lstat path %q: %w", path, err)
	}

	return compareInode(fdStat, &pathStat, path)
}

// verifySameFileAt is verifySameFile for a caller that names its target as an
// entry of an already-open directory, using fstatat with AT_SYMLINK_NOFOLLOW in
// place of lstat. Only the final name can be swapped under such a caller, the
// parent being pinned to an inode by the fd.
func verifySameFileAt(file File, dirFd int, name string) error {
	if err := validateOpenAtName(name); err != nil {
		return err
	}

	fdStat, err := fdStatOf(file)
	if err != nil {
		return err
	}

	var entryStat unix.Stat_t
	if err := unix.Fstatat(dirFd, name, &entryStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return fmt.Errorf("failed to fstatat entry %q: %w", name, err)
	}

	return compareInode(fdStat, &entryStat, name)
}

func fdStatOf(file File) (*syscall.Stat_t, error) {
	fdInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to fstat open file descriptor: %w", err)
	}
	fdStat, ok := fdInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported file info type for fd", ErrUnsupportedFileHandle)
	}
	return fdStat, nil
}

// compareInode is the single comparison behind both verifySameFile forms.
// target names the entry in the message only; the comparison itself is on
// (dev, ino). The two Stat_t types agree field by field on every platform this
// package builds for, so no conversion is involved.
func compareInode(fdStat *syscall.Stat_t, entryStat *unix.Stat_t, target string) error {
	if fdStat.Dev != entryStat.Dev || fdStat.Ino != entryStat.Ino {
		return fmt.Errorf("%w: %q no longer refers to the expected inode", ErrSourceIdentityMismatch, target)
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

// removeVerifiedFileAt is removeVerifiedFileByPath for a caller that names its
// target as an entry of an already-open directory: the check is an fstatat and
// the removal an unlinkat, both relative to dirFd, so only the final name is
// exposed to being swapped and the parent is pinned to an inode throughout.
//
// file is always closed, whether or not the removal happens. The returned error
// says why the removal was skipped or failed; a caller already on a failure
// path must not return it in place of the failure that brought it here, which
// this function has recorded for it. dir names the directory dirFd is open on;
// it is used to make the record locatable and nothing is resolved through it.
func removeVerifiedFileAt(file File, dirFd int, dir, name string) error {
	path := filepath.Join(dir, name)
	verifyErr := verifySameFileAt(file, dirFd, name)
	closeErr := file.Close()

	if verifyErr != nil {
		// Reached both when the entry is known to be a different inode and
		// when the comparison could not be made at all. Both leave it alone:
		// removing whatever sits there now would turn a detected substitution
		// into a deletion the substituter chose.
		slog.Warn("left a temporary file in place: could not confirm it still refers to the inode that was written",
			withCloseError(closeErr,
				slog.String("path", path),
				slog.Any("error", verifyErr))...)
		return verifyErr
	}

	if err := unix.Unlinkat(dirFd, name, 0); err != nil {
		slog.Warn("failed to remove the temporary file left by a failed write",
			withCloseError(closeErr,
				slog.String("path", path),
				slog.Any("error", err))...)
		return err
	}

	if closeErr != nil {
		slog.Warn("failed to close the temporary file removed after a failed write",
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

	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}
	return components
}

// MaxFileSize is the maximum allowed file size for SafeReadFile (128 MB)
const MaxFileSize = 128 * 1024 * 1024

// SafeReadFile reads a file safely after validating the path and checking file properties.
// It enforces a maximum file size of MaxFileSize to prevent memory exhaustion attacks.
// It uses openat2 with RESOLVE_NO_SYMLINKS when available for atomic symlink-safe operations;
// see the package documentation for how strong that protection is on each route.
func SafeReadFile(filePath common.ResolvedPath) ([]byte, error) {
	return SafeReadFileWithFS(filePath, defaultFS)
}

// SafeReadFileWithFS is the internal implementation that accepts a FileSystem for testing
func SafeReadFileWithFS(filePath common.ResolvedPath, fs FileSystem) ([]byte, error) {
	absPath := filePath.String()
	if absPath == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidFilePath)
	}

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

	// MaxFileSize+1 so that a file that grew past the limit since the Stat
	// above is caught by the check below rather than silently truncated.
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
func getFileStatInfo(file File, filePath string) (os.FileInfo, *syscall.Stat_t, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, filePath)
	}

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, nil, fmt.Errorf("%w: failed to get file stat info", ErrInvalidFilePath)
	}

	return fileInfo, stat, nil
}

// accessSubject says what the fd handed to canSafelyAccessFile is, relative to
// the path logged alongside it, so a rejection record is readable as what it is.
type accessSubject int

const (
	// subjectFileAtPath: the fd is the file the path currently names.
	subjectFileAtPath accessSubject = iota
	// subjectPendingDestination: the fd is a file about to be moved to the
	// path, so the path may name something else, or nothing, right now.
	subjectPendingDestination
)

func (s accessSubject) String() string {
	if s == subjectPendingDestination {
		return "pending-destination"
	}
	return "file-at-path"
}

// canSafelyAccessFile checks if the current user can safely access a file by validating
// file permissions, ownership, and group membership in a unified security check.
// It verifies the file is a regular file and uses groupmembership to validate permissions for the operation.
func canSafelyAccessFile(file File, filePath string, subject accessSubject, operation groupmembership.FileOperation, groupMembership *groupmembership.GroupMembership) error {
	fileInfo, stat, err := getFileStatInfo(file, filePath)
	if err != nil {
		return err
	}

	var (
		allowed   bool
		policyErr error
		opName    string
		verb      string
	)
	switch operation {
	case groupmembership.FileOpRead:
		opName, verb = "read", "read from"
		allowed, policyErr = groupMembership.CanCurrentUserSafelyReadFile(stat.Gid, fileInfo.Mode())
	case groupmembership.FileOpWrite:
		opName, verb = "write", "write to"
		allowed, policyErr = groupMembership.CanCurrentUserSafelyWriteFile(stat.Uid, stat.Gid, fileInfo.Mode())
	default:
		return fmt.Errorf("%w: unknown operation type", ErrInvalidFileOperation)
	}

	if policyErr != nil {
		logPermissionRejection(filePath, opName, subject, fileInfo, stat, operation, policyErr)
		return fmt.Errorf("%w: %s - %w", ErrInvalidFilePermissions, filePath, policyErr)
	}
	if !allowed {
		logPermissionRejection(filePath, opName, subject, fileInfo, stat, operation, nil)
		return fmt.Errorf("%w: %s - current user cannot safely %s this file",
			ErrInvalidFilePermissions, filePath, verb)
	}

	return nil
}

// permissionRejectionMsg marks a file refused on its permissions. The move
// path's reordered checks refuse sources that used to be narrowed and then
// accepted, so the record must say which file and which rule.
const permissionRejectionMsg = "refused a file on its permissions"

// logPermissionRejection records what an operator needs to act on a refusal.
func logPermissionRejection(filePath, opName string, subject accessSubject, fileInfo os.FileInfo, stat *syscall.Stat_t, operation groupmembership.FileOperation, cause error) {
	attrs := []any{
		slog.String("path", filePath),
		slog.String("subject", subject.String()),
		slog.String("operation", opName),
		slog.String("mode", fmt.Sprintf("%04o", fileInfo.Mode().Perm())),
		slog.Uint64("uid", uint64(stat.Uid)),
		slog.Uint64("gid", uint64(stat.Gid)),
		slog.String("rule", rejectionRule(operation, cause)),
	}
	if cause != nil {
		attrs = append(attrs, slog.Any("error", cause))
	}
	slog.Warn(permissionRejectionMsg, attrs...)
}

// rejectionRule names the policy that refused a file, so records can be told
// apart without reading the message the policy happened to produce.
//
// The names come from groupmembership's sentinels, with one exception it cannot
// supply: the write policy refuses a group-writable file whose group has another
// member by returning false with no error. That is the only way either policy
// answers false without an error, which is what makes naming it from the
// operation sound.
func rejectionRule(operation groupmembership.FileOperation, cause error) string {
	switch {
	case errors.Is(cause, groupmembership.ErrFileWorldWritable):
		return "world-writable"
	case errors.Is(cause, groupmembership.ErrGroupWritableNonMember):
		return "group-writable-non-member"
	case errors.Is(cause, groupmembership.ErrPermissionsExceedMaximum):
		return "permissions-exceed-maximum"
	case errors.Is(cause, groupmembership.ErrFileNotOwner):
		return "not-owner"
	case errors.Is(cause, groupmembership.ErrFileNotWritable):
		return "not-writable"
	case cause == nil && operation == groupmembership.FileOpWrite:
		return "group-writable-not-sole-member"
	default:
		return "unknown"
	}
}

// canSafelyReadFromFile checks if the current user can safely read from a file with
// more relaxed permissions than write operations. It is canSafelyAccessFile's
// read check with the file's os.FileInfo handed back, which the read path needs
// for the size limit.
//
// The read check judges on (gid, mode) alone and deliberately does not look at
// the owner's UID, where the write check does. A reader that is not the owner
// is a requirement of the separated record/run setup, not an accident: the hash
// files are deliberately not owned by the account that runs the commands, so
// that account cannot rewrite its own hashes. Requiring the owner to match
// would stop the production configuration from working at all, along with the
// ordinary case of a non-root runner reading root-owned binaries and
// configuration.
//
// What limits the damage is the directory permission audit in internal/security
// rather than anything here: a directory's owner can swap the entry for a
// different file altogether, so an owner check on the file would be worth
// little without it. That audit is not unconditional -- a directory with no
// owner write bit, for one, is not owner-checked at all -- so it is defence in
// depth, not a guarantee.
//
// This is also why a source must never be reopened by path name once it has
// been verified through an fd: a file an attacker substituted at that path
// would pass the read check.
func canSafelyReadFromFile(file File, filePath string, groupMembership *groupmembership.GroupMembership) (os.FileInfo, error) {
	fileInfo, _, err := getFileStatInfo(file, filePath)
	if err != nil {
		return nil, err
	}

	if err := canSafelyAccessFile(file, filePath, subjectFileAtPath, groupmembership.FileOpRead, groupMembership); err != nil {
		return nil, err
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

	if err := ensureParentDirsAfterOpen(absPath); err != nil {
		slog.Warn(postOpenCheckFailedMsg,
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

// giveUpOpeningMsg marks the one outcome of the creation probe that a caller
// cannot tell from the returned error: the loop having run to exhaustion,
// rather than a single failure being handed straight back. Naming it lets a
// test tell those apart too.
const giveUpOpeningMsg = "gave up opening the file: it kept disappearing between the creation probe and the reopen"

// postOpenCheckFailedMsg marks the event that must be recorded whether or not
// the cleanup that follows it succeeds: a path component changed while the file
// was being opened, which is what an attack in progress looks like from here.
const postOpenCheckFailedMsg = "path check failed after opening the file; abandoning the open"

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
	slog.Warn(giveUpOpeningMsg,
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
