package dynlibanalysis

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

// closeFile safely closes a safefileio.File and logs any errors as warnings.
// Used for file resources that should be released even if close fails.
// This is particularly important for NFS and other remote filesystems where
// errors can be reported on close even for read-only operations.
func closeFile(f safefileio.File, path string) {
	if err := f.Close(); err != nil {
		slog.Warn("failed to close file", "path", path, "error", err)
	}
}

// closeELF safely closes an *elf.File and logs any errors as warnings.
// Used for ELF file resources that should be released even if close fails.
func closeELF(f *elf.File, path string) {
	if err := f.Close(); err != nil {
		slog.Warn("failed to close ELF file", "path", path, "error", err)
	}
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
	soname     string
	parentPath string   // path of the ELF that has this soname as DT_NEEDED
	runpath    []string // DT_RUNPATH of parentPath
	depth      int
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
	defer closeFile(file, binaryPath)

	// Try to parse as ELF
	elfFile, err := elf.NewFile(file)
	if err != nil {
		// Not an ELF file - this is normal for scripts etc.
		return nil, nil //nolint:nilerr
	}
	defer closeELF(elfFile, binaryPath)

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
	if len(rpath) > 0 {
		return nil, &ErrDTRPATHNotSupported{
			Path:  binaryPath,
			RPATH: strings.Join(rpath, ":"),
		}
	}

	runpath, _ := elfFile.DynString(elf.DT_RUNPATH)
	runpathEntries := splitPathList(runpath)

	// BFS queue and visited set:
	//   recorded: set of resolved paths already processed
	var queue []resolveItem
	recorded := make(map[string]struct{})
	var libs []fileanalysis.LibEntry

	// Seed queue with direct dependencies
	for _, soname := range needed {
		queue = append(queue, resolveItem{
			soname:     soname,
			parentPath: binaryPath,
			runpath:    runpathEntries,
			depth:      1,
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
		resolvedPath, err := resolver.Resolve(item.soname, item.parentPath, item.runpath)
		if err != nil {
			return nil, err
		}

		// Record and traverse each physical library file at most once.
		// Multiple parents may reference the same library; since verification
		// is hash-based, a single entry per path is sufficient.
		if _, ok := recorded[resolvedPath]; ok {
			continue
		}
		recorded[resolvedPath] = struct{}{}

		// Compute hash using safefileio
		hash, err := computeFileHash(a.fs, resolvedPath)
		if err != nil {
			return nil, fmt.Errorf("failed to compute hash for %s: %w", resolvedPath, err)
		}

		libs = append(libs, fileanalysis.LibEntry{
			SOName: item.soname,
			Path:   resolvedPath,
			Hash:   hash,
		})

		// Parse child dependencies
		childNeeded, childRUNPATH, err := a.parseELFDeps(resolvedPath)
		if err != nil {
			// ErrDTRPATHNotSupported must propagate: DT_RPATH in any dependency
			// is a hard error. Other parse failures (non-ELF data sections, etc.)
			// are non-fatal and we skip child traversal for that library.
			if _, ok := errors.AsType[*ErrDTRPATHNotSupported](err); ok {
				return nil, err
			}
			slog.Debug("Failed to parse child ELF dependencies",
				"path", resolvedPath, "error", err)
			continue
		}

		for _, childSoname := range childNeeded {
			queue = append(queue, resolveItem{
				soname:     childSoname,
				parentPath: resolvedPath,
				runpath:    childRUNPATH,
				depth:      item.depth + 1,
			})
		}
	}

	if len(libs) == 0 {
		return nil, nil
	}

	// Sort by (SOName, Path) for deterministic output across runs.
	// BFS traversal order depends on DT_NEEDED order in ELF files, which is
	// stable for a given binary but not guaranteed across re-links or tool
	// changes. Sorting ensures git diff noise is minimised and the JSON output
	// is reproducible.
	sort.Slice(libs, func(i, j int) bool {
		if libs[i].SOName != libs[j].SOName {
			return libs[i].SOName < libs[j].SOName
		}
		return libs[i].Path < libs[j].Path
	})

	return &fileanalysis.DynLibDepsData{
		Libs: libs,
	}, nil
}

// computeFileHash computes the SHA256 hash of the file at the given path
// using the provided FileSystem for symlink attack prevention.
// Shared by DynLibAnalyzer and DynLibVerifier to avoid duplication.
// Streams the file content through sha256.New() to avoid loading the entire
// file into memory (important for large libraries such as libLLVM.so ~50MB).
func computeFileHash(fs safefileio.FileSystem, path string) (string, error) {
	file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}
	defer closeFile(file, path)

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", fmt.Errorf("failed to hash %s: %w", path, err)
	}
	return fmt.Sprintf("%s:%s", hashPrefix, hex.EncodeToString(h.Sum(nil))), nil
}

// parseELFDeps opens the given path as ELF and extracts DT_NEEDED and DT_RUNPATH.
// Returns ErrDTRPATHNotSupported if the library contains DT_RPATH.
// Returns nil slices (not an error) if parsing fails for other reasons.
func (a *DynLibAnalyzer) parseELFDeps(path string) (needed, runpath []string, err error) {
	file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, err
	}
	defer closeFile(file, path)

	elfFile, err := elf.NewFile(file)
	if err != nil {
		return nil, nil, err
	}
	defer closeELF(elfFile, path)

	rpathRaw, _ := elfFile.DynString(elf.DT_RPATH)
	if len(rpathRaw) > 0 {
		return nil, nil, &ErrDTRPATHNotSupported{
			Path:  path,
			RPATH: strings.Join(rpathRaw, ":"),
		}
	}

	neededRaw, _ := elfFile.DynString(elf.DT_NEEDED)
	runpathRaw, _ := elfFile.DynString(elf.DT_RUNPATH)

	return neededRaw, splitPathList(runpathRaw), nil
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
