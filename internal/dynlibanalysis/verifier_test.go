//go:build test

package dynlibanalysis

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestVerifier creates a DynLibVerifier for testing using the real filesystem.
// ld.so.cache parsing failures are tolerated (verifier continues with cache=nil).
func newTestVerifier() *DynLibVerifier {
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	return NewDynLibVerifier(fs)
}

// writeFile creates a file at path with the given content.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

// TestVerify_NilDeps ensures Verify returns nil immediately for nil DynLibDeps.
func TestVerify_NilDeps(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()
	binaryPath := buildTestELFWithDeps(t, tmpDir, "binary", nil, "")

	err := v.Verify(binaryPath, nil)
	assert.NoError(t, err)
}

// TestVerify_EmptyLibs ensures Verify returns nil when DynLibDeps.Libs is empty.
func TestVerify_EmptyLibs(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()
	binaryPath := buildTestELFWithDeps(t, tmpDir, "binary", nil, "")

	deps := &fileanalysis.DynLibDepsData{Libs: []fileanalysis.LibEntry{}}
	err := v.Verify(binaryPath, deps)
	assert.NoError(t, err)
}

// TestVerify_EmptyPath ensures ErrEmptyLibraryPath is returned when an entry
// has an empty Path.
func TestVerify_EmptyPath(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()
	binaryPath := buildTestELFWithDeps(t, tmpDir, "binary", nil, "")
	parentPath := buildTestELFWithDeps(t, tmpDir, "parent", nil, "")

	deps := &fileanalysis.DynLibDepsData{
		Libs: []fileanalysis.LibEntry{
			{
				SOName:     "libtest.so",
				ParentPath: parentPath,
				Path:       "", // Empty path should trigger ErrEmptyLibraryPath
				Hash:       "aabbccdd",
			},
		},
	}

	err := v.Verify(binaryPath, deps)
	require.Error(t, err)
	var errEmpty *ErrEmptyLibraryPath
	assert.ErrorAs(t, err, &errEmpty, "expected ErrEmptyLibraryPath")
	assert.Equal(t, "libtest.so", errEmpty.SOName)
	assert.Equal(t, parentPath, errEmpty.ParentPath)
}

// TestVerify_Stage1_HashMatch ensures Verify returns nil when all library
// hashes match the recorded values (Stage 1 passes).
func TestVerify_Stage1_HashMatch(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()

	// Create a parent ELF and the command binary (no RPATH).
	binaryPath := buildTestELFWithDeps(t, tmpDir, "binary", nil, "")
	parentPath := buildTestELFWithDeps(t, tmpDir, "parent", nil, "")

	// Create a fake library file.
	libPath := filepath.Join(tmpDir, "libfoo.so")
	writeFile(t, libPath, "fake library content for hash test")

	// Compute its real hash.
	actualHash, err := computeFileHash(libPath)
	require.NoError(t, err)

	deps := &fileanalysis.DynLibDepsData{
		Libs: []fileanalysis.LibEntry{
			{
				SOName:     "libfoo.so",
				ParentPath: parentPath,
				Path:       libPath,
				Hash:       actualHash, // Correct hash → Stage 1 passes
			},
		},
	}

	// Stage 2 may fail to resolve libfoo.so from this temp dir (not in standard
	// paths), but Stage 1 must pass. Assert the error is NOT ErrLibraryHashMismatch.
	verifyErr := v.Verify(binaryPath, deps)
	var hashErr *ErrLibraryHashMismatch
	assert.False(t, errors.As(verifyErr, &hashErr),
		"Stage 1 hash check should pass (no ErrLibraryHashMismatch), got: %v", verifyErr)
}

// TestVerify_Stage1_HashMismatch ensures ErrLibraryHashMismatch is returned
// when a recorded hash does not match the current file content.
func TestVerify_Stage1_HashMismatch(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()

	binaryPath := buildTestELFWithDeps(t, tmpDir, "binary", nil, "")
	parentPath := buildTestELFWithDeps(t, tmpDir, "parent", nil, "")

	// Create a library file.
	libPath := filepath.Join(tmpDir, "libbar.so")
	writeFile(t, libPath, "original content")

	// Record a deliberately wrong hash.
	const wrongHash = "0000000000000000000000000000000000000000000000000000000000000000"

	deps := &fileanalysis.DynLibDepsData{
		Libs: []fileanalysis.LibEntry{
			{
				SOName:     "libbar.so",
				ParentPath: parentPath,
				Path:       libPath,
				Hash:       wrongHash,
			},
		},
	}

	err := v.Verify(binaryPath, deps)
	require.Error(t, err)

	var hashErr *ErrLibraryHashMismatch
	require.ErrorAs(t, err, &hashErr, "expected ErrLibraryHashMismatch")
	assert.Equal(t, "libbar.so", hashErr.SOName)
	assert.Equal(t, libPath, hashErr.Path)
	assert.Equal(t, wrongHash, hashErr.ExpectedHash)
	assert.NotEmpty(t, hashErr.ActualHash)
	assert.NotEqual(t, wrongHash, hashErr.ActualHash)
}

