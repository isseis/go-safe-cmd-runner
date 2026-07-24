//go:build linux

// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
//
// This file contains Linux-specific implementation using openat2 system call
// for atomic symlink-safe file operations.
package safefileio

import (
	"crypto/rand"
	"encoding/hex"
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

// openat2 wraps the openat2 system call
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
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

// maxLinkatAttempts bounds retries when a randomly generated temporary link
// name collides with an existing entry (EEXIST). With tmpNameRandBytes of
// entropy, a real collision is astronomically unlikely; this only guards
// against pathological environments and keeps the loop from running forever.
const maxLinkatAttempts = 10

// tmpNameRandBytes is the number of random bytes used to build a temporary
// link name (see randomTempName).
const tmpNameRandBytes = 12

// generateTempLinkName produces the name used for the temporary hard link in
// moveFileAnchored. It is a package variable (rather than a direct call to
// randomTempName) so tests can force deterministic name collisions to
// exercise the EEXIST retry path.
var generateTempLinkName = randomTempName

// moveFileAnchored moves the inode referenced by srcFile to absDst without
// resolving absSrc by path name at move time. Invariant: whenever a file
// ends up at absDst, it is always the exact inode that srcFile refers to; if
// that identity cannot be established, the function fails closed instead of
// moving anything.
//
// It hard-links the fd's inode (via /proc/self/fd, which requires
// AT_SYMLINK_FOLLOW to dereference the magic symlink to the real inode) into
// absDst's directory under a random temporary name, renames the temporary
// name to absDst within the same directory (atomic replace), and then
// unlinks absSrc.
//
// Note on what happens if absSrc is replaced between SafeOpenFile and this
// call: the Linux kernel refuses to give a new name via /proc/self/fd to a
// regular (non-O_TMPFILE) file once its last directory entry has been
// removed (nlink reaches 0) -- see may_linkat in the kernel. Replacing
// absSrc's directory entry (unlink+recreate, or renaming another file over
// it) drops the originally verified inode's nlink to 0, so the hard-link
// step below fails with ENOENT before any rename or unlink runs. The
// practical effect is fail-closed by construction: a replaced source can
// never reach absDst, but the mechanism does not recover the pre-replacement
// content either -- it errors out. See the design document's rationale on
// this kernel constraint for the full explanation.
//
// On any failure, no file is left at absDst and any temporary hard link
// created along the way is removed (fail-closed, no partial move).
func moveFileAnchored(srcFile File, absSrc, absDst string) (err error) {
	osFile, ok := srcFile.(*os.File)
	if !ok {
		return fmt.Errorf("%w: source file handle does not support fd-anchored move", ErrUnsupportedFileHandle)
	}

	tmpPath, err := linkFileToTempName(osFile, filepath.Dir(absDst))
	if err != nil {
		return fmt.Errorf("failed to hard-link source inode into destination directory: %w", err)
	}
	defer func() {
		if err != nil {
			if rmErr := os.Remove(tmpPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
				slog.Warn("failed to remove leaked temporary hard link", slog.Any("error", rmErr), slog.String("path", tmpPath))
			}
		}
	}()

	if err = os.Rename(tmpPath, absDst); err != nil {
		return fmt.Errorf("failed to rename temporary hard link to destination: %w", err)
	}

	if err = verifySameFile(osFile, absSrc); err != nil {
		return fmt.Errorf("refusing to remove source path after move: %w", err)
	}

	if err = os.Remove(absSrc); err != nil {
		return fmt.Errorf("failed to remove original source path after move: %w", err)
	}

	return nil
}

// verifySameFile checks that the directory entry currently at path still
// refers to the same inode as the already-open fd. It uses Lstat (rather
// than Stat) on path so that a symlink swapped in at path is detected as a
// mismatch instead of being followed. This guards the trailing unlink in
// moveFileAnchored against a TOCTOU race where an attacker replaces absSrc's
// directory entry between the rename above and the unlink below, which would
// otherwise cause the unlink to delete an unrelated file.
func verifySameFile(fd *os.File, path string) error {
	fdInfo, err := fd.Stat()
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
		return fmt.Errorf("%w: path %q no longer refers to the expected inode", ErrInvalidFilePath, path)
	}

	return nil
}

// linkFileToTempName hard-links the inode referenced by srcFile into dstDir
// under a random, previously-unused name and returns the full path of the new
// link. Using /proc/self/fd/<n> as the link source (with AT_SYMLINK_FOLLOW)
// binds the link to the fd's inode rather than to any path name.
func linkFileToTempName(srcFile *os.File, dstDir string) (string, error) {
	procPath := fmt.Sprintf("/proc/self/fd/%d", srcFile.Fd())

	for range maxLinkatAttempts {
		name, err := generateTempLinkName()
		if err != nil {
			return "", err
		}
		tmpPath := filepath.Join(dstDir, name)

		err = unix.Linkat(unix.AT_FDCWD, procPath, unix.AT_FDCWD, tmpPath, unix.AT_SYMLINK_FOLLOW)
		switch {
		case err == nil:
			return tmpPath, nil
		case errors.Is(err, unix.EEXIST):
			continue
		default:
			return "", err
		}
	}

	return "", fmt.Errorf("%w: after %d attempts", ErrTempLinkNameExhausted, maxLinkatAttempts)
}

// randomTempName returns a name unlikely to collide with an existing
// directory entry, prefixed so it is recognizable as safefileio-internal
// state if ever left behind.
func randomTempName() (string, error) {
	var b [tmpNameRandBytes]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("failed to generate random temporary name: %w", err)
	}
	return ".safefileio-move-" + hex.EncodeToString(b[:]), nil
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
		return nil, err
	}
	return os.NewFile(uintptr(fd), absPath), nil //nolint:gosec // G115: fd is a valid file descriptor, conversion is safe
}
