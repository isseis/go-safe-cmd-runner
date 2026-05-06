//go:build test

package fileanalysis

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupShebangStoreTest creates a temporary directory with a file analysis store and
// the ShebangInterpreterStore under test.
func setupShebangStoreTest(t *testing.T) (store *Store, shebangStore ShebangInterpreterStore, tmpDir string) {
	t.Helper()
	tmpDir = commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")
	var err error
	store, err = NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)
	shebangStore = NewShebangInterpreterStore(store)
	return
}

// saveRecord saves a Record for a file in the given tmpDir.
// Creates the file if it does not exist.
func saveRecord(t *testing.T, store *Store, tmpDir, name string, record *Record) (filePath string, rp common.ResolvedPath) {
	t.Helper()
	filePath = filepath.Join(tmpDir, name)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o644))
	}
	var err error
	rp, err = common.NewResolvedPath(filePath)
	require.NoError(t, err)
	require.NoError(t, store.Save(rp, record))
	return
}

// TC-01: direct form shebang, both script and interpreter records present.
// Expected: return (interpPath, interpHash, nil).
func TestShebangInterpreterStore_TC01_DirectForm(t *testing.T) {
	store, shebangStore, tmpDir := setupShebangStoreTest(t)

	interpHash := "sha256:interphash01"
	interpPath, _ := saveRecord(t, store, tmpDir, "bash", &Record{
		ContentHash: interpHash,
	})

	scriptHash := "sha256:scripthash01"
	scriptPath, _ := saveRecord(t, store, tmpDir, "script.sh", &Record{
		ContentHash: scriptHash,
		ShebangInterpreter: &ShebangInterpreterInfo{
			InterpreterPath: interpPath,
		},
	})

	gotPath, gotHash, err := shebangStore.LoadInterpreterAnalysisPath(scriptPath, scriptHash)
	require.NoError(t, err)
	assert.Equal(t, interpPath, gotPath)
	assert.Equal(t, interpHash, gotHash)
}

// TC-02: env form shebang, ResolvedPath is used instead of InterpreterPath.
// Expected: return (ResolvedPath, interpHash, nil).
func TestShebangInterpreterStore_TC02_EnvForm(t *testing.T) {
	store, shebangStore, tmpDir := setupShebangStoreTest(t)

	resolvedInterpHash := "sha256:python3hash02"
	resolvedInterpPath, _ := saveRecord(t, store, tmpDir, "python3", &Record{
		ContentHash: resolvedInterpHash,
	})

	// Save env binary record (should NOT be returned because ResolvedPath is set)
	envHash := "sha256:envhash02"
	envPath, _ := saveRecord(t, store, tmpDir, "env", &Record{
		ContentHash: envHash,
	})

	scriptHash := "sha256:scripthash02"
	scriptPath, _ := saveRecord(t, store, tmpDir, "script.py", &Record{
		ContentHash: scriptHash,
		ShebangInterpreter: &ShebangInterpreterInfo{
			InterpreterPath: envPath,
			CommandName:     "python3",
			ResolvedPath:    resolvedInterpPath,
		},
	})

	gotPath, gotHash, err := shebangStore.LoadInterpreterAnalysisPath(scriptPath, scriptHash)
	require.NoError(t, err)
	assert.Equal(t, resolvedInterpPath, gotPath, "ResolvedPath should be preferred over InterpreterPath")
	assert.Equal(t, resolvedInterpHash, gotHash)
}

// TC-03: script record not found.
// Expected: return ("", "", ErrRecordNotFound).
func TestShebangInterpreterStore_TC03_ScriptRecordNotFound(t *testing.T) {
	_, shebangStore, tmpDir := setupShebangStoreTest(t)

	scriptPath := filepath.Join(tmpDir, "nonexistent.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("content"), 0o644))

	_, _, err := shebangStore.LoadInterpreterAnalysisPath(scriptPath, "sha256:anyhash")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRecordNotFound), "expected ErrRecordNotFound, got: %v", err)
}

// TC-04: contentHash mismatch.
// Expected: return ("", "", ErrHashMismatch).
func TestShebangInterpreterStore_TC04_HashMismatch(t *testing.T) {
	store, shebangStore, tmpDir := setupShebangStoreTest(t)

	scriptHash := "sha256:scripthash04"
	scriptPath, _ := saveRecord(t, store, tmpDir, "script.sh", &Record{
		ContentHash: scriptHash,
		ShebangInterpreter: &ShebangInterpreterInfo{
			InterpreterPath: "/usr/bin/bash",
		},
	})

	_, _, err := shebangStore.LoadInterpreterAnalysisPath(scriptPath, "sha256:wronghash")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrHashMismatch), "expected ErrHashMismatch, got: %v", err)
}

