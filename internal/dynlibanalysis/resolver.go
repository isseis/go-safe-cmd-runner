package dynlibanalysis

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
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

// Resolve resolves a DT_NEEDED soname to a filesystem path using the given context.
// The resolution order follows ld.so(8) (DT_RPATH excluded as unsupported):
//  1. LD_LIBRARY_PATH - only if ctx.IncludeLDLibraryPath is true
//  2. OwnRUNPATH     - ctx.OwnRUNPATH ($ORIGIN -> ctx.ParentDir)
//  3. /etc/ld.so.cache
//  4. Default paths (architecture-dependent)
//
// The resolved path is normalized via filepath.EvalSymlinks + filepath.Clean.
func (r *LibraryResolver) Resolve(soname string, ctx *ResolveContext) (string, error) {
	var searchedPaths []string

	// Step 1: LD_LIBRARY_PATH (only at runner time)
	if ctx.IncludeLDLibraryPath {
		// os.Getenv is used directly; YAGNI (no injection needed).
		// Tests should use t.Setenv("LD_LIBRARY_PATH", ...) to control this value.
		ldLibPath := os.Getenv("LD_LIBRARY_PATH")
		if ldLibPath != "" {
			for _, dir := range filepath.SplitList(ldLibPath) {
				candidate := filepath.Join(dir, soname)
				searchedPaths = append(searchedPaths, candidate+" (LD_LIBRARY_PATH)")
				if resolved, err := r.tryResolve(candidate); err == nil {
					return resolved, nil
				}
			}
		}
	}

	// Step 2: OwnRUNPATH
	for _, rp := range ctx.OwnRUNPATH {
		expanded := expandOrigin(rp, ctx.ParentDir)
		candidate := filepath.Join(expanded, soname)
		searchedPaths = append(searchedPaths, candidate+" (RUNPATH)")
		if resolved, err := r.tryResolve(candidate); err == nil {
			return resolved, nil
		}
	}

	// Step 3: ld.so.cache
	if r.cache != nil {
		if cachedPath := r.cache.Lookup(soname); cachedPath != "" {
			searchedPaths = append(searchedPaths, cachedPath+" (ld.so.cache)")
			if resolved, err := r.tryResolve(cachedPath); err == nil {
				return resolved, nil
			}
		}
	}

	// Step 4: Default paths (architecture-dependent)
	for _, dir := range r.archPaths {
		candidate := filepath.Join(dir, soname)
		searchedPaths = append(searchedPaths, candidate+" (default)")
		if resolved, err := r.tryResolve(candidate); err == nil {
			return resolved, nil
		}
	}

	return "", &ErrLibraryNotResolved{
		SOName:      soname,
		ParentPath:  ctx.ParentPath,
		SearchPaths: searchedPaths,
	}
}

// tryResolve checks if the candidate path exists and resolves it via
// filepath.EvalSymlinks + filepath.Clean for normalization.
func (r *LibraryResolver) tryResolve(candidate string) (string, error) {
	// Check if the file exists
	_, err := os.Lstat(candidate)
	if err != nil {
		return "", err
	}

	// Resolve symlinks and normalize
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks for %s: %w", candidate, err)
	}

	return filepath.Clean(resolved), nil
}
