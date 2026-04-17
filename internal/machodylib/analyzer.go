package machodylib

import (
	"bytes"
	"crypto/sha256"
	"debug/macho"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

const (
	// MaxRecursionDepth is the maximum depth for recursive dependency resolution.
	// Normal macOS binaries have 3-5 levels of dependencies; exceeding this limit
	// indicates an abnormal configuration or missed circular dependency.
	MaxRecursionDepth = 20
)

// AnalysisWarning holds information about an unresolvable dependency
// (e.g., unknown @ token) that does not block recording but prevents
// hash verification for that particular library.
type AnalysisWarning struct {
	InstallName string // the unresolved install name
	Reason      string // human-readable reason (e.g., "unknown @ token: @loader_rpath")
}

// String returns a formatted warning string suitable for Record.AnalysisWarnings.
func (w AnalysisWarning) String() string {
	return fmt.Sprintf("dynlib warning: %s: %s", w.Reason, w.InstallName)
}

// MachODynLibAnalyzer resolves and records dynamic library dependencies
// for Mach-O binaries (LC_LOAD_DYLIB / LC_LOAD_WEAK_DYLIB).
type MachODynLibAnalyzer struct {
	fs safefileio.FileSystem
}

// NewMachODynLibAnalyzer creates a new analyzer.
func NewMachODynLibAnalyzer(fs safefileio.FileSystem) *MachODynLibAnalyzer {
	return &MachODynLibAnalyzer{fs: fs}
}

// bfsItem represents a pending library to resolve in the BFS queue.
type bfsItem struct {
	installName string   // LC_LOAD_DYLIB install name (stored as SOName in LibEntry)
	loaderPath  string   // path of the Mach-O that has this dependency
	rpaths      []string // LC_RPATH entries of the loader
	isWeak      bool     // true for LC_LOAD_WEAK_DYLIB
	depth       int      // current recursion depth
}

// Analyze resolves all direct and transitive LC_LOAD_DYLIB / LC_LOAD_WEAK_DYLIB
// dependencies of the given Mach-O binary, computes their hashes, and returns
// a []LibEntry snapshot along with any analysis warnings.
//
// Returns (nil, nil, nil) if the file is not Mach-O or has no LC_LOAD_DYLIB entries.
// Returns an error if any LC_LOAD_DYLIB (strong) dependency cannot be resolved.
// LC_LOAD_WEAK_DYLIB resolution failures are skipped with an info log.
// dyld shared cache libraries are skipped (not included in DynLibDeps).
// Unknown @ tokens generate warnings and skip the library.
func (a *MachODynLibAnalyzer) Analyze(binaryPath string) ([]fileanalysis.LibEntry, []AnalysisWarning, error) {
	// Open and parse Mach-O (or Fat binary)
	machoFile, closer, err := a.openMachO(binaryPath)
	if err != nil {
		if errors.Is(err, ErrNotMachO) {
			return nil, nil, nil // not a Mach-O file; skip silently
		}

		return nil, nil, err // I/O error, ErrNoMatchingSlice, etc.
	}

	defer func() { _ = closer.Close() }()
	defer func() { _ = machoFile.Close() }()

	// Extract load commands: LC_LOAD_DYLIB, LC_LOAD_WEAK_DYLIB, LC_RPATH
	deps, rpaths := extractLoadCommands(machoFile)
	if len(deps) == 0 {
		return nil, nil, nil // no dynamic dependencies
	}

	executableDir := filepath.Dir(binaryPath)
	resolver := NewLibraryResolver(executableDir)

	// BFS queue and visited set
	var queue []bfsItem

	// visited maps resolvedPath -> hash. Used to skip redundant hash computation
	// and recursive child parsing when multiple installNames (e.g. symlinks) resolve
	// to the same physical file, while still recording a LibEntry for every installName.
	visited := make(map[string]string)

	var libs []fileanalysis.LibEntry

	var warnings []AnalysisWarning

	// Seed queue with direct dependencies
	for _, dep := range deps {
		queue = append(queue, bfsItem{
			installName: dep.installName,
			loaderPath:  binaryPath,
			rpaths:      rpaths,
			isWeak:      dep.isWeak,
			depth:       1,
		})
	}

	// Process BFS queue
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		// Check depth limit
		if item.depth > MaxRecursionDepth {
			return nil, nil, &dynlib.ErrRecursionDepthExceeded{
				Depth:    item.depth,
				MaxDepth: MaxRecursionDepth,
				SOName:   item.installName,
			}
		}

		// Resolve install name to filesystem path
		resolvedPath, err := resolver.Resolve(item.installName, item.loaderPath, item.rpaths)
		if err != nil {
			// Check for unknown @ token
			var unknownErr *ErrUnknownAtToken
			if errors.As(err, &unknownErr) {
				warnings = append(warnings, AnalysisWarning{
					InstallName: item.installName,
					Reason:      unknownErr.Error(),
				})

				continue
			}

			// Resolution failed: skip dyld shared cache libraries only when
			// the install-name path is absent from disk (FR-3.1.5 two-condition test).
			// A resolution failure due to permission errors or other I/O issues on a
			// file that exists on disk must not silently bypass verification.
			if IsDyldSharedCacheLib(item.installName) {
				if _, statErr := os.Stat(item.installName); os.IsNotExist(statErr) {
					slog.Info("dynlib: skipping dyld shared cache library (delegating to code signing)",
						"install_name", item.installName)

					continue
				}
			}

			// Weak dependency: skip with info log
			if item.isWeak {
				slog.Info("dynlib: skipping unresolved weak dependency",
					"install_name", item.installName,
					"loader", item.loaderPath)

				continue
			}

			// Strong dependency resolution failure: abort recording
			return nil, nil, fmt.Errorf("failed to resolve LC_LOAD_DYLIB dependency: %w", err)
		}

		// If this physical file was already hashed, record a LibEntry for the
		// current installName (a different symlink may point here) but skip
		// redundant hash computation and recursive child parsing.
		if cachedHash, ok := visited[resolvedPath]; ok {
			libs = append(libs, fileanalysis.LibEntry{
				SOName: item.installName,
				Path:   resolvedPath,
				Hash:   cachedHash,
			})

			continue
		}

		// Compute hash using safefileio
		hash, err := computeFileHash(a.fs, resolvedPath)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to compute hash for %s: %w", resolvedPath, err)
		}

		visited[resolvedPath] = hash

		// Record the library entry
		libs = append(libs, fileanalysis.LibEntry{
			SOName: item.installName,
			Path:   resolvedPath,
			Hash:   hash,
		})

		// Parse child dependencies for BFS continuation
		childDeps, childRpaths, parseErr := a.parseMachODeps(resolvedPath)
		if parseErr != nil {
			slog.Debug("Failed to parse child Mach-O dependencies",
				"path", resolvedPath, "error", parseErr)

			continue
		}

		for _, childDep := range childDeps {
			queue = append(queue, bfsItem{
				installName: childDep.installName,
				loaderPath:  resolvedPath,
				rpaths:      childRpaths,
				isWeak:      childDep.isWeak,
				depth:       item.depth + 1,
			})
		}
	}

	if len(libs) == 0 {
		return nil, warnings, nil
	}

	sort.Slice(libs, func(i, j int) bool {
		if libs[i].SOName != libs[j].SOName {
			return libs[i].SOName < libs[j].SOName
		}
		return libs[i].Path < libs[j].Path
	})

	return libs, warnings, nil
}

