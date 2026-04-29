package machodylib

import (
	"debug/macho"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// LibSystemKernelBytes holds the in-memory bytes of libsystem_kernel.dylib
// extracted from the dyld shared cache, together with its SHA-256 hash.
type LibSystemKernelBytes struct {
	Data []byte // Mach-O bytes.
	Hash string // "sha256:<hex>" (SHA-256 of Data).
}

// LibSystemKernelSource represents the resolved source of libsystem_kernel.dylib.
type LibSystemKernelSource struct {
	// Path is used for the lib_path field and cache file naming.
	// Filesystem case: the resolved library path.
	// dyld shared cache case: the install name "/usr/lib/system/libsystem_kernel.dylib".
	Path string
	// Hash is the cache validity hash in "sha256:<hex>" format.
	Hash string
	// GetData returns Mach-O bytes and is called only on cache miss.
	GetData func() ([]byte, error)
}

// libSystemKernelInstallName is the well-known install name for libsystem_kernel.dylib.
// Used both as a filesystem path candidate and as the cache key when extracting from
// the dyld shared cache.
const libSystemKernelInstallName = "/usr/lib/system/libsystem_kernel.dylib"

// libSystemBDylibInstallName is the install name of the libSystem umbrella library.
const libSystemBDylibInstallName = "/usr/lib/libSystem.B.dylib"

// libSystemKernelBaseName is the base name of libsystem_kernel.dylib.
const libSystemKernelBaseName = "libsystem_kernel.dylib"

// libSystemCandidates holds relevant DynLibDeps entries identified when scanning for libSystem-family libraries.
type libSystemCandidates struct {
	// Umbrella is the libSystem.B.dylib entry if present.
	Umbrella *fileanalysis.LibEntry
	// Kernel is the direct libsystem_kernel.dylib entry if present.
	Kernel *fileanalysis.LibEntry
	// HasLibSystem is true when either Umbrella or Kernel is non-nil.
	HasLibSystem bool
}

// ResolveLibSystemKernel resolves the libsystem_kernel.dylib source.
//
// Resolution order:
//  1. Direct kernel entry from DynLibDeps
//  2. Umbrella LC_REEXPORT_DYLIB traversal
//  3. dyld shared cache extraction — tried before the well-known stub
//     path because on modern macOS /usr/lib/system/libsystem_kernel.dylib is a
//     stub that does not contain real syscall wrappers
//  4. Well-known filesystem path as last resort
//
// hasLibSystemFromLoadCmds must be set to true when the binary has an LC_LOAD_DYLIB
// entry naming a libSystem-family library (e.g. /usr/lib/libSystem.B.dylib or
// libsystem_kernel.dylib). On macOS 11+ these libraries live only in the dyld shared
// cache, so DynLibDeps will be empty even though the binary depends on libSystem.
// Passing hasLibSystemFromLoadCmds=true allows the resolver to skip Step 2 and
// proceed to Step 3 (dyld cache extraction) in that case.
//
// Returns nil, nil when no libSystem-family library is present in either DynLibDeps
// or the binary's load commands, or when all resolution methods fail.
// Returns error only for unrecoverable conditions (permission errors, hash computation failures).
func ResolveLibSystemKernel(
	dynLibDeps []fileanalysis.LibEntry,
	fs safefileio.FileSystem,
	hasLibSystemFromLoadCmds bool,
) (*LibSystemKernelSource, error) {
	// Step 1: Collect libSystem umbrella and kernel candidates from DynLibDeps.
	candidates := findLibSystemCandidates(dynLibDeps)

	if !candidates.HasLibSystem && !hasLibSystemFromLoadCmds {
		// No libSystem-family library in DynLibDeps or load commands: return nil.
		return nil, nil
	}

	// Step 2: DynLibDeps-derived paths (priorities 1 & 2 only; well-known path excluded).
	// Only attempted when DynLibDeps contains a libSystem entry; on macOS 11+ the
	// system libraries are in the dyld shared cache and absent from DynLibDeps, so
	// we fall through to Step 3 directly.
	if candidates.HasLibSystem {
		candidatePath := kernelPathFromDeps(candidates, fs)
		if candidatePath != "" {
			return filesystemKernelSource(candidatePath, fs)
		}
	}

	// Step 3: Try dyld shared cache extraction before the well-known stub path.
	// On modern macOS the real image lives in the shared cache; the well-known filesystem
	// path is a stub that does not contain real syscall wrappers.
	extracted, err := ExtractLibSystemKernel()
	if err != nil {
		// Normally the extractor returns nil, nil on fallback cases.
		return nil, fmt.Errorf("dyld shared cache extraction failed unexpectedly: %w", err)
	}
	if extracted != nil {
		// Use the well-known install name as the canonical cache path for dyld-extracted images.
		data := extracted.Data
		return &LibSystemKernelSource{
			Path:    libSystemKernelInstallName,
			Hash:    extracted.Hash,
			GetData: func() ([]byte, error) { return data, nil },
		}, nil
	}

	// Step 4: Well-known filesystem path as last resort (may be a stub on modern macOS).
	if _, err := os.Stat(libSystemKernelInstallName); err == nil {
		return filesystemKernelSource(libSystemKernelInstallName, fs)
	}

	slog.Info("libsystem_kernel.dylib not found via any method; applying fallback")
	return nil, nil
}

// filesystemKernelSource creates a LibSystemKernelSource backed by a filesystem path.
func filesystemKernelSource(path string, fs safefileio.FileSystem) (*LibSystemKernelSource, error) {
	hash, err := computeFileHash(fs, path)
	if err != nil {
		return nil, fmt.Errorf("failed to compute hash for %s: %w", path, err)
	}
	return &LibSystemKernelSource{
		Path: path,
		Hash: hash,
		GetData: func() ([]byte, error) {
			return safeReadFile(fs, path)
		},
	}, nil
}

// safeReadFile reads the entire file at path using fs.SafeOpenFile,
// applying the same symlink/TOCTOU protections as computeFileHash.
func safeReadFile(fs safefileio.FileSystem, path string) ([]byte, error) {
	f, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(f)
}

// findLibSystemCandidates scans DynLibDeps for libSystem umbrella and kernel entries.
//
// Return value interpretation:
//   - Kernel != nil: libsystem_kernel.dylib is directly present in DynLibDeps.
//   - Kernel == nil, Umbrella != nil: only libSystem.B.dylib is present.
//     resolveLibSystemKernelPath will traverse LC_REEXPORT_DYLIB entries from the umbrella.
//   - HasLibSystem == false: no libSystem-family dependency is present;
//     ResolveLibSystemKernel returns nil, nil.
func findLibSystemCandidates(dynLibDeps []fileanalysis.LibEntry) libSystemCandidates {
	var result libSystemCandidates
	for i, entry := range dynLibDeps {
		if entry.SOName == libSystemBDylibInstallName {
			e := dynLibDeps[i]
			result.Umbrella = &e
			result.HasLibSystem = true
		}
		// A direct kernel dependency takes precedence as the filesystem source.
		if filepath.Base(entry.SOName) == libSystemKernelBaseName {
			e := dynLibDeps[i]
			result.Kernel = &e
			result.HasLibSystem = true
		}
	}
	return result
}

// kernelPathFromDeps selects the best filesystem path for
// libsystem_kernel.dylib from DynLibDeps-derived sources only (priorities 1 and 2).
// The well-known stub path (libSystemKernelInstallName) is explicitly excluded from
// priority-2 results so that dyld shared cache extraction (priority 3) still runs on
// modern macOS where the stub exists on disk but contains no real syscall wrappers.
// Returns "" when neither source yields a usable non-stub path; the caller then tries
// the dyld shared cache before falling back to the well-known stub path.
func kernelPathFromDeps(candidates libSystemCandidates, fs safefileio.FileSystem) string {
	// Priority 1: direct kernel entry in DynLibDeps.
	if candidates.Kernel != nil && candidates.Kernel.Path != "" {
		if _, err := os.Stat(candidates.Kernel.Path); err == nil {
			return candidates.Kernel.Path
		}
	}

	// Priority 2: traverse LC_REEXPORT_DYLIB entries of the umbrella library.
	if candidates.Umbrella != nil && candidates.Umbrella.Path != "" {
		kernelPath, err := findKernelInUmbrella(candidates.Umbrella.Path, fs)
		if err != nil {
			// Non-fatal: log and fall through to dyld extraction.
			slog.Info("Failed to traverse LC_REEXPORT_DYLIB from umbrella; continuing",
				"umbrella", candidates.Umbrella.Path, "error", err)
		} else if kernelPath != "" {
			return kernelPath
		}
	}

	return ""
}

// findKernelInUmbrella opens the umbrella Mach-O binary and scans
// its LC_REEXPORT_DYLIB entries for libsystem_kernel.dylib.
// Returns the resolved filesystem path, or an empty string when not found.
func findKernelInUmbrella(umbrellaPath string, fs safefileio.FileSystem) (string, error) {
	file, err := fs.SafeOpenFile(umbrellaPath, os.O_RDONLY, 0)
	if err != nil {
		return "", fmt.Errorf("failed to open umbrella library: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Reset to start in case SafeOpenFile left the position elsewhere.
	if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
		return "", fmt.Errorf("failed to seek umbrella file: %w", seekErr)
	}

	mf, err := macho.NewFile(file)
	if err != nil {
		return "", fmt.Errorf("failed to parse umbrella as Mach-O: %w", err)
	}
	defer func() { _ = mf.Close() }()

	deps, _ := extractLoadCommands(mf)
	for _, dep := range deps {
		if filepath.Base(dep.installName) == libSystemKernelBaseName {
			// Skip the canonical well-known stub install-name path.
			// On modern macOS, /usr/lib/system/libsystem_kernel.dylib is a linker
			// stub that exists on disk but does not contain real syscall wrappers.
			// Returning it here would prevent the caller from proceeding to dyld
			// shared cache extraction (priority 3), which holds the real image.
			if dep.installName == libSystemKernelInstallName {
				slog.Info("Skipping well-known stub path from umbrella re-exports; will try dyld cache",
					"path", dep.installName)
				continue
			}
			// The install name is typically an absolute path; check if it exists on disk.
			if _, statErr := os.Stat(dep.installName); statErr == nil {
				return dep.installName, nil
			}
		}
	}

	return "", nil
}
