//go:build darwin

package security

import "path/filepath"

var allowedDarwinSymlinkTargets = map[string]string{
	"/tmp": "/private/tmp",
	"/var": "/private/var",
}

// isAllowedOSManagedSymlink allows known macOS root-level aliases only when
// their resolved target exactly matches the expected destination.
func isAllowedOSManagedSymlink(path string) bool {
	cleanPath := filepath.Clean(path)
	expectedTarget, ok := allowedDarwinSymlinkTargets[cleanPath]
	if !ok {
		return false
	}

	resolvedPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return false
	}

	return filepath.Clean(resolvedPath) == expectedTarget
}
