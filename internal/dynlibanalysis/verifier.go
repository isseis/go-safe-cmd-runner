package dynlibanalysis

import (
	"debug/elf"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// DynLibVerifier performs two-stage verification of recorded library dependencies.
type DynLibVerifier struct {
	fs    safefileio.FileSystem
	cache *LDCache // parsed once at construction time; nil if ld.so.cache is unavailable
}

// NewDynLibVerifier creates a new verifier. It parses /etc/ld.so.cache once at
// construction time and reuses the result for every Verify() call.
func NewDynLibVerifier(fs safefileio.FileSystem) *DynLibVerifier {
	cache, err := ParseLDCache(defaultLDCachePath)
	if err != nil {
		slog.Warn("ld.so.cache unavailable, falling back to default paths",
			"error", err)
	}
	return &DynLibVerifier{
		fs:    fs,
		cache: cache,
	}
}

// Verify performs two-stage verification of dynamic library dependencies.
//
// Stage 1 (Hash verification): For each LibEntry, compute the hash of the file
// at entry.Path and compare with entry.Hash.
//
// Stage 2 (Path resolution verification): For each LibEntry, re-resolve
// (entry.ParentPath, entry.SOName) using the current environment (including
// LD_LIBRARY_PATH) and verify that the resolved path matches entry.Path.
//
// Returns nil if all checks pass.
// Returns a descriptive error if any check fails.
func (v *DynLibVerifier) Verify(binaryPath string, deps *fileanalysis.DynLibDepsData) error {
	if deps == nil || len(deps.Libs) == 0 {
		return nil
	}

	// Create resolver for re-resolution (needed for Stage 2).
	// v.cache was parsed once at NewDynLibVerifier() time and is reused here.
	machine, err := v.getELFMachine(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to determine ELF architecture: %w", err)
	}
	resolver := NewLibraryResolver(v.cache, machine)

	// Stage 1: Hash verification
	for _, entry := range deps.Libs {
		if entry.Path == "" {
			return &ErrEmptyLibraryPath{
				SOName:     entry.SOName,
				ParentPath: entry.ParentPath,
			}
		}

		actualHash, err := computeFileHash(entry.Path)
		if err != nil {
			return fmt.Errorf("failed to read library %s at %s: %w",
				entry.SOName, entry.Path, err)
		}

		if actualHash != entry.Hash {
			return &ErrLibraryHashMismatch{
				SOName:       entry.SOName,
				Path:         entry.Path,
				ExpectedHash: entry.Hash,
				ActualHash:   actualHash,
			}
		}
	}

	// Stage 2: Path resolution verification
	for _, entry := range deps.Libs {
		ctx, err := v.buildResolveContext(entry)
		if err != nil {
			return fmt.Errorf("failed to build resolve context for %s: %w",
				entry.SOName, err)
		}

		resolvedPath, err := resolver.Resolve(entry.SOName, ctx)
		if err != nil {
			return fmt.Errorf("failed to re-resolve library %s (parent: %s): %w",
				entry.SOName, entry.ParentPath, err)
		}

		if resolvedPath != entry.Path {
			return &ErrLibraryPathMismatch{
				SOName:       entry.SOName,
				ParentPath:   entry.ParentPath,
				RecordedPath: entry.Path,
				ResolvedPath: resolvedPath,
			}
		}
	}

	return nil
}

// buildResolveContext reconstructs the ResolveContext for a LibEntry at runtime.
// It re-reads the ParentPath ELF to obtain OwnRPATH/OwnRUNPATH, and uses
// the LibEntry's InheritedRPATH for the inherited context.
// IncludeLDLibraryPath is set to true (runner time).
func (v *DynLibVerifier) buildResolveContext(entry fileanalysis.LibEntry) (*ResolveContext, error) {
	// Read parent ELF to get its RPATH/RUNPATH.
	parentRPATH, parentRUNPATH, err := v.readELFPaths(entry.ParentPath)
	if err != nil {
		return nil, err
	}

	ctx := &ResolveContext{
		ParentPath:           entry.ParentPath,
		ParentDir:            filepath.Dir(entry.ParentPath),
		IncludeLDLibraryPath: true, // Runner time: include LD_LIBRARY_PATH
	}

	if len(parentRUNPATH) > 0 {
		ctx.OwnRUNPATH = parentRUNPATH
	} else {
		ctx.OwnRPATH = parentRPATH
	}

	// Reconstruct InheritedRPATH from recorded data.
	// Paths are already fully expanded at record time, so OriginDir is left empty
	// ($ORIGIN expansion is a no-op when Path contains no $ORIGIN).
	if len(entry.InheritedRPATH) > 0 {
		inherited := make([]ExpandedRPATHEntry, len(entry.InheritedRPATH))
		for i, rp := range entry.InheritedRPATH {
			inherited[i] = ExpandedRPATHEntry{
				Path:      rp,
				OriginDir: "", // Already expanded at record time
			}
		}
		ctx.InheritedRPATH = inherited
	}

	return ctx, nil
}

// readELFPaths reads DT_RPATH and DT_RUNPATH from an ELF file.
func (v *DynLibVerifier) readELFPaths(path string) (rpath, runpath []string, err error) {
	file, err := v.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = file.Close() }()

	elfFile, err := elf.NewFile(file)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = elfFile.Close() }()

	rpathRaw, _ := elfFile.DynString(elf.DT_RPATH)
	runpathRaw, _ := elfFile.DynString(elf.DT_RUNPATH)

	return splitPathList(rpathRaw), splitPathList(runpathRaw), nil
}

// getELFMachine returns the ELF machine type of the binary.
func (v *DynLibVerifier) getELFMachine(path string) (elf.Machine, error) {
	file, err := v.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return 0, err
	}
	defer func() { _ = file.Close() }()

	elfFile, err := elf.NewFile(file)
	if err != nil {
		return 0, err
	}
	defer func() { _ = elfFile.Close() }()

	return elfFile.Machine, nil
}

// computeFileHash is defined in analyzer.go and shared by DynLibVerifier.
