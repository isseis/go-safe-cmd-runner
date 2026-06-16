package dynlib

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveRealPath checks that candidate exists (via os.Lstat, distinguishing
// not-found from other errors such as permission denied) and returns its
// symlink-resolved, cleaned absolute path. It is the single path-resolution
// helper shared by the ELF and Mach-O library resolvers.
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
