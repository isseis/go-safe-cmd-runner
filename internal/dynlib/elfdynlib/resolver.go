package elfdynlib

import (
	"debug/elf"
	"path/filepath"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib"
)

// LibraryResolver resolves DT_NEEDED library names to filesystem paths.
// It implements a subset of the ld.so resolution algorithm sufficient for
// security verification purposes. DT_RPATH is not supported; ELF files
// containing DT_RPATH are rejected at Analyze time with ErrDTRPATHNotSupported.
type LibraryResolver struct {
	cache     *LDCache // parsed /etc/ld.so.cache (may be nil)
	archPaths []string // architecture-specific default search paths
}

// NewLibraryResolver creates a new resolver for a specific ELF machine architecture.
// cache is the pre-parsed ld.so.cache (may be nil if unavailable).
// Passing the cache as a parameter allows DynLibAnalyzer to parse it once and
// share it across all Analyze() calls.
func NewLibraryResolver(cache *LDCache, elfMachine elf.Machine) *LibraryResolver {
	return &LibraryResolver{
		cache:     cache,
		archPaths: DefaultSearchPaths(elfMachine),
	}
}

// Resolve resolves a DT_NEEDED soname to a filesystem path using the given
// parentPath and runpath.
// The resolution order follows ld.so(8) (DT_RPATH and LD_LIBRARY_PATH excluded):
//  1. runpath ($ORIGIN -> filepath.Dir(parentPath))
//  2. /etc/ld.so.cache
//  3. Default paths (architecture-dependent)
//
// LD_LIBRARY_PATH is not consulted: record ignores it, runner clears it.
// The resolved path is normalized via filepath.EvalSymlinks + filepath.Clean.
func (r *LibraryResolver) Resolve(soname string, parentPath string, runpath []string) (string, error) {
	var searchedPaths []string

	// Step 1: RUNPATH
	for _, rp := range runpath {
		expanded := expandOrigin(rp, filepath.Dir(parentPath))
		candidate := filepath.Join(expanded, soname)
		searchedPaths = append(searchedPaths, candidate+" (RUNPATH)")
		if resolved, err := dynlib.ResolveRealPath(candidate); err == nil {
			return resolved, nil
		}
	}

	// Step 2: ld.so.cache
	if r.cache != nil {
		if cachedPath := r.cache.Lookup(soname); cachedPath != "" {
			searchedPaths = append(searchedPaths, cachedPath+" (ld.so.cache)")
			if resolved, err := dynlib.ResolveRealPath(cachedPath); err == nil {
				return resolved, nil
			}
		}
	}

	// Step 3: Default paths (architecture-dependent)
	for _, dir := range r.archPaths {
		candidate := filepath.Join(dir, soname)
		searchedPaths = append(searchedPaths, candidate+" (default)")
		if resolved, err := dynlib.ResolveRealPath(candidate); err == nil {
			return resolved, nil
		}
	}

	return "", &ErrLibraryNotResolved{
		SOName:      soname,
		ParentPath:  parentPath,
		SearchPaths: searchedPaths,
	}
}

// expandOrigin replaces $ORIGIN and ${ORIGIN} in the given path with the
// specified directory. glibc accepts both syntaxes (see ld.so(8)).
func expandOrigin(path string, originDir string) string {
	result := strings.ReplaceAll(path, "${ORIGIN}", originDir)
	return strings.ReplaceAll(result, "$ORIGIN", originDir)
}