// TestVerify_Stage2_PathMatch ensures Verify returns nil when Stage 2 resolves
// to the same path as the recorded path (normal case without LD_LIBRARY_PATH hijack).
func TestVerify_Stage2_PathMatch(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()

	// Build a parent ELF that has RPATH pointing to tmpDir so the resolver
	// can locate the library from that directory during Stage 2.
	parentPath := buildTestELFWithDeps(t, tmpDir, "parent", nil, tmpDir)
	binaryPath := buildTestELFWithDeps(t, tmpDir, "binary", nil, "")

	const libName = "libstage2.so"
	libPath := filepath.Join(tmpDir, libName)
	writeFile(t, libPath, "library content for stage2 path match test")

	actualHash, err := computeFileHash(libPath)
	require.NoError(t, err)

	deps := &fileanalysis.DynLibDepsData{
		Libs: []fileanalysis.LibEntry{
			{
				SOName:     libName,
				ParentPath: parentPath,
				Path:       libPath,
				Hash:       actualHash,
			},
		},
	}

	// No LD_LIBRARY_PATH manipulation: Stage 2 resolves via parent's RPATH.
	// Stage 1 passes (hash matches), Stage 2 passes (path matches).
	err = v.Verify(binaryPath, deps)
	assert.NoError(t, err, "Stage 2 should pass when resolved path matches recorded path")
}

// TestVerify_Stage2_PathMismatch_LDLibraryPath ensures ErrLibraryPathMismatch
// is returned when LD_LIBRARY_PATH causes Stage 2 to resolve to a different path
// than the one recorded in Stage 1.
func TestVerify_Stage2_PathMismatch_LDLibraryPath(t *testing.T) {
	v := newTestVerifier()
	tmpDir := t.TempDir()

	// dirA is the recorded location; dirB is a different location injected via env.
	dirA := filepath.Join(tmpDir, "dirA")
	dirB := filepath.Join(tmpDir, "dirB")
	require.NoError(t, os.MkdirAll(dirA, 0o700))
	require.NoError(t, os.MkdirAll(dirB, 0o700))

	const libName = "libhijack.so"
	libA := filepath.Join(dirA, libName)
	libB := filepath.Join(dirB, libName)
	writeFile(t, libA, "original library in dirA")
	writeFile(t, libB, "hijacked library in dirB")

	// Compute the actual hash of libA so Stage 1 passes.
	hashA, err := computeFileHash(libA)
	require.NoError(t, err)

	// Build ELFs: binary (command), parent (no RPATH → resolver uses LD_LIBRARY_PATH).
	binaryPath := buildTestELFWithDeps(t, tmpDir, "binary", nil, "")
	parentPath := buildTestELFWithDeps(t, tmpDir, "parent", nil, "")

	deps := &fileanalysis.DynLibDepsData{
		Libs: []fileanalysis.LibEntry{
			{
				SOName:     libName,
				ParentPath: parentPath,
				Path:       libA,  // Recorded path
				Hash:       hashA, // Hash of libA (Stage 1 passes)
			},
		},
	}

	// LD_LIBRARY_PATH points to dirB → Stage 2 resolves to libB ≠ libA.
	t.Setenv("LD_LIBRARY_PATH", dirB)

	verifyErr := v.Verify(binaryPath, deps)
	require.Error(t, verifyErr)

	var pathErr *ErrLibraryPathMismatch
	require.ErrorAs(t, verifyErr, &pathErr, "expected ErrLibraryPathMismatch")
	assert.Equal(t, libName, pathErr.SOName)
	assert.Equal(t, libA, pathErr.RecordedPath)
	assert.Equal(t, libB, pathErr.ResolvedPath)
}
