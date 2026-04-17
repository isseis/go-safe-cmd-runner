//go:build test && linux

package libccache

import (
	"debug/elf"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeELFFile writes the given elfBuilder bytes to a temp file and returns its path.
func writeELFFile(t *testing.T, eb *elfBuilder) string {
	t.Helper()
	data := eb.buildBytes(t)
	f, err := os.CreateTemp(t.TempDir(), "libc*.so")
	require.NoError(t, err)
	_, err = f.Write(data)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// buildCacheManager creates a LibcCacheManager backed by a temp directory.
func buildCacheManager(t *testing.T) (*LibcCacheManager, string) {
	t.Helper()
	cacheDir := filepath.Join(t.TempDir(), "lib-cache")
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	analyzer := NewLibcWrapperAnalyzer(elfanalyzer.NewSyscallAnalyzer())
	mgr, err := NewLibcCacheManager(cacheDir, fs, analyzer)
	require.NoError(t, err)
	return mgr, cacheDir
}

// minimalELFBuilder returns an elfBuilder with one simple syscall wrapper.
func minimalELFBuilder() *elfBuilder {
	base := uint64(0x1000)
	funcCode := append(movEAX(1), syscallInsn...) // write syscall (number 1)
	return &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: funcCode,
		textBase: base,
		symbols: []elfSym{
			{name: "write", value: base, size: uint64(len(funcCode)), global: true},
		},
	}
}

// TestLibcCacheManager_MissCreatesCache verifies that a cache file is created on a missed cache.
func TestLibcCacheManager_MissCreatesCache(t *testing.T) {
	mgr, cacheDir := buildCacheManager(t)
	libcPath := writeELFFile(t, minimalELFBuilder())

	wrappers, err := mgr.GetOrCreate(libcPath, "sha256:aabbcc")
	require.NoError(t, err)
	assert.NotNil(t, wrappers)

	// A cache file must have been written.
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "expected exactly one cache file")
}

// TestLibcCacheManager_HitReusesCache verifies that on a second call with the same hash the
// analyzer is not called and the cached result is returned.
func TestLibcCacheManager_HitReusesCache(t *testing.T) {
	mgr, cacheDir := buildCacheManager(t)
	libcPath := writeELFFile(t, minimalELFBuilder())
	hash := "sha256:deadbeef"

	wrappers1, err := mgr.GetOrCreate(libcPath, hash)
	require.NoError(t, err)

	// Overwrite the libc file with garbage so that re-analysis would fail.
	require.NoError(t, os.WriteFile(libcPath, []byte("not an ELF"), 0o600))

	wrappers2, err := mgr.GetOrCreate(libcPath, hash)
	require.NoError(t, err, "second call should use cache, not re-read libc")
	assert.Equal(t, wrappers1, wrappers2)

	// Still one cache file.
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

// TestLibcCacheManager_HashMismatchRecreates verifies that a different hash triggers re-analysis.
func TestLibcCacheManager_HashMismatchRecreates(t *testing.T) {
	mgr, _ := buildCacheManager(t)
	libcPath := writeELFFile(t, minimalELFBuilder())

	_, err := mgr.GetOrCreate(libcPath, "sha256:hash1")
	require.NoError(t, err)

	// Call again with a different hash — libc unchanged (same content) but hash differs.
	wrappers2, err := mgr.GetOrCreate(libcPath, "sha256:hash2")
	require.NoError(t, err)
	assert.NotNil(t, wrappers2)
}

// TestLibcCacheManager_CorruptCacheRecreates verifies that a corrupt cache file causes re-analysis.
func TestLibcCacheManager_CorruptCacheRecreates(t *testing.T) {
	mgr, cacheDir := buildCacheManager(t)
	libcPath := writeELFFile(t, minimalELFBuilder())
	hash := "sha256:abc"

	_, err := mgr.GetOrCreate(libcPath, hash)
	require.NoError(t, err)

	// Corrupt the cache file.
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	cacheFile := filepath.Join(cacheDir, entries[0].Name())
	require.NoError(t, os.WriteFile(cacheFile, []byte("{bad json"), 0o640))

	// Should re-analyze without error.
	wrappers, err := mgr.GetOrCreate(libcPath, hash)
	require.NoError(t, err)
	assert.NotNil(t, wrappers)
}

// TestLibcCacheManager_SchemaVersionMismatchRecreates verifies that a different schema version
// causes a cache miss.
func TestLibcCacheManager_SchemaVersionMismatchRecreates(t *testing.T) {
	mgr, cacheDir := buildCacheManager(t)
	libcPath := writeELFFile(t, minimalELFBuilder())
	hash := "sha256:abc"

	_, err := mgr.GetOrCreate(libcPath, hash)
	require.NoError(t, err)

	// Overwrite cache with a different schema_version.
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	cacheFile := filepath.Join(cacheDir, entries[0].Name())
	oldCache := LibcCacheFile{
		SchemaVersion:   LibcCacheSchemaVersion + 1,
		LibPath:         libcPath,
		LibHash:         hash,
		AnalyzedAt:      "2026-01-01T00:00:00Z",
		SyscallWrappers: nil,
	}
	data, err := json.Marshal(oldCache)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(cacheFile, data, 0o640))

	// Should re-analyze.
	wrappers, err := mgr.GetOrCreate(libcPath, hash)
	require.NoError(t, err)
	assert.NotNil(t, wrappers)
}

