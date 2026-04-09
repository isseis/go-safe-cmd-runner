//go:build test

package fileanalysis

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyscallAnalysisStore_SaveAndLoad(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
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
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
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
}

func TestSyscallAnalysisStore_HashMismatch(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
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
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []SyscallInfo{
				{Number: 41, Name: "socket"},
			},
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
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	rp, err := common.NewResolvedPath(testFile)
	require.NoError(t, err)

	// Save record without syscall analysis (only content hash)
	err = fileStore.Save(rp, &Record{
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
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Create a real file with no record saved — should return ErrRecordNotFound
	noRecordFile := filepath.Join(tmpDir, "no-record.bin")
	require.NoError(t, os.WriteFile(noRecordFile, []byte("content"), 0o644))
	loadedResult, err := store.LoadSyscallAnalysis(noRecordFile, "sha256:anyhash")
	assert.ErrorIs(t, err, ErrRecordNotFound, "should return ErrRecordNotFound for non-existent record")
	assert.Nil(t, loadedResult)
}

func TestSyscallAnalysisStore_AnalysisWarnings(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Create result with analysis warnings
	result := &SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []SyscallInfo{
				{
					Number:              -1,
					DeterminationMethod: "unknown:indirect_setting",
					Location:            0x402000,
				},
			},
			AnalysisWarnings: []string{
				"syscall at 0x402000: number could not be determined (unknown:indirect_setting)",
			},
		},
	}

	// Save and load
	fileHash := "sha256:highrisk123"
	err = store.SaveSyscallAnalysis(testFile, fileHash, result)
	require.NoError(t, err)

	loadedResult, err := store.LoadSyscallAnalysis(testFile, fileHash)
	require.NoError(t, err)
	require.NotNil(t, loadedResult)

	// Verify analysis warnings are preserved
	require.Len(t, loadedResult.AnalysisWarnings, 1)
	assert.Contains(t, loadedResult.AnalysisWarnings[0], "indirect_setting")
}

func TestSyscallAnalysisStore_SaveSortsDetectedSyscallsByNumber(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	// Provide syscalls in unsorted order (mimicking Pass1+Pass2 address ordering).
	result := &SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []SyscallInfo{
				{Number: 257, Name: "openat"},
				{Number: 41, Name: "socket"},
				{Number: 1, Name: "write"},
				{Number: 42, Name: "connect"},
			},
		},
	}

	fileHash := "sha256:sorttest"
	err = store.SaveSyscallAnalysis(testFile, fileHash, result)
	require.NoError(t, err)

	loadedResult, err := store.LoadSyscallAnalysis(testFile, fileHash)
	require.NoError(t, err)
	require.Len(t, loadedResult.DetectedSyscalls, 4)

	// Verify number-ascending order.
	assert.Equal(t, 1, loadedResult.DetectedSyscalls[0].Number)
	assert.Equal(t, 41, loadedResult.DetectedSyscalls[1].Number)
	assert.Equal(t, 42, loadedResult.DetectedSyscalls[2].Number)
	assert.Equal(t, 257, loadedResult.DetectedSyscalls[3].Number)
}

func TestSyscallAnalysisStore_UpdatePreservesOtherFields(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
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
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []SyscallInfo{
				{Number: 41, Name: "socket"},
			},
		},
	}
	err = store.SaveSyscallAnalysis(testFile, "sha256:hash1", firstResult)
	require.NoError(t, err)

	// Update with new analysis
	secondResult := &SyscallAnalysisResult{
		SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
			DetectedSyscalls: []SyscallInfo{
				{Number: 42, Name: "connect"},
			},
		},
	}
	err = store.SaveSyscallAnalysis(testFile, "sha256:hash2", secondResult)
	require.NoError(t, err)

	// Load the record directly to verify
	rpLoad, err := common.NewResolvedPath(testFile)
	require.NoError(t, err)
	record, err := fileStore.Load(rpLoad)
	require.NoError(t, err)

	// Content hash should be updated
	assert.Equal(t, "sha256:hash2", record.ContentHash)

	// Syscall analysis should be the new one
	require.NotNil(t, record.SyscallAnalysis)
	require.Len(t, record.SyscallAnalysis.DetectedSyscalls, 1)
	assert.Equal(t, 42, record.SyscallAnalysis.DetectedSyscalls[0].Number)
}