// depEntry holds a dependency's install name and its weak/strong classification.
type depEntry struct {
	installName string
	isWeak      bool
}

// openMachO opens the file at binaryPath and returns a *macho.File along with
// an io.Closer that must be called to release all underlying file descriptors.
// The caller must call closer.Close() when done, regardless of whether
// machoFile.Close() is also called (macho.File.Close does not close the
// underlying os.File / SectionReader source).
//
// For Fat binaries, selects the slice matching runtime.GOARCH.
// Returns ErrNotMachO (via errors.Is) if the file is not a Mach-O or Fat binary.
// Returns ErrNoMatchingSlice if the Fat binary has no slice for the native arch.
// Returns other errors for I/O or permission failures.
func (a *MachODynLibAnalyzer) openMachO(binaryPath string) (*macho.File, io.Closer, error) {
	file, err := a.fs.SafeOpenFile(binaryPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, err
	}

	fatFile, fatErr := macho.NewFatFile(file)
	if fatErr == nil {
		// openFatSlice takes ownership of fatFile and file; both are always closed.
		return a.openFatSlice(binaryPath, fatFile, file)
	}

	// Single-arch path: openSingleArch takes ownership of file.
	return openSingleArch(file)
}

// openFatSlice selects the native-arch slice from a Fat binary and returns
// a *macho.File backed by a SectionReader over the already-open file.
// It takes ownership of fatFile and file; fatFile is always closed before
// returning. file is closed on error; on success it is returned as the closer.
func (a *MachODynLibAnalyzer) openFatSlice(binaryPath string, fatFile *macho.FatFile, file safefileio.File) (*macho.File, io.Closer, error) {
	defer func() { _ = fatFile.Close() }()

	cpuType := goarchToCPUType(runtime.GOARCH)
	if cpuType == 0 {
		_ = file.Close()
		return nil, nil, &ErrNoMatchingSlice{BinaryPath: binaryPath, GOARCH: runtime.GOARCH}
	}

	for _, arch := range fatFile.Arches {
		if arch.Cpu == cpuType {
			// Reuse the already-open file as the SectionReader source.
			// This avoids a second SafeOpenFile call and eliminates the
			// TOCTOU race that would exist between the two opens.
			machoFile, err := macho.NewFile(
				io.NewSectionReader(file, int64(arch.Offset), int64(arch.Size)))
			if err != nil {
				_ = file.Close()
				return nil, nil, fmt.Errorf("%w: %w", ErrNotMachO, err)
			}

			// Return file as the closer: macho.File.Close does not
			// close the SectionReader's underlying source.
			return machoFile, file, nil
		}
	}

	_ = file.Close()
	return nil, nil, &ErrNoMatchingSlice{BinaryPath: binaryPath, GOARCH: runtime.GOARCH}
}

