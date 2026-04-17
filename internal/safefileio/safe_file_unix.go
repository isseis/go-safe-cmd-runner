//go:build !windows

package safefileio

import (
	"os"
	"syscall"
)

// isRootOwnedSymlink reports whether the filesystem entry at path is a
// symbolic link that is owned by uid 0 (root / the OS).
//
// Root-owned symlinks are created by the operating system as part of the
// installed directory layout (e.g. /tmp -> /private/tmp on macOS,
// /var -> /private/var on macOS).  User-owned symlinks must be rejected
// because an attacker who controls a directory component can redirect a
// write to an arbitrary location.
//
// Uses os.Lstat so the symlink itself (not its target) is inspected.
// Returns false on any error to keep the caller's reject-by-default logic.
func isRootOwnedSymlink(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return false // not a symlink
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return false
	}
	return stat.Uid == 0
}
