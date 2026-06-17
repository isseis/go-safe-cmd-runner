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
	// Clean once and use the cleaned path for both the allowlist lookup and the
	// readlink, so a path-spelling difference (e.g. a trailing slash, which would
	// make os.Readlink resolve through to the target directory instead of reading
	// the symlink) cannot cause the two to disagree.
	cleanPath := filepath.Clean(path)
	expectedTarget, ok := osManagedSymlinks[cleanPath]
	if !ok {
		return false
	}
	actualTarget, err := os.Readlink(cleanPath)
	if err != nil {
		return false
	}
	return actualTarget == expectedTarget
}
