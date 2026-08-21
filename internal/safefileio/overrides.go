//go:build !test

package safefileio

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// isAllowedOSManagedSymlink reports whether a symlink found while walking a
// directory path is one of the OS-managed ones this package follows instead of
// rejecting.
//
// This file and test_helpers_overrides.go define isAllowedOSManagedSymlink
// exclusively by build tag. The production build deliberately has no function
// value to swap: the allowlist is the one place where a symlink is followed
// rather than refused, so nothing outside a test build may widen it.
func isAllowedOSManagedSymlink(path string) bool {
	return common.IsAllowedOSManagedSymlink(path)
}

// ensureParentDirsAfterOpen runs the second parent-directory check of
// safeOpenFileFallback, the one that detects a component swapped in while the
// file was being opened.
//
// This file and test_helpers_overrides.go define ensureParentDirsAfterOpen
// exclusively by build tag. The production build deliberately has no function
// value to swap: this check is what makes the fallback open symlink-safe, so
// nothing outside a test build may redirect it.
func ensureParentDirsAfterOpen(absPath string) error {
	return ensureParentDirsNoSymlinks(absPath)
}

// ensureDirAfterOpen runs the second directory check of
// openDirNoSymlinksFallback, the one that detects a component swapped in while
// the directory was being opened.
//
// This file and test_helpers_overrides.go define ensureDirAfterOpen exclusively
// by build tag, for the reason given above: this check is what makes the
// fallback directory open symlink-safe, so nothing outside a test build may
// redirect it.
func ensureDirAfterOpen(dir string) (string, error) {
	return ensureDirNoSymlinks(dir)
}
