//go:build windows

package executor

// defaultIdentityChecker is a no-op on Windows where Unix UID/GID concepts do not apply.
func defaultIdentityChecker() error {
	return nil
}