// openSingleArch parses file as a single-architecture Mach-O.
// It takes ownership of file; file is closed on error but left open on success.
func openSingleArch(file safefileio.File) (_ *macho.File, _ io.Closer, retErr error) {
	defer func() {
		if retErr != nil {
			_ = file.Close()
		}
	}()

	// Reset file position (NewFatFile may have consumed bytes).
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}

	machoFile, err := macho.NewFile(file)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrNotMachO, err)
	}

	// file is the underlying source; return it as the closer.
	return machoFile, file, nil
}

// goarchToCPUType maps runtime.GOARCH to macho.Cpu type.
func goarchToCPUType(goarch string) macho.Cpu {
	switch goarch {
	case "arm64":
		return macho.CpuArm64
	case "amd64":
		return macho.CpuAmd64
	default:
		return 0
	}
}

// Load command constants not defined in Go's debug/macho package as of Go 1.23.
const (
	loadCmdWeakDylib     macho.LoadCmd = 0x80000018 // LC_LOAD_WEAK_DYLIB
	loadCmdReexportDylib macho.LoadCmd = 0x1F       // LC_REEXPORT_DYLIB
	loadCmdLazyLoadDylib macho.LoadCmd = 0x20       // LC_LAZY_LOAD_DYLIB
	loadCmdUpwardDylib   macho.LoadCmd = 0x23       // LC_LOAD_UPWARD_DYLIB
)