// TestLibcCacheManager_SortOrder verifies that the written cache has wrappers sorted by
// Number ascending, then Name ascending within the same Number.
func TestLibcCacheManager_SortOrder(t *testing.T) {
	base := uint64(0x1000)
	// Each function is exactly 7 bytes: movEAX(n) (5B) + syscall (2B).
	funcA := append(movEAX(2), syscallInsn...) // sys_open = 2 on first call
	funcB := append(movEAX(1), syscallInsn...) // sys_write = 1
	funcC := append(movEAX(2), syscallInsn...) // also syscall 2
	text := append(append(funcA, funcB...), funcC...)
	sizeA := uint64(7)
	eb := &elfBuilder{
		machine:  elf.EM_X86_64,
		textCode: text,
		textBase: base,
		symbols: []elfSym{
			{name: "open", value: base, size: sizeA, global: true},
			{name: "write", value: base + sizeA, size: sizeA, global: true},
			{name: "openat", value: base + 2*sizeA, size: sizeA, global: true},
		},
	}
	libcPath := writeELFFile(t, eb)
	mgr, cacheDir := buildCacheManager(t)

	wrappers, err := mgr.GetOrCreate(libcPath, "sha256:sort")
	require.NoError(t, err)
	require.Len(t, wrappers, 3)
	assert.Equal(t, 1, wrappers[0].Number, "first entry should be syscall 1")
	assert.Equal(t, 2, wrappers[1].Number, "second entry should be syscall 2")
	assert.Equal(t, "open", wrappers[1].Name, "within syscall 2, 'open' < 'openat'")
	assert.Equal(t, 2, wrappers[2].Number, "third entry should also be syscall 2")
	assert.Equal(t, "openat", wrappers[2].Name)

	// Verify the cache file contains the same order.
	entries, err := os.ReadDir(cacheDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	raw, err := os.ReadFile(filepath.Join(cacheDir, entries[0].Name()))
	require.NoError(t, err)
	var cached LibcCacheFile
	require.NoError(t, json.Unmarshal(raw, &cached))
	assert.Equal(t, wrappers, cached.SyscallWrappers)
}

// TestLibcCacheManager_LibcNotAccessible verifies that ErrLibcFileNotAccessible is returned
// when the libc file cannot be opened.
func TestLibcCacheManager_LibcNotAccessible(t *testing.T) {
	mgr, _ := buildCacheManager(t)
	_, err := mgr.GetOrCreate("/nonexistent/path/to/libc.so.6", "sha256:x")
	assert.ErrorIs(t, err, ErrLibcFileNotAccessible)
}

// TestLibcCacheManager_WriteFailure verifies that ErrCacheWriteFailed is returned when
// the cache directory is not writable.
func TestLibcCacheManager_WriteFailure(t *testing.T) {
	mgr, cacheDir := buildCacheManager(t)
	libcPath := writeELFFile(t, minimalELFBuilder())

	// Make the cache directory read-only so os.WriteFile will fail.
	require.NoError(t, os.Chmod(cacheDir, 0o500))
	defer os.Chmod(cacheDir, 0o750) //nolint:errcheck // best-effort restore for cleanup

	_, err := mgr.GetOrCreate(libcPath, "sha256:fail")
	assert.ErrorIs(t, err, ErrCacheWriteFailed)
}
