//go:build test

package fileanalysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyscallAnalysisStore_SaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Create test syscall analysis result
	result := &SyscallAnalysisResult{
		Architecture: "x86_64",
		DetectedSyscalls: []SyscallInfo{
			{
				Number:              41,
				Name:                "socket",
				IsNetwork:           true,
				Location:            0x401000,
				DeterminationMethod: "immediate",
			},
		},
		HasUnknownSyscalls: false,
		Summary: SyscallSummary{
			HasNetworkSyscalls:  true,
			IsHighRisk:          false,
			TotalDetectedEvents: 1,
			NetworkSyscallCount: 1,
		},
	}

	// Save
	fileHash := "sha256:abc123def456"
	err = store.SaveSyscallAnalysis(testFile, fileHash, result)
	require.NoError(t, err)

	// Load with matching hash
	loadedResult, err := store.LoadSyscallAnalysis(testFile, fileHash)
	require.NoError(t, err)
	require.NotNil(t, loadedResult)

	// Verify loaded result
	assert.Equal(t, "x86_64", loadedResult.Architecture)
	assert.Len(t, loadedResult.DetectedSyscalls, 1)
	assert.Equal(t, 41, loadedResult.DetectedSyscalls[0].Number)
	assert.Equal(t, "socket", loadedResult.DetectedSyscalls[0].Name)
	assert.True(t, loadedResult.DetectedSyscalls[0].IsNetwork)
	assert.False(t, loadedResult.HasUnknownSyscalls)
	assert.True(t, loadedResult.Summary.HasNetworkSyscalls)
}

func TestSyscallAnalysisStore_HashMismatch(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Save with one hash
	result := &SyscallAnalysisResult{
		DetectedSyscalls: []SyscallInfo{
			{Number: 41, Name: "socket"},
		},
	}
	err = store.SaveSyscallAnalysis(testFile, "sha256:originalhash", result)
	require.NoError(t, err)

	// Try to load with different hash
	loadedResult, err := store.LoadSyscallAnalysis(testFile, "sha256:differenthash")
	assert.ErrorIs(t, err, ErrHashMismatch, "should return ErrHashMismatch for mismatched hash")
	assert.Nil(t, loadedResult)
}

func TestSyscallAnalysisStore_NoSyscallAnalysis(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Save record without syscall analysis (only content hash)
	err = fileStore.Save(common.ResolvedPath(testFile), &Record{
		ContentHash:     "sha256:abc123",
		SyscallAnalysis: nil,
	})
	require.NoError(t, err)

	// Try to load - should return ErrNoSyscallAnalysis since no syscall analysis
	loadedResult, err := store.LoadSyscallAnalysis(testFile, "sha256:abc123")
	assert.ErrorIs(t, err, ErrNoSyscallAnalysis, "should return ErrNoSyscallAnalysis when syscall analysis is nil")
	assert.Nil(t, loadedResult)
}

func TestSyscallAnalysisStore_RecordNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Try to load non-existent record
	loadedResult, err := store.LoadSyscallAnalysis("/nonexistent/file.bin", "sha256:anyhash")
	assert.ErrorIs(t, err, ErrRecordNotFound, "should return ErrRecordNotFound for non-existent record")
	assert.Nil(t, loadedResult)
}

func TestSyscallAnalysisStore_HighRiskReasons(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Create result with high risk reasons
	result := &SyscallAnalysisResult{
		DetectedSyscalls: []SyscallInfo{
			{
				Number:              -1,
				DeterminationMethod: "unknown:indirect_setting",
				Location:            0x402000,
			},
		},
		HasUnknownSyscalls: true,
		HighRiskReasons: []string{
			"syscall at 0x402000: number could not be determined (unknown:indirect_setting)",
		},
		Summary: SyscallSummary{
			IsHighRisk:          true,
			TotalDetectedEvents: 1,
		},
	}

	// Save and load
	fileHash := "sha256:highrisk123"
	err = store.SaveSyscallAnalysis(testFile, fileHash, result)
	require.NoError(t, err)

	loadedResult, err := store.LoadSyscallAnalysis(testFile, fileHash)
	require.NoError(t, err)
	require.NotNil(t, loadedResult)

	// Verify high risk information
	assert.True(t, loadedResult.HasUnknownSyscalls)
	assert.True(t, loadedResult.Summary.IsHighRisk)
	require.Len(t, loadedResult.HighRiskReasons, 1)
	assert.Contains(t, loadedResult.HighRiskReasons[0], "indirect_setting")
}

func TestSyscallAnalysisStore_UpdatePreservesOtherFields(t *testing.T) {
	tmpDir := t.TempDir()
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// First save some syscall analysis
	firstResult := &SyscallAnalysisResult{
		DetectedSyscalls: []SyscallInfo{
			{Number: 41, Name: "socket"},
		},
	}
	err = store.SaveSyscallAnalysis(testFile, "sha256:hash1", firstResult)
	require.NoError(t, err)

	// Update with new analysis
	secondResult := &SyscallAnalysisResult{
		DetectedSyscalls: []SyscallInfo{
			{Number: 42, Name: "connect"},
		},
	}
	err = store.SaveSyscallAnalysis(testFile, "sha256:hash2", secondResult)
	require.NoError(t, err)

	// Load the record directly to verify
	record, err := fileStore.Load(common.ResolvedPath(testFile))
	require.NoError(t, err)

	// Content hash should be updated
	assert.Equal(t, "sha256:hash2", record.ContentHash)

	// Syscall analysis should be the new one
	require.NotNil(t, record.SyscallAnalysis)
	require.Len(t, record.SyscallAnalysis.DetectedSyscalls, 1)
	assert.Equal(t, 42, record.SyscallAnalysis.DetectedSyscalls[0].Number)
}
