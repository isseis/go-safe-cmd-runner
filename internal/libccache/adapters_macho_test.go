//go:build test

package libccache

import (
	"debug/macho"
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- MatchWithMethod tests ----

// TestImportSymbolMatcher_MatchWithMethod_DeterminationMethod verifies that
// MatchWithMethod records the passed determinationMethod in results.
func TestImportSymbolMatcher_MatchWithMethod_DeterminationMethod(t *testing.T) {
	m := NewImportSymbolMatcher(newStubTable())
	wrappers := []WrapperEntry{{Name: "socket", Number: 41}}
	result := m.MatchWithMethod([]string{"socket"}, wrappers, DeterminationMethodLibCacheMatch)
	require.Len(t, result, 1)
	assert.Equal(t, DeterminationMethodLibCacheMatch, result[0].DeterminationMethod)
	assert.Equal(t, SourceLibsystemSymbolImport, result[0].Source)
}

// TestImportSymbolMatcher_MatchWithMethod_Dedup verifies that duplicate entries
// (same syscall number) are deduplicated.
func TestImportSymbolMatcher_MatchWithMethod_Dedup(t *testing.T) {
	m := NewImportSymbolMatcher(newStubTable())
	// Two wrappers mapping to the same syscall number.
	wrappers := []WrapperEntry{
		{Name: "socket", Number: 41},
		{Name: "socket_alias", Number: 41},
	}
	result := m.MatchWithMethod([]string{"socket", "socket_alias"}, wrappers, DeterminationMethodLibCacheMatch)
	assert.Len(t, result, 1, "duplicate syscall numbers should be deduplicated")
}

// ---- classifyLibSystemFallbackReason tests ----

// TestClassifyLibSystemFallbackReason_UmbrellaPresent verifies that the umbrella
// library name produces "dyld_extraction_unavailable".
func TestClassifyLibSystemFallbackReason_UmbrellaPresent(t *testing.T) {
	deps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libSystem.B.dylib"},
	}
	got := classifyLibSystemFallbackReason(deps, false)
	assert.Equal(t, "dyld_extraction_unavailable", got)
}

// TestClassifyLibSystemFallbackReason_KernelPresent verifies that the kernel dylib
// basename produces "dyld_extraction_unavailable".
func TestClassifyLibSystemFallbackReason_KernelPresent(t *testing.T) {
	deps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/system/libsystem_kernel.dylib"},
	}
	got := classifyLibSystemFallbackReason(deps, false)
	assert.Equal(t, "dyld_extraction_unavailable", got)
}

// TestClassifyLibSystemFallbackReason_NeitherPresent verifies that when neither
// libSystem umbrella nor kernel dylib is in deps, the reason is "missing_libsystem_dependency".
func TestClassifyLibSystemFallbackReason_NeitherPresent(t *testing.T) {
	deps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libc.dylib"},
		{SOName: "/usr/lib/libz.1.dylib"},
	}
	got := classifyLibSystemFallbackReason(deps, false)
	assert.Equal(t, "missing_libsystem_dependency", got)
}

// ---- MachoLibSystemAdapter.fallbackNameMatch tests ----

// newTestMachoAdapter constructs a MachoLibSystemAdapter with a temp cache directory.
func newTestMachoAdapter(t *testing.T) *MachoLibSystemAdapter {
	t.Helper()
	cacheDir := t.TempDir()
	mgr, err := NewMachoLibSystemCacheManager(cacheDir)
	require.NoError(t, err)
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	return NewMachoLibSystemAdapter(mgr, fs)
}

// TestMachoLibSystemAdapter_FallbackNameMatch_KnownNames verifies that known
// network syscall wrapper names are matched and return correct syscall info.
func TestMachoLibSystemAdapter_FallbackNameMatch_KnownNames(t *testing.T) {
	adapter := newTestMachoAdapter(t)
	// "socket" is a known network syscall wrapper in macOSSyscallEntries.
	result := adapter.fallbackNameMatch([]string{"socket", "not_a_syscall"})
	found := false
	for _, r := range result {
		if r.Name == "socket" {
			found = true
			assert.Equal(t, 97, r.Number)
			assert.True(t, r.IsNetwork)
			assert.Equal(t, DeterminationMethodSymbolNameMatch, r.DeterminationMethod)
			assert.Equal(t, SourceLibsystemSymbolImport, r.Source)
		}
	}
	assert.True(t, found, "socket should be in fallback results")
}

