//go:build test

package safefileio

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// isAllowedOSManagedSymlinkOverride is the test override for the OS-managed
// symlink allowlist. It reaches a branch that is otherwise unreachable on
// Linux: common.IsAllowedOSManagedSymlink is hard-coded to false everywhere
// except macOS, so the obligation on ensureDirNoSymlinks to return the resolved
// path -- and on openDirNoSymlinks to open that rather than the original --
// would go untested on the platform this project targets.
//
// It lives here, not next to the production definition, so that the production
// build keeps no swappable function value; see overrides.go.
//
// Like linkatFunc, it is shared package-wide, so tests must not call
// t.Parallel() and must restore it with t.Cleanup.
var isAllowedOSManagedSymlinkOverride = common.IsAllowedOSManagedSymlink

// isAllowedOSManagedSymlink consults the OS-managed symlink allowlist, via the
// test override above.
func isAllowedOSManagedSymlink(path string) bool {
	return isAllowedOSManagedSymlinkOverride(path)
}

// ensureParentDirsAfterOpenOverride is the test override for the second
// parent-directory check of safeOpenFileFallback. It reaches a branch a test
// cannot otherwise produce: the check runs twice over the same path, so failing
// only the second one requires intervening between the two, and there is no
// other point at which to do so.
//
// It lives here, not next to the production definition, so that the production
// build keeps no swappable function value; see overrides.go.
//
// Like linkatFunc, it is shared package-wide, so tests must not call
// t.Parallel() and must restore it with t.Cleanup.
var ensureParentDirsAfterOpenOverride = ensureParentDirsNoSymlinks

// ensureParentDirsAfterOpen runs the second parent-directory check of
// safeOpenFileFallback, via the test override above.
func ensureParentDirsAfterOpen(absPath string) error {
	return ensureParentDirsAfterOpenOverride(absPath)
}

// ensureDirAfterOpenOverride is the test override for the second directory
// check of openDirNoSymlinksFallback. Like the one above, it reaches a branch a
// test cannot otherwise produce: the two checks run over the same path, so
// acting between them -- failing the second, or replacing the directory the fd
// already holds -- requires a hook at that point and nowhere else.
//
// It lives here, not next to the production definition, so that the production
// build keeps no swappable function value; see overrides.go.
//
// Like linkatFunc, it is shared package-wide, so tests must not call
// t.Parallel() and must restore it with t.Cleanup.
var ensureDirAfterOpenOverride = ensureDirNoSymlinks

// ensureDirAfterOpen runs the second directory check of
// openDirNoSymlinksFallback, via the test override above.
func ensureDirAfterOpen(dir string) (string, error) {
	return ensureDirAfterOpenOverride(dir)
}
