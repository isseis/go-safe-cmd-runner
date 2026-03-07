package dynlibanalysis

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

const (
	// defaultLDCachePath is the standard location of the dynamic linker cache.
	defaultLDCachePath = "/etc/ld.so.cache"

	// MaxRecursionDepth is the maximum depth for recursive dependency resolution.
	// Normal Linux binaries have 3-5 levels of dependencies; exceeding this limit
	// indicates an abnormal configuration or missed circular dependency.
	MaxRecursionDepth = 20

	// hashPrefix is the algorithm prefix prepended to hex hash digests.
	hashPrefix = "sha256"
)

// knownVDSOs contains virtual shared objects that exist only in kernel memory.
// These should be skipped during dependency resolution as they have no filesystem path.
var knownVDSOs = map[string]struct{}{
	"linux-vdso.so.1":   {},
	"linux-gate.so.1":   {},
	"linux-vdso64.so.1": {},
}

// DynLibAnalyzer resolves and records dynamic library dependencies for ELF binaries.
type DynLibAnalyzer struct {
	fs    safefileio.FileSystem
	cache *LDCache // parsed once at construction time; nil if ld.so.cache is unavailable
}

// NewDynLibAnalyzer creates a new analyzer. It parses /etc/ld.so.cache once at
// construction time and reuses the result for every Analyze() call.
// If the cache is unavailable, resolution falls back to default paths.
// A LibraryResolver is created per Analyze() call (not per DynLibAnalyzer) because
// the resolver holds architecture-specific search paths that vary by binary.
func NewDynLibAnalyzer(fs safefileio.FileSystem) *DynLibAnalyzer {
	cache, err := ParseLDCache(defaultLDCachePath)
	if err != nil {
		slog.Warn("ld.so.cache unavailable, falling back to default paths",
			"error", err)
	}
	return &DynLibAnalyzer{
		fs:    fs,
		cache: cache,
	}
}

// resolveItem represents a pending library to resolve in the BFS queue.
type resolveItem struct {
	soname string
	ctx    *ResolveContext
	depth  int
}

// visitedPath tracks physical files already enqueued for child dependency
// traversal. Keyed by resolvedPath only, because the ELF content (and therefore
// DT_NEEDED entries) is the same regardless of which RPATH context led to the
// file. This set prevents infinite loops on circular dependency graphs.
type visitedPath = string

// entryKey identifies a unique LibEntry to record.
// The same physical library may appear under multiple parents (e.g. both
// lib_a.so and lib_b.so import libssl.so.3), each requiring a separate
// LibEntry so that the verifier can re-resolve every (ParentPath, SOName) pair.
type entryKey struct {
	resolvedPath string
	parentPath   string
}

