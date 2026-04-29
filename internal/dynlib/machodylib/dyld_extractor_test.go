//go:build test && darwin

package machodylib

import (
	"bytes"
	"debug/macho"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireDyldCacheAvailable skips t when not on darwin/arm64 or when no dyld
// shared cache file is found in dyldSharedCachePaths.
func requireDyldCacheAvailable(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("dyld shared cache extraction only applicable on darwin/arm64")
	}
	for _, p := range dyldSharedCachePaths {
		if _, err := os.Stat(p); err == nil {
			return
		}
	}
	t.Skipf("no dyld shared cache found in %v", dyldSharedCachePaths)
}

// TestExtractLibSystemKernel_Live skips the test when
// the dyld shared cache cannot be expected to be present (non-darwin or non-arm64).
// On darwin arm64, it attempts an actual extraction and verifies the invariants.
func TestExtractLibSystemKernel_Live(t *testing.T) {
	requireDyldCacheAvailable(t)

	result, err := ExtractLibSystemKernel()
	require.NoError(t, err)

	if result == nil {
		t.Skip("ExtractLibSystemKernel returned nil; " +
			"libsystem_kernel.dylib may not be present in this environment's dyld shared cache")
	}

	// Verify invariants on a successful extraction.
	assert.NotEmpty(t, result.Data, "extracted Data must not be empty")
	assert.True(t, strings.HasPrefix(result.Hash, "sha256:"),
		"extracted Hash must start with 'sha256:', got %q", result.Hash)
	assert.Greater(t, len(result.Hash), len("sha256:"),
		"extracted Hash must contain a hex digest after the prefix")
}

// TestExtractLibSystemKernel_NoCachePaths verifies that when no
// dyld shared cache is available, the function returns nil, nil.
//
// This test overrides the package-level dyldSharedCachePaths to point to
// non-existent paths, simulating the absence of a shared cache.
func TestExtractLibSystemKernel_NoCachePaths(t *testing.T) {
	original := dyldSharedCachePaths
	t.Cleanup(func() { dyldSharedCachePaths = original })

	// Point to paths that never exist.
	dyldSharedCachePaths = []string{
		"/nonexistent/dyld_shared_cache_arm64e",
		"/nonexistent/dyld_shared_cache_arm64",
	}

	result, err := ExtractLibSystemKernel()
	require.NoError(t, err)
	assert.Nil(t, result, "expected nil result when no cache paths exist")
}

// TestExtractLibSystemKernel_SymbolNames verifies that the extracted Mach-O
// contains known libsystem_kernel.dylib BSD syscall wrapper symbols and no
// dyld-internal C++ symbols.
//
// Regression test for the bug where symtab.symoff and symtab.stroff were
// incorrectly offset by linkeditSeg.fileOff, causing 256 nlist_64 entries to
// be skipped and symbols from an adjacent library to appear instead.
func TestExtractLibSystemKernel_SymbolNames(t *testing.T) {
	requireDyldCacheAvailable(t)

	result, err := ExtractLibSystemKernel()
	require.NoError(t, err)
	if result == nil {
		t.Skip("ExtractLibSystemKernel returned nil; skipping symbol name test")
	}

	mf, err := macho.NewFile(bytes.NewReader(result.Data))
	require.NoError(t, err)
	defer func() { _ = mf.Close() }()

	require.NotNil(t, mf.Symtab, "extracted Mach-O must have a symbol table")

	symSet := make(map[string]bool, len(mf.Symtab.Syms))
	for _, s := range mf.Symtab.Syms {
		symSet[s.Name] = true
	}

	// The symbol count must be in a realistic range for libsystem_kernel.
	// A count far below this indicates the wrong portion of the merged symbol
	// table was read (the pre-fix extraction yielded only 94 symbols).
	assert.Greater(t, len(mf.Symtab.Syms), 1000,
		"extracted symbol count %d is too low; expected >1000 for libsystem_kernel.dylib "+
			"(may indicate symoff/stroff are not treated as absolute file offsets)",
		len(mf.Symtab.Syms))

	// These BSD syscall wrapper symbols have been stable exports of
	// libsystem_kernel.dylib across many macOS versions.
	knownSymbols := []string{
		"___accept",
		"_close",
		"_read",
		"_write",
		"___open",
	}
	for _, name := range knownSymbols {
		assert.True(t, symSet[name],
			"expected libsystem_kernel symbol %q not found "+
				"(may indicate symbols from a different library were extracted)", name)
	}

	// dyld-internal C++ symbols must not appear. Their presence indicates
	// that the symbol table offset was shifted to an adjacent library.
	for _, s := range mf.Symtab.Syms {
		if strings.HasPrefix(s.Name, "__ZN3lsl") {
			t.Errorf("found dyld-internal symbol %q; wrong library's symbols were extracted", s.Name)
		}
	}
}
