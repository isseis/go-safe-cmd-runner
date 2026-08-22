//go:build !test

package safefileio

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// This file and test_helpers_overrides.go define the functions below
// exclusively by build tag. The production build deliberately has no function
// value to swap: each of them is a point where a symlink is followed rather
// than refused, or a check that makes an open symlink-safe, so nothing outside
// a test build may widen or redirect it.

func isAllowedOSManagedSymlink(path string) bool {
	return common.IsAllowedOSManagedSymlink(path)
}

// ensureParentDirsAfterOpen runs the second parent-directory check of
// safeOpenFileFallback, the one that detects a component swapped in while the
// file was being opened.
func ensureParentDirsAfterOpen(absPath string) error {
	return ensureParentDirsNoSymlinks(absPath)
}

// verifyMovedFile runs the identity check moveOpenFileCore makes once the
// rename has put the moved inode at its destination.
func verifyMovedFile(file File, dirFd int, name string) error {
	return verifySameFileAt(file, dirFd, name)
}

// syncDirEntry flushes the destination directory once the rename has published
// the new content, making that directory entry durable.
func syncDirEntry(dirFd int, dir string) error {
	return fsyncDirAt(dirFd, dir)
}

// generateTempName produces the random name createTempFileInDir claims with
// O_EXCL.
func generateTempName(prefix string) (string, error) {
	return randomTempName(prefix)
}

// ensureDirAfterOpen runs the second directory check of
// openDirNoSymlinksFallback, the one that detects a component swapped in while
// the directory was being opened.
func ensureDirAfterOpen(dir string) (string, error) {
	return ensureDirNoSymlinks(dir)
}
