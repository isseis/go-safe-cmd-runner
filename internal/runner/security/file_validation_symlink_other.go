//go:build !darwin

package security

// isAllowedOSManagedSymlink is darwin-specific. Non-darwin platforms reject all
// symlink components in output-path validation.
func isAllowedOSManagedSymlink(_ string) bool {
	return false
}