func TestFilterSyscallsForStorage(t *testing.T) {
	input := []common.SyscallInfo{
		{Number: 41, Name: "socket", IsNetwork: true, DeterminationMethod: "immediate"},
		{Number: 1, Name: "write", IsNetwork: false, DeterminationMethod: "immediate"},
		{Number: -1, Name: "", IsNetwork: false, DeterminationMethod: "unknown:scan_limit_exceeded"},
		{Number: 42, Name: "connect", IsNetwork: true, DeterminationMethod: "immediate"},
		{Number: 2, Name: "open", IsNetwork: false, DeterminationMethod: "immediate"},
	}

	result := FilterSyscallsForStorage(input)

	// Only network syscalls and unknown syscalls should be retained.
	require.Len(t, result, 3)
	numbers := make([]int, len(result))
	for i, s := range result {
		numbers[i] = s.Number
	}
	assert.Contains(t, numbers, 41)
	assert.Contains(t, numbers, -1)
	assert.Contains(t, numbers, 42)
	assert.NotContains(t, numbers, 1)
	assert.NotContains(t, numbers, 2)
}

func TestFilterSyscallsForStorage_Empty(t *testing.T) {
	result := FilterSyscallsForStorage(nil)
	assert.Empty(t, result)
}

func TestStore_ArgEvalResults(t *testing.T) {
	tmpDir := commontesting.SafeTempDir(t)
	analysisDir := filepath.Join(tmpDir, "analysis")

	fileStore, err := NewStore(analysisDir, &mockPathGetter{})
	require.NoError(t, err)

	store := NewSyscallAnalysisStore(fileStore)

	testFile := filepath.Join(tmpDir, "test.bin")
	err = os.WriteFile(testFile, []byte("test content"), 0o644)
	require.NoError(t, err)

	t.Run("ArgEvalResults roundtrip", func(t *testing.T) {
		result := &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				Architecture: "x86_64",
				ArgEvalResults: []common.SyscallArgEvalResult{
					{
						SyscallName: "mprotect",
						Status:      common.SyscallArgEvalExecConfirmed,
						Details:     "prot=0x7",
					},
				},
			},
		}

		fileHash := "sha256:argevalroundtrip"
		err = store.SaveSyscallAnalysis(testFile, fileHash, result)
		require.NoError(t, err)

		loaded, err := store.LoadSyscallAnalysis(testFile, fileHash)
		require.NoError(t, err)
		require.NotNil(t, loaded)

		require.Len(t, loaded.ArgEvalResults, 1)
		assert.Equal(t, "mprotect", loaded.ArgEvalResults[0].SyscallName)
		assert.Equal(t, common.SyscallArgEvalExecConfirmed, loaded.ArgEvalResults[0].Status)
		assert.Equal(t, "prot=0x7", loaded.ArgEvalResults[0].Details)
	})

	t.Run("nil ArgEvalResults is omitted from JSON", func(t *testing.T) {
		testFile2 := filepath.Join(tmpDir, "test2.bin")
		err = os.WriteFile(testFile2, []byte("test content 2"), 0o644)
		require.NoError(t, err)

		result := &SyscallAnalysisResult{
			SyscallAnalysisResultCore: common.SyscallAnalysisResultCore{
				Architecture:   "x86_64",
				ArgEvalResults: nil,
			},
		}

		fileHash := "sha256:argevalnilomit"
		err = store.SaveSyscallAnalysis(testFile2, fileHash, result)
		require.NoError(t, err)

		loaded, err := store.LoadSyscallAnalysis(testFile2, fileHash)
		require.NoError(t, err)
		require.NotNil(t, loaded)

		assert.Nil(t, loaded.ArgEvalResults, "nil ArgEvalResults should remain nil after roundtrip")
	})
}