// extractLoadCommands extracts all dylib load commands and LC_RPATH
// entries from the Mach-O file's load commands.
//
// Implementation note: macho.File.ImportedLibraries() returns all dependency names
// but does not distinguish LC_LOAD_DYLIB from LC_LOAD_WEAK_DYLIB. This function
// walks macho.File.Loads directly to extract both the install name and the command
// type (FR-3.1.1).
func extractLoadCommands(f *macho.File) (deps []depEntry, rpaths []string) {
	const minLoadCmdSize = 8 // minimum bytes for a valid load command: cmd(4) + cmdsize(4)

	for _, load := range f.Loads {
		raw := load.Raw()
		if len(raw) < minLoadCmdSize {
			continue
		}

		cmd := f.ByteOrder.Uint32(raw[0:4])

		switch macho.LoadCmd(cmd) {
		case macho.LoadCmdDylib: // LC_LOAD_DYLIB = 0xC
			name := extractDylibName(raw, f.ByteOrder)
			if name != "" {
				deps = append(deps, depEntry{installName: name, isWeak: false})
			}

		case loadCmdWeakDylib: // LC_LOAD_WEAK_DYLIB = 0x80000018
			name := extractDylibName(raw, f.ByteOrder)
			if name != "" {
				deps = append(deps, depEntry{installName: name, isWeak: true})
			}

		case loadCmdReexportDylib, loadCmdLazyLoadDylib, loadCmdUpwardDylib:
			name := extractDylibName(raw, f.ByteOrder)
			if name != "" {
				deps = append(deps, depEntry{installName: name, isWeak: false})
			}

		case macho.LoadCmdRpath: // LC_RPATH = 0x8000001C
			path := extractRpathName(raw, f.ByteOrder)
			if path != "" {
				rpaths = append(rpaths, path)
			}
		}
	}

	return deps, rpaths
}

// extractDylibName extracts the library name from an LC_LOAD_DYLIB or
// LC_LOAD_WEAK_DYLIB load command's raw bytes.
// Layout: cmd(4) + cmdsize(4) + name_offset(4) + timestamp(4) + current_version(4)
// + compat_version(4) + name string (null-terminated).
func extractDylibName(raw []byte, bo binary.ByteOrder) string {
	// name_offset field starts at byte 8; minimum header size is 12 to read it.
	const minDylibCmdSize = 12

	if len(raw) < minDylibCmdSize {
		return ""
	}

	nameOffset := bo.Uint32(raw[8:12])
	if int(nameOffset) >= len(raw) {
		return ""
	}

	name := raw[nameOffset:]
	if idx := bytes.IndexByte(name, 0); idx >= 0 {
		name = name[:idx]
	}

	return string(name)
}

// extractRpathName extracts the path from an LC_RPATH load command's raw bytes.
// Layout: cmd(4) + cmdsize(4) + path_offset(4) + path string (null-terminated).
func extractRpathName(raw []byte, bo binary.ByteOrder) string {
	// path_offset field starts at byte 8; minimum header size is 12 to read it.
	const minRpathCmdSize = 12

	if len(raw) < minRpathCmdSize {
		return ""
	}

	pathOffset := bo.Uint32(raw[8:12])
	if int(pathOffset) >= len(raw) {
		return ""
	}

	path := raw[pathOffset:]
	if idx := bytes.IndexByte(path, 0); idx >= 0 {
		path = path[:idx]
	}

	return string(path)
}

// parseMachODeps opens a resolved .dylib and extracts its LC_LOAD_DYLIB /
// LC_LOAD_WEAK_DYLIB / LC_RPATH entries for BFS continuation.
// Returns nil slices (not an error) if parsing fails.
func (a *MachODynLibAnalyzer) parseMachODeps(path string) ([]depEntry, []string, error) {
	file, err := a.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, nil, err
	}

	defer func() { _ = file.Close() }()

	machoFile, err := macho.NewFile(file)
	if err != nil {
		return nil, nil, err
	}

	defer func() { _ = machoFile.Close() }()

	deps, rpaths := extractLoadCommands(machoFile)

	return deps, rpaths, nil
}

