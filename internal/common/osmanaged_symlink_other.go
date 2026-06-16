//go:build !darwin

package common

// IsAllowedOSManagedSymlink always returns false on non-Darwin platforms. Only
// macOS ships OS-managed top-level directory symlinks (firmlinks); everywhere else
// every symlink in a validated path is treated as untrusted.
func IsAllowedOSManagedSymlink(_ string) bool {
	return false
}
