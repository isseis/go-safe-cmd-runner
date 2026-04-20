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

// libsystemKernelInstallName is the well-known install name for libsystem_kernel.dylib.
// Used both as a filesystem path candidate and as the cache key when extracting from
// the dyld shared cache (FR-3.1.5, FR-3.1.6).
const libsystemKernelInstallName = "/usr/lib/system/libsystem_kernel.dylib"

// libSystemBDylibInstallName is the install name of the libSystem umbrella library.
const libSystemBDylibInstallName = "/usr/lib/libSystem.B.dylib"

// libsystemKernelBaseName is the base name of libsystem_kernel.dylib.
const libsystemKernelBaseName = "libsystem_kernel.dylib"

// libSystemCandidates holds relevant DynLibDeps entries identified during FR-3.1.5 resolution.
type libSystemCandidates struct {
	// Umbrella is the libSystem.B.dylib entry if present.
	Umbrella *fileanalysis.LibEntry
	// Kernel is the direct libsystem_kernel.dylib entry if present.
	Kernel *fileanalysis.LibEntry
	// HasLibSystem is true when either Umbrella or Kernel is non-nil.
	HasLibSystem bool
}

// ResolveLibSystemKernel resolves the libsystem_kernel.dylib source from DynLibDeps.
//
// Resolution order:
//  1. Direct kernel entry from DynLibDeps (FR-3.1.5 priority 1)
//  2. Umbrella LC_REEXPORT_DYLIB traversal (FR-3.1.5 priority 2)
//  3. dyld shared cache extraction (FR-3.1.6) — tried before the well-known stub
//     path because on modern macOS /usr/lib/system/libsystem_kernel.dylib is a
//     stub that does not contain real syscall wrappers
//  4. Well-known filesystem path as last resort
//
// Returns nil, nil when no libSystem-family library is present in DynLibDeps,
// or when all resolution methods fail.
// Returns error only for unrecoverable conditions (permission errors, hash computation failures).
func ResolveLibSystemKernel(
	dynLibDeps []fileanalysis.LibEntry,
	fs safefileio.FileSystem,
) (*LibSystemKernelSource, error) {
	// Step 1: Collect libSystem umbrella and kernel candidates from DynLibDeps (FR-3.1.5).
	candidates := findLibSystemCandidates(dynLibDeps)

	if !candidates.HasLibSystem {
		// No libSystem-family library in DynLibDeps: return nil (fallback condition 2).
		return nil, nil
	}

	// Step 2: DynLibDeps-derived paths (priorities 1 & 2 only; well-known path excluded).
	candidatePath := resolveLibSystemKernelPathFromDeps(candidates, fs)
	if candidatePath != "" {
		return filesystemKernelSource(candidatePath, fs)
	}

	// Step 3: Try dyld shared cache extraction (FR-3.1.6) before the well-known stub path.
	// On modern macOS the real image lives in the shared cache; the well-known filesystem
	// path is a stub that does not contain real syscall wrappers.
	extracted, err := ExtractLibSystemKernelFromDyldCache()
	if err != nil {
		// Normally the extractor returns nil, nil on fallback cases.
		return nil, fmt.Errorf("dyld shared cache extraction failed unexpectedly: %w", err)
	}
	if extracted != nil {
		// Extraction succeeded: use the install name as the canonical cache path (FR-3.1.6).
		data := extracted.Data
		return &LibSystemKernelSource{
			Path:    libsystemKernelInstallName,
			Hash:    extracted.Hash,
			GetData: func() ([]byte, error) { return data, nil },
		}, nil
	}

	// Step 4: Well-known filesystem path as last resort (may be a stub on modern macOS).
	if _, err := os.Stat(libsystemKernelInstallName); err == nil {
		return filesystemKernelSource(libsystemKernelInstallName, fs)
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
			return os.ReadFile(path) //nolint:gosec // #nosec G304 -- path is a system library path from DynLibDeps or well-known locations
		},
	}, nil
}

// findLibSystemCandidates scans DynLibDeps for libSystem umbrella and kernel entries (FR-3.1.5).
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
		if filepath.Base(entry.SOName) == libsystemKernelBaseName {
			e := dynLibDeps[i]
			result.Kernel = &e
			result.HasLibSystem = true
		}
	}
	return result
}

// resolveLibSystemKernelPathFromDeps selects the best filesystem path for
// libsystem_kernel.dylib from DynLibDeps-derived sources only (priorities 1 and 2).
// Returns "" when neither source yields a usable path; the caller then tries the
// dyld shared cache before falling back to the well-known stub path.
func resolveLibSystemKernelPathFromDeps(candidates libSystemCandidates, fs safefileio.FileSystem) string {
	// Priority 1: direct kernel entry in DynLibDeps.
	if candidates.Kernel != nil && candidates.Kernel.Path != "" {
		if _, err := os.Stat(candidates.Kernel.Path); err == nil {
			return candidates.Kernel.Path
		}
	}

	// Priority 2: traverse LC_REEXPORT_DYLIB entries of the umbrella library.
	if candidates.Umbrella != nil && candidates.Umbrella.Path != "" {
		kernelPath, err := findKernelInUmbrellaReexports(candidates.Umbrella.Path, fs)
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

// findKernelInUmbrellaReexports opens the umbrella Mach-O binary and scans
// its LC_REEXPORT_DYLIB entries for libsystem_kernel.dylib.
// Returns the resolved filesystem path, or an empty string when not found.
func findKernelInUmbrellaReexports(umbrellaPath string, fs safefileio.FileSystem) (string, error) {
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
		if filepath.Base(dep.installName) == libsystemKernelBaseName {
			// The install name is typically an absolute path; check if it exists on disk.
			if _, statErr := os.Stat(dep.installName); statErr == nil {
				return dep.installName, nil
			}
		}
	}

	return "", nil
}
