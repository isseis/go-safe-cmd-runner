//go:build test

package dynlibanalysisstore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlibanalysisstore"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLibraryAnalyzer is a test double for LibraryAnalyzer.
type mockLibraryAnalyzer struct {
	result   *dynlibanalysisstore.DynamicLibAnalysisResult
	err      error
	callArgs []string
}

func (m *mockLibraryAnalyzer) AnalyzeLibrary(libPath string) (*dynlibanalysisstore.DynamicLibAnalysisResult, error) {
	m.callArgs = append(m.callArgs, libPath)
	return m.result, m.err
}

func newTestStore(t *testing.T, analyzer dynlibanalysisstore.LibraryAnalyzer) *dynlibanalysisstore.DynamicLibAnalysisStoreImpl {
	t.Helper()
	storeDir := filepath.Join(t.TempDir(), "dynlibstore")
	store, err := dynlibanalysisstore.NewDynamicLibAnalysisStore(storeDir, analyzer)
	require.NoError(t, err)
	return store
}

// TestDynamicLibAnalysisStore_LoadOrAnalyzeAndStore_Reuse verifies that a second call
// with the same path and hash reuses the stored result without invoking the analyzer again.
func TestDynamicLibAnalysisStore_LoadOrAnalyzeAndStore_Reuse(t *testing.T) {
	analyzer := &mockLibraryAnalyzer{
		result: &dynlibanalysisstore.DynamicLibAnalysisResult{
			SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
				DetectedSymbols: []string{"socket"},
			},
		},
	}
	store := newTestStore(t, analyzer)

	const libPath = "/usr/lib/libfoo.so.1"
	const libHash = "sha256:abc123"

	// First call: analyzer is invoked.
	result1, err := store.LoadOrAnalyzeAndStore(libPath, libHash)
	require.NoError(t, err)
	require.NotNil(t, result1)
	assert.Len(t, analyzer.callArgs, 1, "analyzer should be called once on first miss")

	// Second call with same path and hash: result is reused from disk.
	result2, err := store.LoadOrAnalyzeAndStore(libPath, libHash)
	require.NoError(t, err)
	require.NotNil(t, result2)
	assert.Len(t, analyzer.callArgs, 1, "analyzer should not be called again on reuse")

	require.NotNil(t, result2.SymbolAnalysis)
	assert.Equal(t, []string{"socket"}, result2.SymbolAnalysis.DetectedSymbols)
}

// TestDynamicLibAnalysisStore_LoadOrAnalyzeAndStore_HashChanged verifies that
// when the library hash changes, the stored result is not reused and a fresh
// analysis is performed.
func TestDynamicLibAnalysisStore_LoadOrAnalyzeAndStore_HashChanged(t *testing.T) {
	analyzer := &mockLibraryAnalyzer{
		result: &dynlibanalysisstore.DynamicLibAnalysisResult{},
	}
	store := newTestStore(t, analyzer)

	const libPath = "/usr/lib/libbar.so.2"
	const hash1 = "sha256:aaa111"
	const hash2 = "sha256:bbb222"

	// First call: analysis stored with hash1.
	_, err := store.LoadOrAnalyzeAndStore(libPath, hash1)
	require.NoError(t, err)
	assert.Len(t, analyzer.callArgs, 1)

	// Second call with different hash: stored result is invalid, re-analysis runs.
	_, err = store.LoadOrAnalyzeAndStore(libPath, hash2)
	require.NoError(t, err)
	assert.Len(t, analyzer.callArgs, 2, "analyzer should be called again when hash changes")
}

// TestDynamicLibAnalysisStore_CorruptFile_Reanalyze verifies that a corrupt
// store file is treated as not found and a fresh analysis is performed.
func TestDynamicLibAnalysisStore_CorruptFile_Reanalyze(t *testing.T) {
	analyzer := &mockLibraryAnalyzer{
		result: &dynlibanalysisstore.DynamicLibAnalysisResult{},
	}
	storeDir := filepath.Join(t.TempDir(), "dynlibstore")
	store, err := dynlibanalysisstore.NewDynamicLibAnalysisStore(storeDir, analyzer)
	require.NoError(t, err)

	const libPath = "/usr/lib/libbaz.so.3"
	const libHash = "sha256:deadbeef"

	// First call: analysis stored successfully.
	_, err = store.LoadOrAnalyzeAndStore(libPath, libHash)
	require.NoError(t, err)
	assert.Len(t, analyzer.callArgs, 1)

	// Corrupt all store files.
	entries, globErr := filepath.Glob(filepath.Join(storeDir, "*"))
	require.NoError(t, globErr)
	for _, entry := range entries {
		//nolint:gosec // G306: test file
		require.NoError(t, os.WriteFile(entry, []byte("not-valid-json"), 0o644))
	}

	// Call again: corrupt file treated as not found, re-analysis runs.
	_, err = store.LoadOrAnalyzeAndStore(libPath, libHash)
	require.NoError(t, err)
	assert.Len(t, analyzer.callArgs, 2, "analyzer should be called again on corrupt file")
}

// TestDynamicLibAnalysisStore_LoadAnalysis_NotFound verifies that LoadAnalysis returns
// ErrAnalysisNotFound when no entry exists for the given library.
func TestDynamicLibAnalysisStore_LoadAnalysis_NotFound(t *testing.T) {
	store := newTestStore(t, nil)

	_, err := store.LoadAnalysis("/usr/lib/libunknown.so.1", "sha256:000")
	assert.ErrorIs(t, err, dynlibanalysisstore.ErrAnalysisNotFound)
}

// TestDynamicLibAnalysisStore_LoadAnalysis_HashMismatch verifies that LoadAnalysis
// returns ErrAnalysisNotFound when the stored hash does not match the requested hash.
func TestDynamicLibAnalysisStore_LoadAnalysis_HashMismatch(t *testing.T) {
	analyzer := &mockLibraryAnalyzer{
		result: &dynlibanalysisstore.DynamicLibAnalysisResult{},
	}
	store := newTestStore(t, analyzer)

	const libPath = "/usr/lib/libhash.so.1"
	_, err := store.LoadOrAnalyzeAndStore(libPath, "sha256:original")
	require.NoError(t, err)

	_, err = store.LoadAnalysis(libPath, "sha256:different")
	assert.ErrorIs(t, err, dynlibanalysisstore.ErrAnalysisNotFound)
}
