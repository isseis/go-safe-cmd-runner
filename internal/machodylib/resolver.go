package machodylib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultSearchPaths are the fallback search paths used when an install name
// has no @ token and is not an absolute path. Corresponds to
// DYLD_FALLBACK_LIBRARY_PATH defaults: /usr/local/lib, /usr/lib.
var defaultSearchPaths = []string{
	"/usr/local/lib",
	"/usr/lib",
}

// LibraryResolver resolves Mach-O install names to filesystem paths.
// It implements the subset of dyld's path resolution algorithm needed for
// security verification (FR-3.1.2).
type LibraryResolver struct {
	executableDir string // directory of the main binary (@executable_path expansion)
}

// NewLibraryResolver creates a new resolver.
// executableDir is the directory of the main binary (used for @executable_path).
func NewLibraryResolver(executableDir string) *LibraryResolver {
	return &LibraryResolver{executableDir: executableDir}
}

// Resolve resolves an install name to a filesystem path.
//
// Resolution order (FR-3.1.2):
//  1. Absolute path (starts with /, no @ token): use directly
//  2. @executable_path token: expand to executableDir
//  3. @loader_path token: expand to directory of loaderPath
//  4. @rpath token: try each rpath entry, first existing path wins
//  5. Default search paths: /usr/local/lib, /usr/lib
//
// Returns ErrUnknownAtToken for unrecognized @ prefix tokens.
// Returns ErrLibraryNotResolved if no existing file is found.
//
// The returned path is normalized via filepath.EvalSymlinks + filepath.Clean.
func (r *LibraryResolver) Resolve(installName, loaderPath string, rpaths []string) (string, error) {
	var tried []string

	// Check for @ token
	if strings.HasPrefix(installName, "@") {
		token, suffix := splitAtToken(installName)

		switch token {
		case "@executable_path":
			candidate := filepath.Join(r.executableDir, suffix)
			resolved, err := tryResolve(candidate)
			if err == nil {
				return resolved, nil
			}

			tried = append(tried, candidate)

			return "", &ErrLibraryNotResolved{
				InstallName: installName,
				LoaderPath:  loaderPath,
				Tried:       tried,
			}

		case "@loader_path":
			loaderDir := filepath.Dir(loaderPath)
			candidate := filepath.Join(loaderDir, suffix)
			resolved, err := tryResolve(candidate)
			if err == nil {
				return resolved, nil
			}

			tried = append(tried, candidate)

			return "", &ErrLibraryNotResolved{
				InstallName: installName,
				LoaderPath:  loaderPath,
				Tried:       tried,
			}

		case "@rpath":
			for _, rp := range rpaths {
				// LC_RPATH entries may contain @executable_path or @loader_path
				expandedRpath := r.expandRpathEntry(rp, loaderPath)
				candidate := filepath.Join(expandedRpath, suffix)
				resolved, err := tryResolve(candidate)
				if err == nil {
					return resolved, nil
				}

				tried = append(tried, candidate)
			}

			return "", &ErrLibraryNotResolved{
				InstallName: installName,
				LoaderPath:  loaderPath,
				Tried:       tried,
			}

		default:
			// Unknown @ token (e.g., @loader_rpath, @rpath_fallback)
			return "", &ErrUnknownAtToken{
				InstallName: installName,
				Token:       token,
			}
		}
	}

	// Absolute path (no @ token, starts with /)
	if filepath.IsAbs(installName) {
		resolved, err := tryResolve(installName)
		if err == nil {
			return resolved, nil
		}

		return "", &ErrLibraryNotResolved{
			InstallName: installName,
			LoaderPath:  loaderPath,
			Tried:       []string{installName},
		}
	}

	// Relative name without @ token: search default paths
	basename := filepath.Base(installName)
	for _, dir := range defaultSearchPaths {
		candidate := filepath.Join(dir, basename)
		resolved, err := tryResolve(candidate)
		if err == nil {
			return resolved, nil
		}

		tried = append(tried, candidate)
	}

	return "", &ErrLibraryNotResolved{
		InstallName: installName,
		LoaderPath:  loaderPath,
		Tried:       tried,
	}
}

// expandRpathEntry expands @executable_path and @loader_path tokens
// within an LC_RPATH entry.
func (r *LibraryResolver) expandRpathEntry(rpathEntry, loaderPath string) string {
	if strings.HasPrefix(rpathEntry, "@executable_path") {
		suffix := strings.TrimPrefix(rpathEntry, "@executable_path")
		suffix = strings.TrimPrefix(suffix, "/")

		return filepath.Clean(filepath.Join(r.executableDir, suffix))
	}

	if strings.HasPrefix(rpathEntry, "@loader_path") {
		suffix := strings.TrimPrefix(rpathEntry, "@loader_path")
		suffix = strings.TrimPrefix(suffix, "/")
		loaderDir := filepath.Dir(loaderPath)

		return filepath.Clean(filepath.Join(loaderDir, suffix))
	}

	return rpathEntry
}

// splitAtToken splits an install name like "@rpath/libFoo.dylib" into
// ("@rpath", "libFoo.dylib"). The suffix does not include the leading separator.
func splitAtToken(installName string) (token, suffix string) {
	idx := strings.Index(installName, "/")
	if idx < 0 {
		return installName, ""
	}

	return installName[:idx], installName[idx+1:]
}

// tryResolve checks if the candidate path exists and resolves it via
// filepath.EvalSymlinks + filepath.Clean for normalization.
func tryResolve(candidate string) (string, error) {
	// Check if the file exists; distinguishes not-found from other errors
	// (e.g., permission denied) consistent with the ELF resolver.
	if _, err := os.Lstat(candidate); err != nil {
		return "", err
	}

	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks for %s: %w", candidate, err)
	}

	return filepath.Clean(resolved), nil
}
