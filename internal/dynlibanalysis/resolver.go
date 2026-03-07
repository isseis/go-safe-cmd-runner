package dynlibanalysis

import (
	"debug/elf"
	"fmt"
	"os"
	"path/filepath"
)

// LibraryResolver resolves DT_NEEDED library names to filesystem paths.
// It implements a subset of the ld.so resolution algorithm sufficient for
// security verification purposes.
type LibraryResolver struct {
	cache     *LDCache // parsed /etc/ld.so.cache (may be nil)
	archPaths []string // architecture-specific default search paths
}

// NewLibraryResolver creates a new resolver for a specific ELF machine architecture.
// cache is the pre-parsed ld.so.cache (may be nil if unavailable).
// Passing the cache as a parameter allows DynLibAnalyzer to parse it once and
// share it across all Analyze() calls.
// fs is not needed: tryResolve uses os.Lstat + filepath.EvalSymlinks directly,
// which is appropriate for path existence checks (safefileio is for content reads).
func NewLibraryResolver(cache *LDCache, elfMachine elf.Machine) *LibraryResolver {
	return &LibraryResolver{
		cache:     cache,
		archPaths: DefaultSearchPaths(elfMachine),
	}
}

// Resolve resolves a DT_NEEDED soname to a filesystem path using the given context.
// The resolution order follows ld.so(8) with inherited RPATH support:
//  1. OwnRPATH     - ctx.OwnRPATH, only when ctx.OwnRUNPATH is absent
//  2. InheritedRPATH - ctx.InheritedRPATH from ancestor ELFs
//  3. LD_LIBRARY_PATH - only if ctx.IncludeLDLibraryPath is true
//  4. OwnRUNPATH   - ctx.OwnRUNPATH ($ORIGIN -> ctx.ParentDir)
//  5. /etc/ld.so.cache
//  6. Default paths (architecture-dependent)
//
// The resolved path is normalized via filepath.EvalSymlinks + filepath.Clean.
func (r *LibraryResolver) Resolve(soname string, ctx *ResolveContext) (string, error) {
	var searchedPaths []string

	// Step 1: OwnRPATH (only when RUNPATH is absent)
	if len(ctx.OwnRUNPATH) == 0 && len(ctx.OwnRPATH) > 0 {
		for _, rp := range ctx.OwnRPATH {
			expanded := expandOrigin(rp, ctx.ParentDir)
			candidate := filepath.Join(expanded, soname)
			searchedPaths = append(searchedPaths, candidate+" (RPATH)")
			if resolved, err := r.tryResolve(candidate); err == nil {
				return resolved, nil
			}
		}
	}

	// Step 2: InheritedRPATH
	for _, entry := range ctx.InheritedRPATH {
		expanded := expandOrigin(entry.Path, entry.OriginDir)
		candidate := filepath.Join(expanded, soname)
		searchedPaths = append(searchedPaths, candidate+" (inherited RPATH)")
		if resolved, err := r.tryResolve(candidate); err == nil {
			return resolved, nil
		}
	}

	// Step 3: LD_LIBRARY_PATH (only at runner time)
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

	// Step 4: OwnRUNPATH
	for _, rp := range ctx.OwnRUNPATH {
		expanded := expandOrigin(rp, ctx.ParentDir)
		candidate := filepath.Join(expanded, soname)
		searchedPaths = append(searchedPaths, candidate+" (RUNPATH)")
		if resolved, err := r.tryResolve(candidate); err == nil {
			return resolved, nil
		}
	}

	// Step 5: ld.so.cache
	if r.cache != nil {
		if cachedPath := r.cache.Lookup(soname); cachedPath != "" {
			searchedPaths = append(searchedPaths, cachedPath+" (ld.so.cache)")
			if resolved, err := r.tryResolve(cachedPath); err == nil {
				return resolved, nil
			}
		}
	}

	// Step 6: Default paths (architecture-dependent)
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
