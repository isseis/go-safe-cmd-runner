package dynlib

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveRealPath checks that candidate exists (via os.Lstat, distinguishing
// not-found from other errors such as permission denied) and returns its
// symlink-resolved, cleaned path (via filepath.EvalSymlinks + filepath.Clean).
// The result is absolute when candidate is absolute, which is how the ELF and
// Mach-O resolvers call it (they join an absolute search directory with the
// library name); it does not force a relative candidate to absolute, so it
// introduces no working-directory dependency. It is the single path-resolution
// helper shared by those resolvers.
func ResolveRealPath(candidate string) (string, error) {
	if _, err := os.Lstat(candidate); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks for %s: %w", candidate, err)
	}
	return filepath.Clean(resolved), nil
}
