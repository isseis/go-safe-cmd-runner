//go:build windows

package safefileio

// isAllowedOSManagedSymlink always returns false on Windows.
// Windows does not use OS-managed directory symlinks in the same way Unix
// does, so every symlink is treated as untrusted and rejected by
// ensureParentDirsNoSymlinks.
func isAllowedOSManagedSymlink(_ string) bool {
	return false
}
