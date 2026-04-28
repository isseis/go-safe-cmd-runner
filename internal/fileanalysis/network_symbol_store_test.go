//go:build test

package fileanalysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkSymbolStore_LoadNetworkSymbolAnalysis_Normal(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewNetworkSymbolStore(fileStore)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	rp, err := common.NewResolvedPath(testFile)
	require.NoError(t, err)

	// Save a record with NetworkSymbolAnalysis
	fileHash := "sha256:abc123def456"
	nsaData := &SymbolAnalysisData{
		DetectedSymbols:    []string{"socket"},
		DynamicLoadSymbols: []string{"dlopen"},
	}
	err = fileStore.Save(rp, &Record{
		ContentHash:    fileHash,
		SymbolAnalysis: nsaData,
	})
	require.NoError(t, err)

	// Load and verify
	loaded, err := store.LoadNetworkSymbolAnalysis(testFile, fileHash)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	require.Len(t, loaded.DetectedSymbols, 1)
	assert.Equal(t, "socket", loaded.DetectedSymbols[0])
	require.Len(t, loaded.DynamicLoadSymbols, 1)
	assert.Equal(t, "dlopen", loaded.DynamicLoadSymbols[0])
}

func TestNetworkSymbolStore_LoadNetworkSymbolAnalysis_NoNetworkSymbols(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewNetworkSymbolStore(fileStore)

	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	rp, err := common.NewResolvedPath(testFile)
	require.NoError(t, err)

	fileHash := "sha256:netnodynload"
	err = fileStore.Save(rp, &Record{
		ContentHash:    fileHash,
		SymbolAnalysis: &SymbolAnalysisData{},
	})
	require.NoError(t, err)

	loaded, err := store.LoadNetworkSymbolAnalysis(testFile, fileHash)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Nil(t, loaded.DetectedSymbols)
	assert.Nil(t, loaded.DynamicLoadSymbols)
}

func TestNetworkSymbolStore_LoadNetworkSymbolAnalysis_HashMismatch(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewNetworkSymbolStore(fileStore)

	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	rp, err := common.NewResolvedPath(testFile)
	require.NoError(t, err)

	// Save with one hash
	err = fileStore.Save(rp, &Record{
		ContentHash: "sha256:originalhash",
		SymbolAnalysis: &SymbolAnalysisData{
			DetectedSymbols: []string{"socket"},
		},
	})
	require.NoError(t, err)

	// Load with a different hash
	loaded, err := store.LoadNetworkSymbolAnalysis(testFile, "sha256:differenthash")
	assert.ErrorIs(t, err, ErrHashMismatch, "should return ErrHashMismatch for mismatched hash")
	assert.Nil(t, loaded)
}

func TestNetworkSymbolStore_LoadNetworkSymbolAnalysis_NoAnalysisData(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewNetworkSymbolStore(fileStore)

	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	rp, err := common.NewResolvedPath(testFile)
	require.NoError(t, err)

	// Save record without NetworkSymbolAnalysis
	fileHash := "sha256:abc123"
	err = fileStore.Save(rp, &Record{
		ContentHash:    fileHash,
		SymbolAnalysis: nil,
	})
	require.NoError(t, err)

	loaded, err := store.LoadNetworkSymbolAnalysis(testFile, fileHash)
	assert.NoError(t, err, "should return nil error when analysis is nil")
	assert.Nil(t, loaded)
}

func TestNetworkSymbolStore_LoadNetworkSymbolAnalysis_RecordNotFound(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewNetworkSymbolStore(fileStore)

	// Create a real file with no record saved — should return ErrRecordNotFound
	noRecordFile := filepath.Join(tmpDir, "no-record.bin")
	require.NoError(t, os.WriteFile(noRecordFile, []byte("content"), 0o644))
	loaded, err := store.LoadNetworkSymbolAnalysis(noRecordFile, "sha256:anyhash")
	assert.ErrorIs(t, err, ErrRecordNotFound, "should return ErrRecordNotFound for non-existent record")
	assert.Nil(t, loaded)
}

func TestNetworkSymbolStore_LoadNetworkSymbolAnalysis_SchemaVersionMismatch(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewNetworkSymbolStore(fileStore)

	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Write a raw JSON record with an older schema version directly (bypassing Store.Save).
	// This simulates a record written by an older version of the tool.
	getter := &mockPathGetter{}
	resolvedPath, err := common.NewResolvedPath(testFile)
	require.NoError(t, err)
	resolvedAnalysisDir, err := common.NewResolvedPath(analysisDir)
	require.NoError(t, err)
	recordFilePath, err := getter.GetHashFilePath(resolvedAnalysisDir, resolvedPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(recordFilePath), 0o750))

	oldRecord := map[string]interface{}{
		"schema_version": CurrentSchemaVersion - 1,
		"file_path":      testFile,
		"content_hash":   "sha256:abc123",
	}
	data, err := json.MarshalIndent(oldRecord, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recordFilePath, data, 0o600))

	// Loading should return SchemaVersionMismatchError, not ErrHashMismatch or similar.
	loaded, err := store.LoadNetworkSymbolAnalysis(testFile, "sha256:abc123")
	assert.Nil(t, loaded)
	require.Error(t, err)

	var schemaErr *SchemaVersionMismatchError
	assert.ErrorAs(t, err, &schemaErr,
		"should propagate SchemaVersionMismatchError without converting it to a cache miss error")
	assert.NotErrorIs(t, err, ErrHashMismatch,
		"SchemaVersionMismatchError should not be wrapped as ErrHashMismatch")
}
