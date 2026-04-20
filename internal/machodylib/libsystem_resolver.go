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
// Returns nil, nil when:
//   - no libSystem-family library is present in DynLibDeps (non-libSystem binary)
//   - dyld shared cache extraction also failed (fallback path)
//
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

	// Step 2: Choose a filesystem candidate path in priority order:
	// direct kernel entry, umbrella re-export, well-known path.
	candidatePath := resolveLibSystemKernelPath(candidates, fs)

	// Step 3: Use the filesystem path if one was resolved.
	if candidatePath != "" {
		hash, err := computeFileHash(fs, candidatePath)
		if err != nil {
			return nil, fmt.Errorf("failed to compute hash for %s: %w", candidatePath, err)
		}
		path := candidatePath
		return &LibSystemKernelSource{
			Path: path,
			Hash: hash,
			GetData: func() ([]byte, error) {
				return os.ReadFile(path) //nolint:gosec // G304: path is a system library path from DynLibDeps or well-known
			},
		}, nil
	}

	// Step 4: Try dyld shared cache extraction (FR-3.1.6).
	extracted, err := ExtractLibSystemKernelFromDyldCache()
	if err != nil {
		// Normally the extractor returns nil, nil on fallback cases.
		return nil, fmt.Errorf("dyld shared cache extraction failed unexpectedly: %w", err)
	}
	if extracted == nil {
		slog.Info("dyld shared cache extraction for libsystem_kernel.dylib also failed; applying fallback")
		return nil, nil
	}

	// Extraction succeeded: use the install name as the canonical cache path (FR-3.1.6).
	data := extracted.Data
	return &LibSystemKernelSource{
		Path:    libsystemKernelInstallName,
		Hash:    extracted.Hash,
		GetData: func() ([]byte, error) { return data, nil },
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

// resolveLibSystemKernelPath selects the best filesystem path for libsystem_kernel.dylib
// according to FR-3.1.5 priority order:
//  1. direct kernel Path from DynLibDeps (already resolved and on disk)
//  2. umbrella file on disk → LC_REEXPORT_DYLIB traversal (task 0096 resolver reuse)
//  3. well-known path /usr/lib/system/libsystem_kernel.dylib
//  4. empty string (caller proceeds to dyld shared cache extraction)
func resolveLibSystemKernelPath(candidates libSystemCandidates, fs safefileio.FileSystem) string {
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
			// Non-fatal: log and fall through to the well-known path.
			slog.Info("Failed to traverse LC_REEXPORT_DYLIB from umbrella; continuing",
				"umbrella", candidates.Umbrella.Path, "error", err)
		} else if kernelPath != "" {
			return kernelPath
		}
	}

	// Priority 3: well-known filesystem path.
	if _, err := os.Stat(libsystemKernelInstallName); err == nil {
		return libsystemKernelInstallName
	}

	// No filesystem path found; caller will try the dyld shared cache.
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
