//go:build test

package safefileio

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// The overrides below live here, not next to the production definitions, so
// that the production build keeps no swappable function value; see
// overrides.go. Like linkatFunc, they are shared package-wide, so tests using
// them must not call t.Parallel() and must restore them with t.Cleanup.

// isAllowedOSManagedSymlinkOverride reaches a branch that is otherwise
// unreachable on Linux: common.IsAllowedOSManagedSymlink is hard-coded to false
// everywhere except macOS, so the obligation on ensureDirNoSymlinks to return
// the resolved path -- and on openDirNoSymlinks to open that rather than the
// original -- would go untested on the platform this project targets.
var isAllowedOSManagedSymlinkOverride = common.IsAllowedOSManagedSymlink

func isAllowedOSManagedSymlink(path string) bool {
	return isAllowedOSManagedSymlinkOverride(path)
}

// ensureParentDirsAfterOpenOverride reaches a branch a test cannot otherwise
// produce: the check runs twice over the same path, so failing only the second
// one requires intervening between the two, and there is no other point at
// which to do so.
var ensureParentDirsAfterOpenOverride = ensureParentDirsNoSymlinks

func ensureParentDirsAfterOpen(absPath string) error {
	return ensureParentDirsAfterOpenOverride(absPath)
}

// verifyMovedFileOverride reaches a branch a test cannot otherwise produce: the
// failure has to land after a rename that succeeded, and a move's source and
// destination can share a directory, so taking away the parent's permissions
// stops the rename itself instead.
var verifyMovedFileOverride = verifySameFileAt

func verifyMovedFile(file File, dirFd int, name string) error {
	return verifyMovedFileOverride(file, dirFd, name)
}

// generateTempNameOverride reaches the collision retry in createTempFileInDir
// and the bound that ends it. Neither is reachable otherwise: the names carry
// 12 random bytes, so a collision does not occur, let alone ten in a row.
var generateTempNameOverride = randomTempName

func generateTempName(prefix string) (string, error) {
	return generateTempNameOverride(prefix)
}

// ensureDirAfterOpenOverride reaches a branch a test cannot otherwise produce
// for the same reason, with one more thing to intervene on: the directory the
// fd already holds can be replaced between the two checks.
var ensureDirAfterOpenOverride = ensureDirNoSymlinks

func ensureDirAfterOpen(dir string) (string, error) {
	return ensureDirAfterOpenOverride(dir)
}
