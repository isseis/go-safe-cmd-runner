//go:build linux

// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
//
// This file contains Linux-specific implementation using openat2 system call
// for atomic symlink-safe file operations.
package safefileio

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
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

// openHow struct for openat2 system call
type openHow struct {
	flags   uint64
	mode    uint64
	resolve uint64
}

const testFilePerm = 0o600 // Read/write for owner only

// isOpenat2Available checks if openat2 system call is available and working
func isOpenat2Available() bool {
	// Create a temporary directory for testing
	testDir, err := os.MkdirTemp("", "openat2test")
	if err != nil {
		return false
	}
	defer func() {
		if err := os.RemoveAll(testDir); err != nil {
			slog.Warn("failed to remove test directory", slog.Any("error", err), slog.String("path", testDir))
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

	return err == nil
}

// mayCreateFile reports whether an open with these flags can bring a new inode
// into existence, which is exactly when open(2) reads the mode argument.
//
// O_TMPFILE must be tested for equality, not for a non-zero intersection: the
// constant is a bit pattern that includes O_DIRECTORY, so a plain directory
// open would match an intersection test and be misread as creating.
func mayCreateFile(flag int) bool {
	return flag&os.O_CREATE != 0 || flag&unix.O_TMPFILE == unix.O_TMPFILE
}

// openat2Mode returns the value to place in openHow.mode. It exists to keep
// the openat2 and fallback paths in agreement, in both directions:
//
// For a non-creating open, openat2(2) rejects a non-zero mode with EINVAL,
// while os.OpenFile hands the same value to a kernel that ignores it. Zeroing
// it here is what removes that divergence.
//
// For a creating open the kernel does apply the mode, O_TMPFILE included -- it
// does not reject a zero mode there, it creates a 0000 file, so treating
// O_TMPFILE as non-creating would silently reintroduce the divergence from the
// other side. Verified against Linux 6.12.
func openat2Mode(flag int, perm os.FileMode) uint64 {
	return uint64(openPermBits(flag, perm))
}

// dirAccessFlag is the access mode this package opens a directory with when it
// only means to anchor later operations to it.
//
// O_PATH asks for no access to the directory's contents, which is what the
// openat/renameat/linkat/unlinkat/fstatat calls anchored to the fd need. Opening
// it O_RDONLY instead would demand read permission and so refuse a write-only
// drop directory (mode 0733 and the like) that the path-based code this replaces
// moved into without complaint.
const dirAccessFlag = unix.O_PATH

// openDirNoSymlinks opens dir as a directory fd, refusing to follow a symlink
// at any component. With openat2 that is a single system call, so unlike the
// fallback there is no window between checking the path and opening it.
//
// The caller owns the returned fd and must close it.
func (fs *osFS) openDirNoSymlinks(dir string) (int, error) {
	if !fs.openat2Available {
		return openDirNoSymlinksFallback(dir)
	}

	how := openHow{
		flags:   uint64(unix.O_DIRECTORY | unix.O_CLOEXEC | dirAccessFlag),
		resolve: ResolveNoSymlinks,
	}
	fd, err := openat2(AtFdcwd, dir, &how)
	if err != nil {
		return -1, fmt.Errorf("failed to open directory %s: %w", dir, mapOpenErrno(err))
	}
	return fd, nil
}

// openFileAt opens a single name relative to an already-open directory fd. The
// directory is pinned to an inode by the fd, and name is one component, so the
// only thing an open can reach is an entry of the directory the caller checked.
func (fs *osFS) openFileAt(dirFd int, name string, flag int, perm os.FileMode) (*os.File, error) {
	if !fs.openat2Available {
		return openFileAtFallback(dirFd, name, flag, perm)
	}
	if err := validateOpenAtName(name); err != nil {
		return nil, err
	}
	if err := validateOpenPerm(perm); err != nil {
		return nil, err
	}

	how := openHow{
		// #nosec G115 - flag conversion is intentional and safe within valid flag range
		flags:   uint64(flag | unix.O_CLOEXEC),
		mode:    openat2Mode(flag, perm),
		resolve: ResolveNoSymlinks,
	}
	fd, err := openat2(dirFd, name, &how)
	if err != nil {
		return nil, mapOpenErrno(err)
	}
	return os.NewFile(uintptr(fd), name), nil //nolint:gosec // G115: fd is a valid file descriptor returned by the kernel
}

// openat2 wraps the openat2 system call, retrying while the kernel reports
// EINTR. Every other errno is passed through untouched, leaving the mapping in
// safeOpenFileInternal to interpret it.
//
// The retry count is deliberately unbounded. EINTR means the open was
// interrupted before it took effect, so retrying is a redo rather than a
// repeated operation; the Go standard library retries open the same way. In
// this package the interruption comes from the runtime's asynchronous
// preemption signal (SIGURG) rather than from anything an attacker controls.
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
	for {
		fd, err := openat2Syscall(dirfd, pathname, how)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		return fd, err
	}
}

// rawOpenat2 issues exactly one openat2 system call, with no retry handling.
func rawOpenat2(dirfd int, pathname string, how *openHow) (int, error) {
	pathBytes, err := syscall.BytePtrFromString(pathname)
	if err != nil {
		return -1, err
	}

	fd, _, errno := syscall.Syscall6(
		SysOpenat2,
		uintptr(dirfd), //nolint:gosec // G115: dirfd is a valid file descriptor, conversion is safe
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

	return int(fd), nil //nolint:gosec // G115: fd is a valid file descriptor returned by the kernel, conversion is safe
}

// tempLinkNamePrefix marks the temporary hard link moveFileAnchored creates in
// the destination directory.
const tempLinkNamePrefix = ".safefileio-move-"

// generateTempLinkName produces the name used for the temporary hard link in
// moveFileAnchored. It is a package variable (rather than a direct call to
// randomTempName) so tests can force deterministic name collisions to
// exercise the EEXIST retry path.
var generateTempLinkName = randomTempName

// linkatFunc performs the linkat syscall used by linkFileToTempName. It is a
// package variable (rather than a direct call to unix.Linkat) so tests can
// force deterministic non-EEXIST failures (e.g. EPERM from
// fs.protected_hardlinks, or ETXTBSY) without depending on environment-
// specific privilege setups to trigger them for real.
//
// Tests must not call t.Parallel() in this package: this variable (and
// generateTempLinkName) is mutated by tests and shared package-wide.
var linkatFunc = unix.Linkat

// moveFileAnchored moves the inode referenced by srcFile to dstName in the
// directory dstDirFd is open on. Invariant: whenever a file ends up at the
// destination, it is always the exact inode that srcFile refers to; if that
// identity cannot be established, the function fails closed instead of moving
// anything.
//
// Neither side is named by a path here. Each directory is pinned to an inode by
// its fd and each file by a single name within it, so nothing this function does
// re-resolves a path the caller had checked. The source additionally never
// depends on its name: the move goes through the fd's own inode.
//
// It hard-links that inode (via /proc/self/fd, which requires
// AT_SYMLINK_FOLLOW to dereference the magic symlink to the real inode) into
// the destination directory under a random temporary name, renames the
// temporary name to dstName within that same directory (atomic replace), and
// then unlinks the source entry.
//
// Note on what happens if the source entry is replaced between the open and
// this call: the Linux kernel refuses to give a new name via /proc/self/fd to a
// regular (non-O_TMPFILE) file once its last directory entry has been
// removed (nlink reaches 0) -- see may_linkat in the kernel. Replacing the
// source's directory entry (unlink+recreate, or renaming another file over
// it) drops the originally verified inode's nlink to 0, so the hard-link
// step below fails with ENOENT before any rename or unlink runs. The
// practical effect is fail-closed by construction: a replaced source can
// never reach the destination, but the mechanism does not recover the
// pre-replacement content either -- it errors out. See the design document's
// rationale on this kernel constraint for the full explanation.
//
// On any failure before the rename below succeeds, no file is left at the
// destination and any temporary hard link created along the way is removed
// (fail-closed, no partial move). Once the rename has succeeded, a
// subsequent failure (verifySameFileAt mismatch, or source removal failure)
// intentionally leaves the destination populated with the moved content rather
// than undoing the rename.
func moveFileAnchored(srcFile File, srcDirFd int, srcName string, dstDirFd int, dstName string) (err error) {
	osFile, ok := srcFile.(*os.File)
	if !ok || osFile == nil {
		return fmt.Errorf("%w: source file handle does not support fd-anchored move", ErrUnsupportedFileHandle)
	}
	if err := validateOpenAtName(dstName); err != nil {
		return err
	}

	tmpName, err := linkFileToTempName(osFile, dstDirFd)
	if err != nil {
		return fmt.Errorf("failed to hard-link source inode into destination directory: %w", err)
	}
	defer func() {
		if err != nil {
			if rmErr := unix.Unlinkat(dstDirFd, tmpName, 0); rmErr != nil && !errors.Is(rmErr, unix.ENOENT) {
				slog.Warn("failed to remove leaked temporary hard link", slog.Any("error", rmErr), slog.String("name", tmpName))
			}
		}
	}()

	if err = unix.Renameat(dstDirFd, tmpName, dstDirFd, dstName); err != nil {
		return fmt.Errorf("failed to rename temporary hard link to destination: %w", mapRenameErrno(err))
	}

	// The source entry is removed by name, so confirm it still names the inode
	// that was moved before removing it.
	if err = verifySameFileAt(osFile, srcDirFd, srcName); err != nil {
		return fmt.Errorf("refusing to remove source path after move: %w", err)
	}

	if err = unix.Unlinkat(srcDirFd, srcName, 0); err != nil {
		return fmt.Errorf("failed to remove original source path after move: %w", err)
	}

	return nil
}

// linkFileToTempName hard-links the inode referenced by srcFile into the
// directory dstDirFd is open on, under a random, previously-unused name, and
// returns that name. Using /proc/self/fd/<n> as the link source (with
// AT_SYMLINK_FOLLOW) binds the link to the fd's inode rather than to any path
// name, and the destination side is a name under a directory fd, so no path is
// resolved on either side.
func linkFileToTempName(srcFile *os.File, dstDirFd int) (string, error) {
	procPath := fmt.Sprintf("/proc/self/fd/%d", srcFile.Fd())

	for range maxTempNameAttempts {
		name, err := generateTempLinkName(tempLinkNamePrefix)
		if err != nil {
			return "", err
		}
		if err := validateOpenAtName(name); err != nil {
			return "", err
		}

		err = linkatFunc(unix.AT_FDCWD, procPath, dstDirFd, name, unix.AT_SYMLINK_FOLLOW)
		switch {
		case err == nil:
			return name, nil
		case errors.Is(err, unix.EEXIST):
			continue
		default:
			return "", err
		}
	}

	return "", fmt.Errorf("%w: after %d attempts", ErrTempNameExhausted, maxTempNameAttempts)
}

// safeOpenFileInternal implements Linux-specific file opening with openat2 support.
// It attempts to use the openat2 system call with RESOLVE_NO_SYMLINKS for atomic
// symlink-safe operations. When openat2 is unavailable or disabled, it falls back
// to safeOpenFileFallback which performs two-phase verification.
func (fs *osFS) safeOpenFileInternal(absPath string, flag int, perm os.FileMode) (*os.File, error) {
	if !fs.openat2Available {
		// Fall back to the portable method when openat2 is not available
		return safeOpenFileFallback(absPath, flag, perm)
	}

	// Use openat2 with RESOLVE_NO_SYMLINKS for atomic operation
	how := openHow{
		// O_CLOEXEC matches every other open in this package, including the
		// os.OpenFile the fallback route below reaches: the runner forks and
		// execs, and a descriptor this package opened must not be inherited by a
		// command it runs.
		// #nosec G115 - flag conversion is intentional and safe within valid flag range
		flags:   uint64(flag | unix.O_CLOEXEC),
		mode:    openat2Mode(flag, perm),
		resolve: ResolveNoSymlinks,
	}

	fd, err := openat2(AtFdcwd, absPath, &how)
	if err != nil {
		return nil, mapOpenErrno(err)
	}
	return os.NewFile(uintptr(fd), absPath), nil //nolint:gosec // G115: fd is a valid file descriptor, conversion is safe
}
