//go:build !linux

// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
//
// This file contains the non-Linux platform implementation. openat2 does not
// exist here, so every open takes the fallback route (safeOpenFileFallback),
// which is best-effort rather than race-free. These platforms are for
// development and limited use, not for production; see the package
// documentation in safe_file.go.
package safefileio

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// isOpenat2Available always returns false on non-Linux platforms
func isOpenat2Available() bool {
	return false
}

// mayCreateFile reports whether these flags can bring a new inode into
// existence, which is the only case in which openPermBits passes a mode on.
// O_TMPFILE, which the Linux counterpart also accounts for, does not exist on
// these platforms.
func mayCreateFile(flag int) bool {
	return flag&os.O_CREATE != 0
}

// dirAccessFlag has no O_PATH equivalent here, so a directory must be readable
// to be anchored to: the write-only drop directory Linux accepts is refused on
// these platforms. Non-Linux is a development and limited-use configuration;
// see the design document's residual-risk table.
const dirAccessFlag = unix.O_RDONLY

// openDirNoSymlinks opens dir as a directory fd, refusing to follow a symlink
// at any component. Without openat2 this is the two-step form; see
// openDirNoSymlinksFallback for the window that leaves and why it is accepted.
//
// The caller owns the returned fd and must close it.
func (fs *osFS) openDirNoSymlinks(dir string) (int, error) {
	return openDirNoSymlinksFallback(dir)
}

// openFileAt opens a single name relative to an already-open directory fd. The
// directory is pinned to an inode by the fd, and name is one component, so the
// only thing an open can reach is an entry of the directory the caller checked.
func (fs *osFS) openFileAt(dirFd int, name string, flag int, perm os.FileMode) (*os.File, error) {
	return openFileAtFallback(dirFd, name, flag, perm)
}

// safeOpenFileInternal implements file opening for non-Linux platforms.
// It uses the portable safeOpenFileFallback method which performs two-phase
// verification to detect symlink attacks and TOCTOU race conditions.
func (fs *osFS) safeOpenFileInternal(absPath string, flag int, perm os.FileMode) (*os.File, error) {
	return safeOpenFileFallback(absPath, flag, perm)
}

// moveFileAnchored moves the source entry to dstName in the directory dstDirFd
// is open on. Both directories are pinned to an inode by their fd, so neither
// parent is resolved by path name at move time.
//
// The source, however, is only pinned as far as its name: renameat names what it
// moves, and the fd-anchored hard-link technique that would move srcFile's inode
// itself (see safe_file_linux.go) relies on /proc/self/fd, which is
// Linux-specific. Confirming srcName still refers to srcFile's inode
// immediately before the rename narrows the window in which a substituted
// source could be moved, but the check and the rename remain separate system
// calls, so it does not close it. See the design document's residual-risk
// table.
//
// The rename is the last thing this does, so no failure it reports can have
// left the destination replaced, and none is marked with errRenameCommitted.
func moveFileAnchored(srcFile File, srcDirFd int, srcName string, dstDirFd int, dstName string) error {
	if err := validateOpenAtName(dstName); err != nil {
		return err
	}
	if err := verifySameFileAt(srcFile, srcDirFd, srcName); err != nil {
		return fmt.Errorf("refusing to move source: %w", err)
	}

	if err := unix.Renameat(srcDirFd, srcName, dstDirFd, dstName); err != nil {
		return fmt.Errorf("failed to rename source to destination: %w", mapRenameErrno(err))
	}
	return nil
}
