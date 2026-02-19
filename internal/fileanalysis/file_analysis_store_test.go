//go:build test

package fileanalysis

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockPathGetter is a simple test implementation of HashFilePathGetter
type mockPathGetter struct{}

func (m *mockPathGetter) GetHashFilePath(hashDir string, filePath common.ResolvedPath) (string, error) {
	// Use a simple hash of the path for testing
	return filepath.Join(hashDir, filepath.Base(filePath.String())+".json"), nil
}

func TestStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Save record
	originalRecord := &Record{
		ContentHash: "sha256:abc123",
	}
	err = store.Save(common.ResolvedPath(testFile), originalRecord)
	require.NoError(t, err)

	// Load record
	loadedRecord, err := store.Load(common.ResolvedPath(testFile))
	require.NoError(t, err)

	// Verify fields
	assert.Equal(t, CurrentSchemaVersion, loadedRecord.SchemaVersion)
	assert.Equal(t, testFile, loadedRecord.FilePath)
	assert.Equal(t, "sha256:abc123", loadedRecord.ContentHash)
	assert.False(t, loadedRecord.UpdatedAt.IsZero())
}

func TestStore_RecordNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	// Try to load non-existent record
	_, err = store.Load(common.ResolvedPath("/nonexistent/file.bin"))
	assert.ErrorIs(t, err, ErrRecordNotFound)
}

func TestStore_SchemaVersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Manually write record with wrong schema version
	recordPath := filepath.Join(analysisDir, "test.bin.json")
	wrongVersionRecord := map[string]interface{}{
		"schema_version": 999,
		"file_path":      testFile,
		"content_hash":   "sha256:abc123",
		"updated_at":     time.Now().UTC(),
	}
	data, err := json.MarshalIndent(wrongVersionRecord, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(recordPath, data, 0o600)
	require.NoError(t, err)

	// Try to load - should get schema version mismatch error
	_, err = store.Load(common.ResolvedPath(testFile))
	var schemaErr *SchemaVersionMismatchError
	assert.ErrorAs(t, err, &schemaErr)
	assert.Equal(t, CurrentSchemaVersion, schemaErr.Expected)
	assert.Equal(t, 999, schemaErr.Actual)
}

func TestStore_CorruptedRecord(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Manually write corrupted JSON
	recordPath := filepath.Join(analysisDir, "test.bin.json")
	err = os.WriteFile(recordPath, []byte("not valid json {{{"), 0o600)
	require.NoError(t, err)

	// Try to load - should get corrupted error
	_, err = store.Load(common.ResolvedPath(testFile))
	var corruptedErr *RecordCorruptedError
	assert.ErrorAs(t, err, &corruptedErr)
}

func TestStore_PreservesExistingFields(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Save record with syscall analysis
	originalRecord := &Record{
		ContentHash: "sha256:abc123",
		SyscallAnalysis: &SyscallAnalysisData{
			Architecture:       "x86_64",
			AnalyzedAt:         time.Now().UTC(),
			HasUnknownSyscalls: true,
			HighRiskReasons:    []string{"reason1"},
		},
	}
	err = store.Save(common.ResolvedPath(testFile), originalRecord)
	require.NoError(t, err)

	// Update only the content hash
	err = store.Update(common.ResolvedPath(testFile), func(record *Record) error {
		record.ContentHash = "sha256:def456"
		return nil
	})
	require.NoError(t, err)

	// Load and verify syscall analysis is preserved
	loadedRecord, err := store.Load(common.ResolvedPath(testFile))
	require.NoError(t, err)
	assert.Equal(t, "sha256:def456", loadedRecord.ContentHash)
	assert.NotNil(t, loadedRecord.SyscallAnalysis)
	assert.Equal(t, "x86_64", loadedRecord.SyscallAnalysis.Architecture)
	assert.True(t, loadedRecord.SyscallAnalysis.HasUnknownSyscalls)
}

func TestStore_Update_CreatesNewRecord(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Update non-existent record - should create new
	err = store.Update(common.ResolvedPath(testFile), func(record *Record) error {
		record.ContentHash = "sha256:newrecord"
		return nil
	})
	require.NoError(t, err)

	// Load and verify
	loadedRecord, err := store.Load(common.ResolvedPath(testFile))
	require.NoError(t, err)
	assert.Equal(t, "sha256:newrecord", loadedRecord.ContentHash)
}

func TestStore_Update_SchemaVersionMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Manually write record with wrong schema version
	recordPath := filepath.Join(analysisDir, "test.bin.json")
	wrongVersionRecord := map[string]interface{}{
		"schema_version": 999,
		"file_path":      testFile,
		"content_hash":   "sha256:oldvalue",
		"updated_at":     time.Now().UTC(),
	}
	data, err := json.MarshalIndent(wrongVersionRecord, "", "  ")
	require.NoError(t, err)
	err = os.WriteFile(recordPath, data, 0o600)
	require.NoError(t, err)

	// Update should fail due to schema version mismatch
	err = store.Update(common.ResolvedPath(testFile), func(record *Record) error {
		record.ContentHash = "sha256:newvalue"
		return nil
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "schema version mismatch")

	// Original record should be preserved
	data, err = os.ReadFile(recordPath)
	require.NoError(t, err)
	var preserved map[string]interface{}
	err = json.Unmarshal(data, &preserved)
	require.NoError(t, err)
	assert.Equal(t, float64(999), preserved["schema_version"])
	assert.Equal(t, "sha256:oldvalue", preserved["content_hash"])
}

func TestStore_Update_CorruptedRecord(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Manually write corrupted JSON
	recordPath := filepath.Join(analysisDir, "test.bin.json")
	err = os.WriteFile(recordPath, []byte("not valid json {{{"), 0o600)
	require.NoError(t, err)

	// Update should succeed by creating fresh record
	err = store.Update(common.ResolvedPath(testFile), func(record *Record) error {
		record.ContentHash = "sha256:fresh"
		return nil
	})
	require.NoError(t, err)

	// Load and verify new record
	loadedRecord, err := store.Load(common.ResolvedPath(testFile))
	require.NoError(t, err)
	assert.Equal(t, "sha256:fresh", loadedRecord.ContentHash)
}

func TestNewStore_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "new", "nested", "dir")

	// Directory should not exist yet
	_, err := os.Stat(analysisDir)
	assert.True(t, os.IsNotExist(err))

	// Create store - should auto-create directory
	store, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)
	assert.NotNil(t, store)

	// Directory should now exist
	info, err := os.Stat(analysisDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestNewStore_ExistingDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Use existing temp directory
	store, err := NewStore(tmpDir, &mockPathGetter{})
	require.NoError(t, err)
	assert.NotNil(t, store)
}

func TestNewStore_NotADirectory(t *testing.T) {
	tmpDir := t.TempDir()
	notADir := filepath.Join(tmpDir, "file.txt")

	// Create a file where we expect a directory
	err := os.WriteFile(notADir, []byte("not a directory"), 0o644)
	require.NoError(t, err)

	// Should fail because path is not a directory
	_, err = NewStore(notADir, &mockPathGetter{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}
