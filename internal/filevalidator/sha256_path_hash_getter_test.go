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

func TestNewSHA256PathHashGetter(t *testing.T) {
	getter := NewSHA256PathHashGetter()

	assert.NotNil(t, getter)
}

func TestSHA256PathHashGetter_GetHashFilePath(t *testing.T) {
	getter := NewSHA256PathHashGetter()
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
	assert.True(t, strings.HasPrefix(result, hashDir.String()))

	// Verify the result is a valid file path
	assert.True(t, filepath.IsAbs(result))

	// Extract filename and verify it's not empty
	filename := filepath.Base(result)
	assert.NotEmpty(t, filename)
	assert.NotEqual(t, ".", filename)
	assert.NotEqual(t, "/", filename)

	// Production implementation always adds .json extension
	assert.True(t, strings.HasSuffix(filename, ".json"), "Production implementation should always have .json extension")

	// Verify filename format (12 chars + .json = 17 chars total)
	assert.Equal(t, 17, len(filename), "Production filename should be exactly 17 characters (12 hash + 5 for .json)")
}

func TestSHA256PathHashGetter_GetHashFilePath_ErrorCases(t *testing.T) {
	getter := NewSHA256PathHashGetter()

	hashDirRaw := t.TempDir()
	tmpFile, err := os.CreateTemp(hashDirRaw, "testfile-*.txt")
	require.NoError(t, err)
	tmpFile.Close()
	resolvedPath, err := common.NewResolvedPath(tmpFile.Name())
	require.NoError(t, err)

	// Empty hashDir (zero value) should return an error
	result, err := getter.GetHashFilePath(common.ResolvedPath{}, resolvedPath)

	assert.Error(t, err)
	assert.Empty(t, result)
}

func TestSHA256PathHashGetter_GetHashFilePath_Consistency(t *testing.T) {
	getter := NewSHA256PathHashGetter()
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

func TestSHA256PathHashGetter_GetHashFilePath_DifferentHashDirs(t *testing.T) {
	getter := NewSHA256PathHashGetter()

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
		assert.True(t, strings.HasPrefix(result, hashDir.String()))
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
