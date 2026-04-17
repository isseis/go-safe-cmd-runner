//go:build test && darwin

package filevalidator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/machodylib"
	machodylibtesting "github.com/isseis/go-safe-cmd-runner/internal/machodylib/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRecord_Force_MachO_UpdatesDynLibDeps verifies that SaveRecord with force=true
// re-runs Mach-O dynlib analysis and updates DynLibDeps with the new hash.
// This covers Phase 3 completion criterion: record --force で Mach-O の DynLibDeps が更新されること.
func TestRecord_Force_MachO_UpdatesDynLibDeps(t *testing.T) {
	tempDir := safeTempDir(t)
	hashDir := filepath.Join(tempDir, "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o700))

	// Create a minimal dylib file on disk (content is the binary's hash source).
	libPath := filepath.Join(tempDir, "libfoo.dylib")
	require.NoError(t, os.WriteFile(libPath, []byte("initial dylib content"), 0o600))

	// Create a synthetic Mach-O binary referencing libfoo.dylib.
	binPath := filepath.Join(tempDir, "testbin")
	require.NoError(t, os.WriteFile(binPath,
		machodylibtesting.BuildMachOWithDeps(machodylibtesting.NativeCPU(), []string{libPath}, nil, nil), 0o700))

	v, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)
	v.SetMachODynLibAnalyzer(machodylib.NewMachODynLibAnalyzer(
		safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
	))

	// First record: DynLibDeps should capture libfoo.dylib with its initial hash.
	_, _, err = v.SaveRecord(binPath, false)
	require.NoError(t, err, "initial SaveRecord should succeed")

	rec1, err := v.LoadRecord(binPath)
	require.NoError(t, err)
	require.NotEmpty(t, rec1.DynLibDeps, "DynLibDeps must be populated after first record")
	hash1 := rec1.DynLibDeps[0].Hash

	// Modify the dylib to change its content hash.
	require.NoError(t, os.WriteFile(libPath, []byte("modified dylib content"), 0o600))

	// Force re-record: DynLibDeps must be updated with the new hash.
	_, _, err = v.SaveRecord(binPath, true)
	require.NoError(t, err, "force re-record should succeed")

	rec2, err := v.LoadRecord(binPath)
	require.NoError(t, err)
	require.NotEmpty(t, rec2.DynLibDeps, "DynLibDeps must still be present after force re-record")
	hash2 := rec2.DynLibDeps[0].Hash

	assert.NotEqual(t, hash1, hash2, "DynLibDeps hash must change after the dylib was modified")
}
