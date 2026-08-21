//go:build !test

package safefileio

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
