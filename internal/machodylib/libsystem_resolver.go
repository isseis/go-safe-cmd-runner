package machodylib

import (
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

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

// ResolveLibSystemKernel resolves the libsystem_kernel.dylib source from DynLibDeps.
//
// Returns nil, nil when:
//   - no libSystem-family library is present in DynLibDeps (non-libSystem binary)
//   - dyld shared cache extraction also failed (fallback path)
//
// Returns error only for unrecoverable conditions (permission errors, hash computation failures).
//
// NOTE: Full implementation is provided in libsystem_resolver.go (Step 2).
// This stub always returns nil, nil for now.
func ResolveLibSystemKernel(
	_ []fileanalysis.LibEntry,
	_ safefileio.FileSystem,
) (*LibSystemKernelSource, error) {
	// TODO(task-0100-step2): implement full resolution logic.
	// Temporary stub: always fall back to symbol-name matching.
	return nil, nil
}