// TC-05: ShebangInterpreter is nil (not a shebang script).
// Expected: return ("", "", nil).
func TestShebangInterpreterStore_TC05_NoShebang(t *testing.T) {
	store, shebangStore, tmpDir := setupShebangStoreTest(t)

	scriptHash := "sha256:scripthash05"
	scriptPath, _ := saveRecord(t, store, tmpDir, "binary", &Record{
		ContentHash:        scriptHash,
		ShebangInterpreter: nil,
	})

	gotPath, gotHash, err := shebangStore.LoadInterpreterAnalysisPath(scriptPath, scriptHash)
	require.NoError(t, err)
	assert.Empty(t, gotPath)
	assert.Empty(t, gotHash)
}

// TC-06: interpreter record not found.
// Expected: return ("", "", ErrInterpreterRecordMissing).
func TestShebangInterpreterStore_TC06_InterpRecordNotFound(t *testing.T) {
	store, shebangStore, tmpDir := setupShebangStoreTest(t)

	// interpPath exists as a file but has no analysis record saved
	interpPath := filepath.Join(tmpDir, "bash_no_record")
	require.NoError(t, os.WriteFile(interpPath, []byte("content"), 0o644))

	scriptHash := "sha256:scripthash06"
	scriptPath, _ := saveRecord(t, store, tmpDir, "script.sh", &Record{
		ContentHash: scriptHash,
		ShebangInterpreter: &ShebangInterpreterInfo{
			InterpreterPath: interpPath,
		},
	})

	_, _, err := shebangStore.LoadInterpreterAnalysisPath(scriptPath, scriptHash)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInterpreterRecordMissing), "expected ErrInterpreterRecordMissing, got: %v", err)
}

// TC-07: interpreter record load returns a non-ErrRecordNotFound error (e.g. schema mismatch).
// Simulated by saving a record with an invalid JSON content in the analysis dir.
// Expected: return ("", "", non-nil error) that is NOT ErrInterpreterRecordMissing.
func TestShebangInterpreterStore_TC07_InterpLoadError(t *testing.T) {
	store, shebangStore, tmpDir := setupShebangStoreTest(t)

	// Save an interpreter record normally first to get the record path
	interpHash := "sha256:interphash07"
	interpPath, interpRP := saveRecord(t, store, tmpDir, "bash07", &Record{
		ContentHash: interpHash,
	})

	// Overwrite the analysis record file with invalid JSON to simulate a corrupted record
	recordPath, err := store.getRecordPath(interpRP)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recordPath, []byte("not valid json"), 0o600))

	scriptHash := "sha256:scripthash07"
	scriptPath, _ := saveRecord(t, store, tmpDir, "script07.sh", &Record{
		ContentHash: scriptHash,
		ShebangInterpreter: &ShebangInterpreterInfo{
			InterpreterPath: interpPath,
		},
	})

	_, _, err = shebangStore.LoadInterpreterAnalysisPath(scriptPath, scriptHash)
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrInterpreterRecordMissing), "should NOT be ErrInterpreterRecordMissing for parse error")
}

// TC-08: interpreter ContentHash is empty.
// Expected: return ("", "", ErrInterpreterRecordMissing).
func TestShebangInterpreterStore_TC08_InterpEmptyContentHash(t *testing.T) {
	store, shebangStore, tmpDir := setupShebangStoreTest(t)

	// Save interpreter record with empty ContentHash
	interpPath, _ := saveRecord(t, store, tmpDir, "bash08", &Record{
		ContentHash: "", // empty
	})

	scriptHash := "sha256:scripthash08"
	scriptPath, _ := saveRecord(t, store, tmpDir, "script08.sh", &Record{
		ContentHash: scriptHash,
		ShebangInterpreter: &ShebangInterpreterInfo{
			InterpreterPath: interpPath,
		},
	})

	_, _, err := shebangStore.LoadInterpreterAnalysisPath(scriptPath, scriptHash)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInterpreterRecordMissing), "expected ErrInterpreterRecordMissing, got: %v", err)
}
