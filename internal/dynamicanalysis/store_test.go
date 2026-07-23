//go:build test

package dynamicanalysis_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynamicanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeLibFile writes content to a file under t.TempDir() and returns both
// its path and its "sha256:<hex>" content hash. LoadOrAnalyzeAndStore now
// verifies libPath's actual content hash against the caller-supplied libHash
// before persisting, so tests must use a real file with a hash that
// actually matches — a placeholder path/hash pair is rejected as a mismatch.
func writeLibFile(t *testing.T, content string) (path, hash string) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "lib.so")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path, fileContentHash(t, path)
}

// fileContentHash returns the "sha256:<hex>" content hash of the file at path.
func fileContentHash(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // G304: test-controlled path
	require.NoError(t, err)
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// mockAnalyzer is a test double for Analyzer.
type mockAnalyzer struct {
	result   *dynamicanalysis.Result
	err      error
	callArgs []string
}

func (m *mockAnalyzer) AnalyzeLibrary(_ safefileio.File, libPath string) (*dynamicanalysis.Result, error) {
	m.callArgs = append(m.callArgs, libPath)
	return m.result, m.err
}

// contentCapturingAnalyzer is a test double for Analyzer that records the
// exact bytes it reads from the fd LoadOrAnalyzeAndStore passes it. It reads
// via ReadAt at offset 0 (not the sequential Reader), matching how the real
// analyzers in this codebase read a shared fd: LoadOrAnalyzeAndStore already
// consumed the sequential read position computing the verification hash
// before calling AnalyzeLibrary, so a conforming Analyzer must not depend on
// that position either.
type contentCapturingAnalyzer struct {
	capturedContent []byte
	result          *dynamicanalysis.Result
}

func (a *contentCapturingAnalyzer) AnalyzeLibrary(file safefileio.File, _ string) (*dynamicanalysis.Result, error) {
	fi, err := file.Stat()
	if err != nil {
		return nil, err
	}
	buf := make([]byte, fi.Size())
	if _, err := file.ReadAt(buf, 0); err != nil {
		return nil, err
	}
	a.capturedContent = buf
	return a.result, nil
}

func newTestStore(t *testing.T, analyzer dynamicanalysis.Analyzer) dynamicanalysis.Store {
	t.Helper()
	storeDir := filepath.Join(t.TempDir(), "dynlibstore")
	store, err := dynamicanalysis.New(storeDir, analyzer)
	require.NoError(t, err)
	return store
}

// TestStore_LoadOrAnalyzeAndStore_Reuse verifies that a second call
// with the same path and hash reuses the stored result without invoking the analyzer again.
func TestStore_LoadOrAnalyzeAndStore_Reuse(t *testing.T) {
	analyzer := &mockAnalyzer{
		result: &dynamicanalysis.Result{
			SymbolAnalysis: &fileanalysis.SymbolAnalysisData{
				DetectedSymbols: []fileanalysis.DetectedSymbol{{Name: "socket"}},
			},
		},
	}
	store := newTestStore(t, analyzer)

	libPath, libHash := writeLibFile(t, "lib-content-reuse")

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
	assert.Equal(t, []fileanalysis.DetectedSymbol{{Name: "socket"}}, result2.SymbolAnalysis.DetectedSymbols)
}

// TestStore_LoadOrAnalyzeAndStore_HashChanged verifies that when the library hash
// changes, the stored result is not reused and a fresh analysis is performed.
func TestStore_LoadOrAnalyzeAndStore_HashChanged(t *testing.T) {
	analyzer := &mockAnalyzer{
		result: &dynamicanalysis.Result{},
	}
	store := newTestStore(t, analyzer)

	libPath, hash1 := writeLibFile(t, "lib-content-v1")

	// First call: analysis stored with hash1.
	_, err := store.LoadOrAnalyzeAndStore(libPath, hash1)
	require.NoError(t, err)
	assert.Len(t, analyzer.callArgs, 1)

	// Library recompiled: content (and hash) changed.
	require.NoError(t, os.WriteFile(libPath, []byte("lib-content-v2"), 0o644))
	hash2 := fileContentHash(t, libPath)

	// Second call with the new hash: stored result under hash1 is invalid,
	// re-analysis runs against the new content.
	_, err = store.LoadOrAnalyzeAndStore(libPath, hash2)
	require.NoError(t, err)
	assert.Len(t, analyzer.callArgs, 2, "analyzer should be called again when hash changes")
}

// TestStore_CorruptFile_Reanalyze verifies that a corrupt store file is treated
// as not found and a fresh analysis is performed.
func TestStore_CorruptFile_Reanalyze(t *testing.T) {
	analyzer := &mockAnalyzer{
		result: &dynamicanalysis.Result{},
	}
	storeDir := filepath.Join(t.TempDir(), "dynlibstore")
	store, err := dynamicanalysis.New(storeDir, analyzer)
	require.NoError(t, err)

	libPath, libHash := writeLibFile(t, "lib-content-corrupt")

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

// TestStore_LoadOrAnalyzeAndStore_HashKeyMismatchError verifies that
// LoadOrAnalyzeAndStore returns ErrLibraryHashKeyMismatch, and does not
// persist a result, when libPath's actual content hash does not match the
// caller-supplied libHash. This models a TOCTOU-style substitution:
// the caller determined libHash from one version of the file, but by the
// time the library is analyzed, libPath resolves to different content.
func TestStore_LoadOrAnalyzeAndStore_HashKeyMismatchError(t *testing.T) {
	analyzer := &mockAnalyzer{
		result: &dynamicanalysis.Result{},
	}
	storeDir := filepath.Join(t.TempDir(), "dynlibstore")
	store, err := dynamicanalysis.New(storeDir, analyzer)
	require.NoError(t, err)

	libPath, _ := writeLibFile(t, "lib-content-actual")
	wrongHash := fileContentHash(t, libPath) + "00" // guaranteed not to match

	_, err = store.LoadOrAnalyzeAndStore(libPath, wrongHash)
	require.ErrorIs(t, err, dynamicanalysis.ErrLibraryHashKeyMismatch)
	assert.Empty(t, analyzer.callArgs, "the hash check runs before AnalyzeLibrary; analysis must not run on a mismatch")

	// The mismatched result must not be persisted: a later call with the
	// correct hash must still trigger a fresh analysis, not reuse anything
	// written by the failed call above.
	_, err = store.LoadAnalysis(libPath, wrongHash)
	assert.ErrorIs(t, err, dynamicanalysis.ErrAnalysisNotFound)
}

// TestStore_LoadOrAnalyzeAndStore_AnalysisReadsHashVerifiedContent verifies
// that the fd passed to Analyzer.AnalyzeLibrary carries the exact content
// that was just hash-verified against libHash, i.e. that verification and
// analysis are bound to a single read rather than two independent opens
// that a file swap could make disagree.
func TestStore_LoadOrAnalyzeAndStore_AnalysisReadsHashVerifiedContent(t *testing.T) {
	const content = "lib-content-verified-flow"
	analyzer := &contentCapturingAnalyzer{result: &dynamicanalysis.Result{}}
	store := newTestStore(t, analyzer)

	libPath, libHash := writeLibFile(t, content)

	_, err := store.LoadOrAnalyzeAndStore(libPath, libHash)
	require.NoError(t, err)
	assert.Equal(t, content, string(analyzer.capturedContent))
}

// TestStore_LoadAnalysis_NotFound verifies that LoadAnalysis returns
// ErrAnalysisNotFound when no entry exists for the given library.
func TestStore_LoadAnalysis_NotFound(t *testing.T) {
	store := newTestStore(t, nil)

	_, err := store.LoadAnalysis("/usr/lib/libunknown.so.1", "sha256:000")
	assert.ErrorIs(t, err, dynamicanalysis.ErrAnalysisNotFound)
}

// TestStore_LoadAnalysis_HashMismatch verifies that LoadAnalysis returns
// ErrAnalysisNotFound when the stored hash does not match the requested hash.
func TestStore_LoadAnalysis_HashMismatch(t *testing.T) {
	analyzer := &mockAnalyzer{
		result: &dynamicanalysis.Result{},
	}
	store := newTestStore(t, analyzer)

	libPath, libHash := writeLibFile(t, "lib-content-hash")
	_, err := store.LoadOrAnalyzeAndStore(libPath, libHash)
	require.NoError(t, err)

	_, err = store.LoadAnalysis(libPath, "sha256:different")
	assert.ErrorIs(t, err, dynamicanalysis.ErrAnalysisNotFound)
}
