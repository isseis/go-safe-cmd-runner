//go:build test && darwin

package filevalidator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/machodylib/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecord_Force_MachO_UpdatesDynLibDeps verifies that SaveRecord with force=true
// re-runs Mach-O dynlib analysis and updates DynLibDeps with the new hash.
// Verifies force re-record updates Mach-O DynLibDeps.
func TestRecord_Force_MachO_UpdatesDynLibDeps(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// Create a minimal, real Mach-O dylib on disk (content is the binary's hash
	// source). It must be a valid Mach-O, not arbitrary bytes: SaveRecord's BFS
	// dependency walk parses every strong LC_LOAD_DYLIB target and fails closed
	// on a parse error, mirroring the ELF analyzer's warn-then-abort behavior.
	libPath := filepath.Join(tempDir, "libfoo.dylib")
	require.NoError(t, os.WriteFile(libPath,
		machodylibtestutil.BuildMachOWithDeps(machodylibtestutil.NativeCPU(), nil, nil, nil), 0o600))

	// Create a synthetic Mach-O binary referencing libfoo.dylib.
	binPath := filepath.Join(tempDir, "testbin")
	require.NoError(t, os.WriteFile(binPath,
		machodylibtestutil.BuildMachOWithDeps(machodylibtestutil.NativeCPU(), []string{libPath}, nil, nil), 0o700))

	v, err := New(&SHA256{}, hashDir, ValidatorConfig{
		MachODynLibAnalyzer: machodylib.NewMachODynLibAnalyzer(
			safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
		),
	})
	require.NoError(t, err)

	// First record: DynLibDeps should capture libfoo.dylib with its initial hash.
	_, _, err = v.SaveRecord(binPath, false)
	require.NoError(t, err, "initial SaveRecord should succeed")

	rec1, err := v.LoadRecord(binPath)
	require.NoError(t, err)
	require.NotEmpty(t, rec1.DynLibDeps, "DynLibDeps must be populated after first record")
	hash1 := rec1.DynLibDeps[0].Hash

	// Modify the dylib to change its content hash, while staying a valid,
	// dependency-free Mach-O (an added rpath entry changes the bytes without
	// introducing another LC_LOAD_DYLIB target for the walk to follow).
	require.NoError(t, os.WriteFile(libPath,
		machodylibtestutil.BuildMachOWithDeps(machodylibtestutil.NativeCPU(), nil, nil, []string{"/unused/rpath"}), 0o600))

	// Force re-record: DynLibDeps must be updated with the new hash.
	_, _, err = v.SaveRecord(binPath, true)
	require.NoError(t, err, "force re-record should succeed")

	rec2, err := v.LoadRecord(binPath)
	require.NoError(t, err)
	require.NotEmpty(t, rec2.DynLibDeps, "DynLibDeps must still be present after force re-record")
	hash2 := rec2.DynLibDeps[0].Hash

	assert.NotEqual(t, hash1, hash2, "DynLibDeps hash must change after the dylib was modified")
}
