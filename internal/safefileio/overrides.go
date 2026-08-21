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

// ensureDirAfterOpen runs the second directory check of
// openDirNoSymlinksFallback, the one that detects a component swapped in while
// the directory was being opened.
func ensureDirAfterOpen(dir string) (string, error) {
	return ensureDirNoSymlinks(dir)
}