// computeFileHash computes the SHA256 hash of the file at the given path
// using safefileio for symlink attack prevention.
// Uses streaming (SafeOpenFile + io.Copy) to avoid loading large libraries
// into memory.
//
// Note: This is functionally identical to elfdynlib.computeFileHash but
// is defined separately to avoid a circular import between machodylib and
// elfdynlib. Both implementations use the same algorithm (SHA256) and
// format ("sha256:<hex>").
func computeFileHash(fs safefileio.FileSystem, path string) (string, error) {
	file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return "", err
	}

	defer func() { _ = file.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", fmt.Errorf("failed to hash %s: %w", path, err)
	}

	return fmt.Sprintf("sha256:%s", hex.EncodeToString(h.Sum(nil))), nil
}

// HasDynamicLibDeps checks if the file at the given path is a Mach-O binary
// that has at least one LC_LOAD_DYLIB or LC_LOAD_WEAK_DYLIB entry pointing to
// a non-dyld-shared-cache library.
//
// Used by runner to detect Mach-O binaries that should have DynLibDeps recorded
// but don't (e.g., recorded before this feature was added).
//
// Returns (false, nil) for non-Mach-O files, Mach-O files with no dependencies,
// or Mach-O files whose dependencies are all dyld shared cache libraries.
func HasDynamicLibDeps(path string, fs safefileio.FileSystem) (bool, error) {
	file, err := fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return false, fmt.Errorf("failed to open binary for Mach-O inspection: %w", err)
	}

	defer func() { _ = file.Close() }()

	// Try as Fat binary first
	fatFile, fatErr := macho.NewFatFile(file)
	if fatErr == nil {
		cpuType := goarchToCPUType(runtime.GOARCH)
		if cpuType == 0 {
			_ = fatFile.Close()

			return false, nil
		}

		for _, arch := range fatFile.Arches {
			if arch.Cpu == cpuType {
				_ = fatFile.Close()

				// Reuse the already-open file as the SectionReader source to
				// avoid a second SafeOpenFile call and eliminate the TOCTOU race.
				machoFile, err := macho.NewFile(
					io.NewSectionReader(file, int64(arch.Offset), int64(arch.Size)))
				if err != nil {
					return false, nil
				}

				defer func() { _ = machoFile.Close() }()

				deps, _ := extractLoadCommands(machoFile)
				for _, dep := range deps {
					// Treat as dyld shared cache only when the install name is
					// system-prefixed AND the file is absent from disk.
					if IsDyldSharedCacheLib(dep.installName) {
						if _, statErr := os.Stat(dep.installName); os.IsNotExist(statErr) {
							continue
						}
					}

					return true, nil
				}

				return false, nil
			}
		}

		_ = fatFile.Close()

		return false, nil // no matching architecture
	}

	// Try as single-architecture Mach-O
	if seeker, ok := file.(io.Seeker); ok {
		if _, err := seeker.Seek(0, io.SeekStart); err != nil {
			return false, nil
		}
	}

	machoFile, err := macho.NewFile(file)
	if err != nil {
		// Not a Mach-O binary
		return false, nil
	}

	defer func() { _ = machoFile.Close() }()

	deps, _ := extractLoadCommands(machoFile)
	for _, dep := range deps {
		// Treat as dyld shared cache only when the install name is
		// system-prefixed AND the file is absent from disk.
		if IsDyldSharedCacheLib(dep.installName) {
			if _, statErr := os.Stat(dep.installName); os.IsNotExist(statErr) {
				continue
			}
		}

		return true, nil
	}

	return false, nil
}
