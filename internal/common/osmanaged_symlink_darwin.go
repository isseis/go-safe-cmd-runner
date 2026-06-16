//go:build darwin

package common

import (
	"os"
	"path/filepath"
)

// osManagedSymlinks is an explicit allowlist of macOS system-installed top-level
// directory symlinks (firmlinks / synthetic-root layout) and the exact target
// string os.Readlink must return for each. macOS readlink returns a RELATIVE
// target (e.g. "private/tmp", not "/private/tmp"), so the expected values are
// intentionally relative.
//
//	/tmp -> private/tmp
//	/var -> private/var
//	/etc -> private/etc
var osManagedSymlinks = map[string]string{
	"/tmp": "private/tmp",
	"/var": "private/var",
	"/etc": "private/etc",
}

// IsAllowedOSManagedSymlink reports whether path is a known macOS OS-managed
// symlink whose os.Readlink target exactly matches the expected allowlist entry.
// Verifying the target prevents an attacker-substituted symlink from being
// trusted. It returns false on any error and for any non-allowlisted path, so
// callers keep their reject-by-default behavior. It is the single implementation
// shared by safefileio and the output-path validator.
func IsAllowedOSManagedSymlink(path string) bool {
	expectedTarget, ok := osManagedSymlinks[filepath.Clean(path)]
	if !ok {
		return false
	}
	actualTarget, err := os.Readlink(path)
	if err != nil {
		return false
	}
	return actualTarget == expectedTarget
}
