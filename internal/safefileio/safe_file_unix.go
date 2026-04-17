//go:build !windows && !darwin

package safefileio

// isAllowedOSManagedSymlink always returns false on non-Darwin Unix systems.
// Linux and other Unix variants do not use OS-managed directory symlinks in
// the parent path, so every symlink is treated as untrusted and rejected by
// ensureParentDirsNoSymlinks.
func isAllowedOSManagedSymlink(_ string) bool {
	return false
}
