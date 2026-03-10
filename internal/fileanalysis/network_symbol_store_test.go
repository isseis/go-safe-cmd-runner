//go:build test

package fileanalysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

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

	// Save a record with NetworkSymbolAnalysis
	fileHash := "sha256:abc123def456"
	analyzedAt := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	nsaData := &NetworkSymbolAnalysisData{
		AnalyzedAt:        analyzedAt,
		HasNetworkSymbols: true,
		DetectedSymbols: []DetectedSymbolEntry{
			{Name: "socket", Category: "socket"},
		},
		DynamicLoadSymbols: []DetectedSymbolEntry{
			{Name: "dlopen", Category: "dynamic_load"},
		},
	}
	err = fileStore.Save(common.ResolvedPath(testFile), &Record{
		ContentHash:           fileHash,
		NetworkSymbolAnalysis: nsaData,
	})
	require.NoError(t, err)

	// Load and verify
	loaded, err := store.LoadNetworkSymbolAnalysis(testFile, fileHash)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.Equal(t, analyzedAt, loaded.AnalyzedAt)
	assert.True(t, loaded.HasNetworkSymbols)
	require.Len(t, loaded.DetectedSymbols, 1)
	assert.Equal(t, "socket", loaded.DetectedSymbols[0].Name)
	assert.Equal(t, "socket", loaded.DetectedSymbols[0].Category)
	require.Len(t, loaded.DynamicLoadSymbols, 1)
	assert.Equal(t, "dlopen", loaded.DynamicLoadSymbols[0].Name)
	assert.Equal(t, "dynamic_load", loaded.DynamicLoadSymbols[0].Category)
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

	fileHash := "sha256:netnodynload"
	err = fileStore.Save(common.ResolvedPath(testFile), &Record{
		ContentHash: fileHash,
		NetworkSymbolAnalysis: &NetworkSymbolAnalysisData{
			AnalyzedAt:        time.Now().UTC(),
			HasNetworkSymbols: false,
		},
	})
	require.NoError(t, err)

	loaded, err := store.LoadNetworkSymbolAnalysis(testFile, fileHash)
	require.NoError(t, err)
	require.NotNil(t, loaded)

	assert.False(t, loaded.HasNetworkSymbols)
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

	// Save with one hash
	err = fileStore.Save(common.ResolvedPath(testFile), &Record{
		ContentHash: "sha256:originalhash",
		NetworkSymbolAnalysis: &NetworkSymbolAnalysisData{
			AnalyzedAt:        time.Now().UTC(),
			HasNetworkSymbols: true,
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

	// Save record without NetworkSymbolAnalysis
	fileHash := "sha256:abc123"
	err = fileStore.Save(common.ResolvedPath(testFile), &Record{
		ContentHash:           fileHash,
		NetworkSymbolAnalysis: nil,
	})
	require.NoError(t, err)

	loaded, err := store.LoadNetworkSymbolAnalysis(testFile, fileHash)
	assert.ErrorIs(t, err, ErrNoNetworkSymbolAnalysis,
		"should return ErrNoNetworkSymbolAnalysis when analysis is nil")
	assert.Nil(t, loaded)
}

func TestNetworkSymbolStore_LoadNetworkSymbolAnalysis_RecordNotFound(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewNetworkSymbolStore(fileStore)

	loaded, err := store.LoadNetworkSymbolAnalysis("/nonexistent/file.bin", "sha256:anyhash")
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
	resolvedPath := common.ResolvedPath(testFile)
	recordFilePath, err := getter.GetHashFilePath(analysisDir, resolvedPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(recordFilePath), 0o750))

	oldRecord := map[string]interface{}{
		"schema_version": CurrentSchemaVersion - 1,
		"file_path":      testFile,
		"content_hash":   "sha256:abc123",
		"updated_at":     time.Now().UTC(),
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
	assert.NotErrorIs(t, err, ErrNoNetworkSymbolAnalysis,
		"SchemaVersionMismatchError should not be wrapped as ErrNoNetworkSymbolAnalysis")
}