// Analyze resolves all direct and transitive DT_NEEDED dependencies of the given
// ELF binary, computes their hashes, and returns a DynLibDepsData snapshot.
//
// Returns nil (not an error) if the file is not ELF or has no DT_NEEDED entries.
// Returns an error if any library cannot be resolved (FR-3.1.7).
func (a *DynLibAnalyzer) Analyze(binaryPath string) (*fileanalysis.DynLibDepsData, error) {
	// Open file safely
	file, err := a.fs.SafeOpenFile(binaryPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Try to parse as ELF
	elfFile, err := elf.NewFile(file)
	if err != nil {
		// Not an ELF file - this is normal for scripts etc.
		return nil, nil //nolint:nilerr
	}
	defer func() { _ = elfFile.Close() }()

	// Get DT_NEEDED entries
	needed, err := elfFile.DynString(elf.DT_NEEDED)
	if err != nil || len(needed) == 0 {
		// No DT_NEEDED entries (static binary or no dependencies)
		return nil, nil //nolint:nilerr
	}

	// Create resolver for this binary's architecture.
	// a.cache was parsed once at NewDynLibAnalyzer() time and is reused here.
	resolver := NewLibraryResolver(a.cache, elfFile.Machine)

	// Get RPATH and RUNPATH
	rpath, _ := elfFile.DynString(elf.DT_RPATH)
	runpath, _ := elfFile.DynString(elf.DT_RUNPATH)

	rpathEntries := splitPathList(rpath)
	runpathEntries := splitPathList(runpath)

	// Create root resolution context (LD_LIBRARY_PATH is NOT used at record time)
	rootCtx := NewRootContext(binaryPath, rpathEntries, runpathEntries, false)

	// BFS queue and visited sets:
	//   traversed: resolvedPath → struct{}   prevents re-traversing a file's DT_NEEDED entries
	//   recorded:  entryKey → struct{}       prevents duplicate LibEntry for the same (path, parent)
	var queue []resolveItem
	traversed := make(map[visitedPath]struct{})
	recorded := make(map[entryKey]struct{})
	var libs []fileanalysis.LibEntry

	// Seed queue with direct dependencies
	for _, soname := range needed {
		queue = append(queue, resolveItem{
			soname: soname,
			ctx:    rootCtx,
			depth:  1,
		})
	}

	// Process queue
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		// Skip known vDSOs
		if _, isVDSO := knownVDSOs[item.soname]; isVDSO {
			continue
		}

		// Check depth limit
		if item.depth > MaxRecursionDepth {
			return nil, &ErrRecursionDepthExceeded{
				Depth:    item.depth,
				MaxDepth: MaxRecursionDepth,
				SOName:   item.soname,
			}
		}

		// Resolve library path
		resolvedPath, err := resolver.Resolve(item.soname, item.ctx)
		if err != nil {
			return nil, err
		}

		// Record a LibEntry for each unique (resolvedPath, parentPath) pair.
		// The same physical library may be imported by multiple parents, and
		// the verifier re-resolves every recorded (ParentPath, SOName) pair
		// independently, so each such pair must appear as a separate entry.
		eKey := entryKey{resolvedPath: resolvedPath, parentPath: item.ctx.ParentPath}
		if _, ok := recorded[eKey]; !ok {
			recorded[eKey] = struct{}{}

			// Compute hash using safefileio
			hash, err := computeFileHash(a.fs, resolvedPath)
			if err != nil {
				return nil, fmt.Errorf("failed to compute hash for %s: %w", resolvedPath, err)
			}

			// Build InheritedRPATH for serialization
			var inheritedRPATHStrings []string
			if len(item.ctx.InheritedRPATH) > 0 {
				inheritedRPATHStrings = make([]string, len(item.ctx.InheritedRPATH))
				for i, entry := range item.ctx.InheritedRPATH {
					inheritedRPATHStrings[i] = expandOrigin(entry.Path, entry.OriginDir)
				}
			}

			libs = append(libs, fileanalysis.LibEntry{
				SOName:         item.soname,
				ParentPath:     item.ctx.ParentPath,
				Path:           resolvedPath,
				Hash:           hash,
				InheritedRPATH: inheritedRPATHStrings,
			})
		}

		// Traverse child dependencies only once per physical file.
		if _, ok := traversed[resolvedPath]; ok {
			continue
		}
		traversed[resolvedPath] = struct{}{}

		// Parse child dependencies
		childNeeded, childRPATH, childRUNPATH, err := a.parseELFDeps(resolvedPath)
		if err != nil {
			slog.Debug("Failed to parse child ELF dependencies",
				"path", resolvedPath, "error", err)
			continue
		}

		// Create child context and enqueue
		childCtx := item.ctx.NewChildContext(resolvedPath, childRPATH, childRUNPATH)
		for _, childSoname := range childNeeded {
			queue = append(queue, resolveItem{
				soname: childSoname,
				ctx:    childCtx,
				depth:  item.depth + 1,
			})
		}
	}

	if len(libs) == 0 {
		return nil, nil
	}

	return &fileanalysis.DynLibDepsData{
		RecordedAt: time.Now(),
		Libs:       libs,
	}, nil
}

// computeFileHash computes the SHA256 hash of the file at the given path
// using the provided FileSystem for symlink attack prevention.
// Shared by DynLibAnalyzer and DynLibVerifier to avoid duplication.
//
// Design note: SafeReadFile loads the entire file into memory before hashing.
// For typical shared libraries (libc ~2MB, libssl ~1MB), this is acceptable.
// With 10-30 dependencies per binary, peak memory usage remains within
// practical limits. However, very large libraries (e.g., libLLVM.so ~50MB)
// could cause memory pressure if many such libraries are hashed concurrently.
// Future improvement: replace with fs.SafeOpenFile + io.Copy(sha256.New(), file)
// for streaming hash computation without loading the full file into memory.
func computeFileHash(fs safefileio.FileSystem, path string) (string, error) {
	content, err := safefileio.SafeReadFileWithFS(path, fs)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := h.Write(content); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:%s", hashPrefix, hex.EncodeToString(h.Sum(nil))), nil
}

// parseELFDeps opens the given path as ELF and extracts DT_NEEDED, DT_RPATH,
// and DT_RUNPATH. Returns nil slices (not an error) if parsing fails.
func (a *DynLibAnalyzer) parseELFDeps(path string) (needed, rpath, runpath []string, err error) {
	file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = file.Close() }()

	elfFile, err := elf.NewFile(file)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = elfFile.Close() }()

	neededRaw, _ := elfFile.DynString(elf.DT_NEEDED)
	rpathRaw, _ := elfFile.DynString(elf.DT_RPATH)
	runpathRaw, _ := elfFile.DynString(elf.DT_RUNPATH)

	return neededRaw, splitPathList(rpathRaw), splitPathList(runpathRaw), nil
}

// splitPathList splits colon-separated path lists (as returned by DynString)
// into individual paths. Returns nil for empty input.
func splitPathList(pathLists []string) []string {
	if len(pathLists) == 0 {
		return nil
	}
	var result []string
	for _, pl := range pathLists {
		for _, p := range filepath.SplitList(pl) {
			if p != "" {
				result = append(result, p)
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