// TestMachoLibSystemAdapter_FallbackNameMatch_NocancelNames verifies that
// nocancel network wrappers are matched by the fallback path.
func TestMachoLibSystemAdapter_FallbackNameMatch_NocancelNames(t *testing.T) {
	adapter := newTestMachoAdapter(t)
	result := adapter.fallbackNameMatch([]string{"connect_nocancel", "recvmsg_nocancel"})
	require.Len(t, result, 2)

	names := make(map[string]common.SyscallInfo, len(result))
	for _, syscall := range result {
		names[syscall.Name] = syscall
		assert.True(t, syscall.IsNetwork)
		assert.Equal(t, DeterminationMethodSymbolNameMatch, syscall.DeterminationMethod)
	}

	assert.Equal(t, 409, names["connect_nocancel"].Number)
	assert.Equal(t, 401, names["recvmsg_nocancel"].Number)
}

// TestMachoLibSystemAdapter_FallbackNameMatch_UnknownSymbol verifies that unknown
// import symbols produce no results.
func TestMachoLibSystemAdapter_FallbackNameMatch_UnknownSymbol(t *testing.T) {
	adapter := newTestMachoAdapter(t)
	result := adapter.fallbackNameMatch([]string{"malloc", "free", "pthread_create"})
	assert.Empty(t, result, "non-syscall symbols should not be matched in fallback")
}

// TestMachoLibSystemAdapter_FallbackNameMatch_SortedByNumber verifies that results
// are sorted by syscall number.
func TestMachoLibSystemAdapter_FallbackNameMatch_SortedByNumber(t *testing.T) {
	adapter := newTestMachoAdapter(t)
	// socket=97, connect=98
	result := adapter.fallbackNameMatch([]string{"connect", "socket"})
	require.Len(t, result, 2)
	assert.Less(t, result[0].Number, result[1].Number, "results should be sorted by Number")
	assert.Equal(t, 97, result[0].Number)
	assert.Equal(t, 98, result[1].Number)
}

// ---- MachoLibSystemAdapter.GetSyscallInfos tests ----

// TestMachoLibSystemAdapter_GetSyscallInfos_FallbackOnNilSource verifies that when
// ResolveLibSystemKernel returns nil (e.g., dyld cache unavailable), the adapter falls
// back to symbol-name matching.
func TestMachoLibSystemAdapter_GetSyscallInfos_FallbackOnNilSource(t *testing.T) {
	adapter := newTestMachoAdapter(t)
	// Inject a nil-returning resolver to simulate a dyld cache unavailability.
	adapter.resolveFunc = func(_ []fileanalysis.LibEntry, _ safefileio.FileSystem, _ bool) (*machodylib.LibSystemKernelSource, error) {
		return nil, nil
	}
	dynDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libSystem.B.dylib"},
	}
	result, err := adapter.GetSyscallInfos(dynDeps, []string{"socket", "connect", "malloc"}, true)
	require.NoError(t, err)
	// socket and connect should be matched; malloc should not.
	names := make(map[string]bool)
	for _, r := range result {
		names[r.Name] = true
		assert.Equal(t, DeterminationMethodSymbolNameMatch, r.DeterminationMethod)
	}
	assert.True(t, names["socket"])
	assert.True(t, names["connect"])
	assert.False(t, names["malloc"])
}

// TestMachoLibSystemAdapter_GetSyscallInfos_NoLibSystem verifies that when dynDeps
// does not contain any libSystem-family library and hasLibSystemLoadCmd is false,
// the result is nil (no detection: neither DynLibDeps nor load commands indicate libSystem).
func TestMachoLibSystemAdapter_GetSyscallInfos_NoLibSystem(t *testing.T) {
	adapter := newTestMachoAdapter(t)
	dynDeps := []fileanalysis.LibEntry{
		{SOName: "/usr/lib/libobjc.A.dylib"},
	}
	result, err := adapter.GetSyscallInfos(dynDeps, []string{"socket"}, false)
	require.NoError(t, err)
	// Neither DynLibDeps nor hasLibSystemLoadCmd signals libSystem: resolver returns
	// nil and the adapter must also return nil (no fallback detection).
	assert.Nil(t, result)
}

// TestMachoLibSystemAdapter_GetSyscallInfos_EmptyImports verifies that no results
// are returned when importSymbols is empty.
func TestMachoLibSystemAdapter_GetSyscallInfos_EmptyImports(t *testing.T) {
	adapter := newTestMachoAdapter(t)
	result, err := adapter.GetSyscallInfos(nil, nil, false)
	require.NoError(t, err)
	assert.Empty(t, result)
}

