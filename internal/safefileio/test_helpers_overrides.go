//go:build test

package safefileio

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
