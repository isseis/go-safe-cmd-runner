//go:build darwin

package safefileio

import (
	"os"
)

// osManagedSymlinks is an explicit allowlist of macOS system-installed directory
// symlinks and the exact target strings that os.Readlink must return for them.
//
// macOS ships with three top-level symlinks created by the OS installer as part
// of the "firmlink" / synthetic-root layout.  os.Readlink returns a relative
// target (e.g. "private/tmp", not "/private/tmp"), so the expected values here
// are intentionally relative.
//
//	/tmp -> private/tmp
//	/var -> private/var
//	/etc -> private/etc
var osManagedSymlinks = map[string]string{
	"/tmp": "private/tmp",
	"/var": "private/var",
	"/etc": "private/etc",
}

// isAllowedOSManagedSymlink reports whether path is a known macOS OS-managed
// symlink whose target matches the expected entry in osManagedSymlinks.
//
// The check is two-fold:
//  1. The path must appear in the explicit allowlist (osManagedSymlinks).
//  2. The symlink target returned by os.Readlink must exactly equal the
//     expected value recorded in the allowlist.
//
// Verifying the target prevents a scenario where an attacker replaces an
// OS-managed symlink with one pointing somewhere unexpected.
// Returns false on any error to keep the caller's reject-by-default logic.
func isAllowedOSManagedSymlink(path string) bool {
	expectedTarget, ok := osManagedSymlinks[path]
	if !ok {
		return false
	}
	actualTarget, err := os.Readlink(path)
	if err != nil {
		return false
	}
	return actualTarget == expectedTarget
}