// TestMachoLibSystemAdapter_GetSyscallInfos_FallbackOnEmptyWrappers verifies that when
// GetOrCreate returns an empty wrappers slice (e.g., the resolved dylib is a non-arm64
// stub with no usable __TEXT/__SYMTAB), the adapter falls back to symbol-name matching
// instead of returning no results (false negative).
func TestMachoLibSystemAdapter_GetSyscallInfos_FallbackOnEmptyWrappers(t *testing.T) {
	mgr, _ := newMachoTestCacheManager(t)
	fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	adapter := NewMachoLibSystemAdapter(mgr, fs)

	const libPath = "/usr/lib/system/libsystem_kernel.dylib"
	const libHash = "sha256:nonarm64stub"

	// Pre-populate the cache with empty wrappers by analyzing a non-arm64 Mach-O.
	// Use nil text bytes: the analyzer returns nil, nil for non-arm64 without examining
	// the text section content, so the bytes do not matter.
	nonArm64Bytes := buildRawMachoBytes(t, macho.CpuAmd64, nil, nil)
	cachedWrappers, err := mgr.GetOrCreate(libPath, libHash, func() ([]byte, error) {
		return nonArm64Bytes, nil
	})
	require.NoError(t, err)
	require.Empty(t, cachedWrappers, "non-arm64 analysis should return empty wrappers")

	// Inject a resolver that returns the pre-populated source so GetOrCreate hits the cache.
	adapter.resolveFunc = func(_ []fileanalysis.LibEntry, _ safefileio.FileSystem, _ bool) (*machodylib.LibSystemKernelSource, error) {
		return &machodylib.LibSystemKernelSource{
			Path:    libPath,
			Hash:    libHash,
			GetData: func() ([]byte, error) { return nil, errors.New("should not be called") },
		}, nil
	}

	// GetSyscallInfos must fall back to symbol-name matching when wrappers are empty.
	result, err := adapter.GetSyscallInfos(nil, []string{"socket", "connect", "malloc"}, false)
	require.NoError(t, err)
	names := make(map[string]bool)
	for _, r := range result {
		names[r.Name] = true
		assert.Equal(t, DeterminationMethodSymbolNameMatch, r.DeterminationMethod,
			"fallback path should use symbol-name matching")
	}
	assert.True(t, names["socket"], "socket should be matched via symbol-name fallback")
	assert.True(t, names["connect"], "connect should be matched via symbol-name fallback")
	assert.False(t, names["malloc"], "malloc should not be matched (not a syscall wrapper)")
}

// TestClassifyLibSystemFallbackReason_HasLoadCmd verifies that when hasLibSystemLoadCmd
// is true the reason is "dyld_extraction_unavailable" even when DynLibDeps is empty.
func TestClassifyLibSystemFallbackReason_HasLoadCmd(t *testing.T) {
	got := classifyLibSystemFallbackReason(nil, true)
	assert.Equal(t, "dyld_extraction_unavailable", got)
}

// TestMachoLibSystemAdapter_GetSyscallInfos_DyldCacheLibSystem verifies that when
// dynDeps is empty (macOS 11+ dyld cache case) but hasLibSystemLoadCmd is true,
// the adapter falls back to symbol-name matching when the resolver cannot extract
// from the dyld cache (e.g., non-Darwin or unavailable cache).
func TestMachoLibSystemAdapter_GetSyscallInfos_DyldCacheLibSystem(t *testing.T) {
	adapter := newTestMachoAdapter(t)
	// Inject a nil-returning resolver to simulate dyld cache unavailability.
	adapter.resolveFunc = func(_ []fileanalysis.LibEntry, _ safefileio.FileSystem, _ bool) (*machodylib.LibSystemKernelSource, error) {
		return nil, nil
	}
	// dynDeps is empty: MachODynLibAnalyzer did not record libSystem because it
	// lives in the dyld shared cache. hasLibSystemLoadCmd=true signals that the
	// binary's LC_LOAD_DYLIB contains /usr/lib/libSystem.B.dylib.
	result, err := adapter.GetSyscallInfos(nil, []string{"socket", "connect"}, true)
	require.NoError(t, err)
	// The resolver returns nil (simulated unavailability), so we fall through to
	// symbol-name matching. socket and connect must be detected.
	names := make(map[string]bool)
	for _, r := range result {
		names[r.Name] = true
	}
	assert.True(t, names["socket"], "socket must be detected via fallback when hasLibSystemLoadCmd=true")
	assert.True(t, names["connect"], "connect must be detected via fallback when hasLibSystemLoadCmd=true")
}
