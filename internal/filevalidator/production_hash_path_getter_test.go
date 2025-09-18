package filevalidator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProductionHashFilePathGetter(t *testing.T) {
	getter := NewProductionHashFilePathGetter()

	assert.NotNil(t, getter)
}

func TestProductionHashFilePathGetter_GetHashFilePath(t *testing.T) {
	getter := NewProductionHashFilePathGetter()
	algorithm := &MockHashAlgorithm{}
	hashDir := "/tmp/hash"

	tests := []struct {
		name        string
		filePath    string
		shouldError bool
	}{
		{
			name:        "simple_absolute_path",
			filePath:    "/home/user/file.txt",
			shouldError: false,
		},
		{
			name:        "root_path",
			filePath:    "/",
			shouldError: false,
		},
		{
			name:        "nested_path",
			filePath:    "/very/deep/nested/directory/structure/file.txt",
			shouldError: false,
		},
		{
			name:        "path_with_special_chars",
			filePath:    "/path/with#hash/and~tilde/chars.txt",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedPath, err := common.NewResolvedPath(tt.filePath)
			require.NoError(t, err)

			result, err := getter.GetHashFilePath(algorithm, hashDir, resolvedPath)

			if tt.shouldError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.True(t, strings.HasPrefix(result, hashDir))

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
		})
	}
}

func TestProductionHashFilePathGetter_GetHashFilePath_ErrorCases(t *testing.T) {
	getter := NewProductionHashFilePathGetter()

	tests := []struct {
		name        string
		algorithm   HashAlgorithm
		hashDir     string
		filePath    string
		expectedErr error
	}{
		{
			name:        "nil_algorithm",
			algorithm:   nil,
			hashDir:     "/tmp/hash",
			filePath:    "/home/user/file.txt",
			expectedErr: ErrNilAlgorithm,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedPath, err := common.NewResolvedPath(tt.filePath)
			require.NoError(t, err)

			result, err := getter.GetHashFilePath(tt.algorithm, tt.hashDir, resolvedPath)

			assert.Error(t, err)
			assert.Empty(t, result)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestProductionHashFilePathGetter_GetHashFilePath_Consistency(t *testing.T) {
	getter := NewProductionHashFilePathGetter()
	algorithm := &MockHashAlgorithm{}
	hashDir := "/tmp/hash"
	filePath := "/home/user/consistent.txt"

	resolvedPath, err := common.NewResolvedPath(filePath)
	require.NoError(t, err)

	// Call the function multiple times with the same input
	results := make([]string, 5)
	for i := range results {
		result, err := getter.GetHashFilePath(algorithm, hashDir, resolvedPath)
		require.NoError(t, err)
		results[i] = result
	}

	// All results should be identical (deterministic behavior)
	for i := 1; i < len(results); i++ {
		assert.Equal(t, results[0], results[i], "Results should be consistent across calls")
	}
}

func TestProductionHashFilePathGetter_GetHashFilePath_DifferentHashDirs(t *testing.T) {
	getter := NewProductionHashFilePathGetter()
	algorithm := &MockHashAlgorithm{}
	filePath := "/home/user/file.txt"

	resolvedPath, err := common.NewResolvedPath(filePath)
	require.NoError(t, err)

	hashDirs := []string{
		"/tmp/hash1",
		"/tmp/hash2",
		"/var/lib/hashes",
		"/home/user/.hashes",
	}

	results := make([]string, len(hashDirs))
	for i, hashDir := range hashDirs {
		result, err := getter.GetHashFilePath(algorithm, hashDir, resolvedPath)
		require.NoError(t, err)
		results[i] = result

		// Verify the result uses the correct hash directory
		assert.True(t, strings.HasPrefix(result, hashDir))
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
