package filevalidator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHybridHashFilePathGetter(t *testing.T) {
	getter := NewHybridHashFilePathGetter()

	assert.NotNil(t, getter)
	assert.NotNil(t, getter.encoder)
	assert.NotNil(t, getter.fallbackGetter)
}

func TestHybridHashFilePathGetter_GetHashFilePath(t *testing.T) {
	getter := NewHybridHashFilePathGetter()
	hashDirRaw := t.TempDir()
	hashDir, err := common.NewResolvedPath(hashDirRaw)
	require.NoError(t, err)

	// Create a real temp file to use as filePath
	tmpFile, err := os.CreateTemp(hashDirRaw, "testfile-*.txt")
	require.NoError(t, err)
	tmpFile.Close()
	resolvedPath, err := common.NewResolvedPath(tmpFile.Name())
	require.NoError(t, err)

	result, err := getter.GetHashFilePath(hashDir, resolvedPath)

	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(result, hashDirRaw))
	assert.True(t, filepath.IsAbs(result))

	filename := filepath.Base(result)
	assert.NotEmpty(t, filename)
	assert.NotEqual(t, ".", filename)
	assert.NotEqual(t, "/", filename)

	// Normal encoding: no .json extension (path is short)
	assert.False(t, strings.HasSuffix(filename, ".json"), "Short path should use normal encoding without .json extension")
}

func TestHybridHashFilePathGetter_GetHashFilePath_ErrorCases(t *testing.T) {
	getter := NewHybridHashFilePathGetter()

	baseDir := t.TempDir()
	tmpFile, err := os.CreateTemp(baseDir, "testfile-*.txt")
	require.NoError(t, err)
	tmpFile.Close()
	resolvedPath, err := common.NewResolvedPath(tmpFile.Name())
	require.NoError(t, err)

	// Empty hashDir (zero value) should return ErrEmptyHashDir
	result, err := getter.GetHashFilePath(common.ResolvedPath{}, resolvedPath)

	assert.Error(t, err)
	assert.Empty(t, result)
	assert.ErrorIs(t, err, ErrEmptyHashDir)
}

func TestHybridHashFilePathGetter_GetHashFilePath_EncodingFallback(t *testing.T) {
	getter := NewHybridHashFilePathGetter()
	hashDirRaw := t.TempDir()
	hashDir, err := common.NewResolvedPath(hashDirRaw)
	require.NoError(t, err)

	// Short path — normal encoding (no .json extension)
	t.Run("short_path_normal_encoding", func(t *testing.T) {
		tmpFile, err := os.CreateTemp(hashDirRaw, "short-*.txt")
		require.NoError(t, err)
		tmpFile.Close()
		resolvedPath, err := common.NewResolvedPath(tmpFile.Name())
		require.NoError(t, err)

		result, err := getter.GetHashFilePath(hashDir, resolvedPath)
		require.NoError(t, err)

		filename := filepath.Base(result)
		assert.True(t, strings.HasPrefix(filename, "~"), "Normal encoding should start with ~")
		assert.False(t, strings.HasSuffix(filename, ".json"), "Normal encoding should not have .json extension")
	})

	// Very long path — SHA256 fallback (.json extension)
	t.Run("very_long_path_fallback", func(t *testing.T) {
		// Create a filename long enough to exceed MaxFilenameLength (250) when encoded.
		// The encoded form is ~dir~...~<filename>, so a filename of 240 chars is sufficient.
		longName := strings.Repeat("a", 240) + ".txt"
		longFilePath := filepath.Join(hashDirRaw, longName)
		require.NoError(t, os.WriteFile(longFilePath, []byte("x"), 0o600))
		resolvedPath, err := common.NewResolvedPath(longFilePath)
		require.NoError(t, err)

		result, err := getter.GetHashFilePath(hashDir, resolvedPath)
		require.NoError(t, err)

		filename := filepath.Base(result)
		assert.False(t, strings.HasPrefix(filename, "~"), "Fallback encoding should not start with ~")
		assert.True(t, strings.HasSuffix(filename, ".json"), "Fallback encoding should have .json extension")
		assert.True(t, len(filename) < 25, "Fallback encoding should be reasonably short")
		assert.False(t, strings.Contains(filename, ".json.json"), "Should not have double .json extension")
	})
}

func TestHybridHashFilePathGetter_GetHashFilePath_Consistency(t *testing.T) {
	getter := NewHybridHashFilePathGetter()
	hashDirRaw := t.TempDir()
	hashDir, err := common.NewResolvedPath(hashDirRaw)
	require.NoError(t, err)

	tmpFile, err := os.CreateTemp(hashDirRaw, "testfile-*.txt")
	require.NoError(t, err)
	tmpFile.Close()
	resolvedPath, err := common.NewResolvedPath(tmpFile.Name())
	require.NoError(t, err)

	// Call the function multiple times with the same input
	results := make([]string, 5)
	for i := range results {
		result, err := getter.GetHashFilePath(hashDir, resolvedPath)
		require.NoError(t, err)
		results[i] = result
	}

	// All results should be identical (deterministic behavior)
	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0], results[i], "Results should be consistent across calls")
	}
}

func TestHybridHashFilePathGetter_GetHashFilePath_DifferentHashDirs(t *testing.T) {
	getter := NewHybridHashFilePathGetter()

	// Create a single temp file to use as filePath
	baseDir := t.TempDir()
	tmpFile, err := os.CreateTemp(baseDir, "testfile-*.txt")
	require.NoError(t, err)
	tmpFile.Close()
	resolvedPath, err := common.NewResolvedPath(tmpFile.Name())
	require.NoError(t, err)

	// Create multiple hash directories
	hashDirRaws := []string{
		t.TempDir(),
		t.TempDir(),
		t.TempDir(),
		t.TempDir(),
	}

	results := make([]string, len(hashDirRaws))
	for i, rawDir := range hashDirRaws {
		hashDir, err := common.NewResolvedPath(rawDir)
		require.NoError(t, err)

		result, err := getter.GetHashFilePath(hashDir, resolvedPath)
		require.NoError(t, err)
		results[i] = result

		// Verify the result uses the correct hash directory
		assert.True(t, strings.HasPrefix(result, rawDir))
	}

	// All results should have different prefixes but same filename
	filenames := make([]string, len(results))
	for i, result := range results {
		filenames[i] = filepath.Base(result)
	}

	// All filenames should be identical (same file, same encoding)
	for i := 1; i < len(filenames); i++ {
		assert.Equal(t, filenames[0], filenames[i], "Filename should be consistent regardless of hash directory")
	}
}
