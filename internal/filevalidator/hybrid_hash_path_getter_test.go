package filevalidator

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testHashDir = "/tmp/hash"

func TestNewHybridHashFilePathGetter(t *testing.T) {
	getter := NewHybridHashFilePathGetter()

	assert.NotNil(t, getter)
	assert.NotNil(t, getter.encoder)
	assert.NotNil(t, getter.fallbackGetter)
}

func TestHybridHashFilePathGetter_GetHashFilePath(t *testing.T) {
	getter := NewHybridHashFilePathGetter()
	hashDir := testHashDir

	tests := []struct {
		name            string
		filePath        string
		expectExtension bool
		shouldError     bool
	}{
		{
			name:            "simple_absolute_path",
			filePath:        "/home/user/file.txt",
			expectExtension: false, // Normal encoding has no extension
			shouldError:     false,
		},
		{
			name:            "root_path",
			filePath:        "/",
			expectExtension: false, // Normal encoding has no extension
			shouldError:     false,
		},
		{
			name:            "nested_path",
			filePath:        "/very/deep/nested/directory/structure/file.txt",
			expectExtension: false, // Normal encoding has no extension
			shouldError:     false,
		},
		{
			name:            "path_with_special_chars",
			filePath:        "/path/with#hash/and~tilde/chars.txt",
			expectExtension: false, // Normal encoding has no extension
			shouldError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedPath, err := common.NewResolvedPath(tt.filePath)
			require.NoError(t, err)

			result, err := getter.GetHashFilePath(hashDir, resolvedPath)

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

			// Check extension expectation
			if tt.expectExtension {
				assert.True(t, strings.HasSuffix(filename, ".json"), "Should have .json extension")
			} else {
				assert.False(t, strings.HasSuffix(filename, ".json"), "Should not have .json extension")
			}
		})
	}
}

func TestHybridHashFilePathGetter_GetHashFilePath_ErrorCases(t *testing.T) {
	getter := NewHybridHashFilePathGetter()

	tests := []struct {
		name        string
		hashDir     string
		filePath    string
		expectedErr error
	}{
		{
			name:        "empty_hash_directory",
			hashDir:     "",
			filePath:    "/home/user/file.txt",
			expectedErr: ErrEmptyHashDir,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedPath, err := common.NewResolvedPath(tt.filePath)
			require.NoError(t, err)

			result, err := getter.GetHashFilePath(tt.hashDir, resolvedPath)

			assert.Error(t, err)
			assert.Empty(t, result)
			assert.ErrorIs(t, err, tt.expectedErr)
		})
	}
}

func TestHybridHashFilePathGetter_GetHashFilePath_EncodingFallback(t *testing.T) {
	getter := NewHybridHashFilePathGetter()
	hashDir := testHashDir

	tests := []struct {
		name             string
		filePath         string
		expectNormalPath bool
		expectFallback   bool
	}{
		{
			name:             "short_path_normal_encoding",
			filePath:         "/home/user/file.txt",
			expectNormalPath: true,
			expectFallback:   false,
		},
		{
			name:             "very_long_path_fallback",
			filePath:         "/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/very/long/path/that/exceeds/max/filename/length/limit/and/should/trigger/sha256/fallback/encoding/mechanism/file.txt",
			expectNormalPath: false,
			expectFallback:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolvedPath, err := common.NewResolvedPath(tt.filePath)
			require.NoError(t, err)

			result, err := getter.GetHashFilePath(hashDir, resolvedPath)
			assert.NoError(t, err)

			filename := filepath.Base(result)

			if tt.expectNormalPath {
				// Normal encoding should start with ~ and be based on the path
				assert.True(t, strings.HasPrefix(filename, "~"), "Normal encoding should start with ~")
				// Normal encoding should NOT have .json extension
				assert.False(t, strings.HasSuffix(filename, ".json"), "Normal encoding should not have .json extension")
			}

			if tt.expectFallback {
				// Fallback encoding should be a hash followed by .json
				// Should not start with ~ and should be a short hash
				assert.False(t, strings.HasPrefix(filename, "~"), "Fallback encoding should not start with ~")
				// SHA256 fallback creates filename like "AbCdEf123456.json" (17 chars)
				assert.True(t, len(filename) < 25, "Fallback encoding should be reasonably short")
				// Should contain only alphanumeric chars and .json extension
				withoutExt := strings.TrimSuffix(filename, ".json")
				assert.True(t, len(withoutExt) > 0, "Should have content before .json")
				// Verify no double .json extension
				assert.False(t, strings.Contains(filename, ".json.json"), "Should not have double .json extension")
				// Fallback encoding SHOULD have .json extension
				assert.True(t, strings.HasSuffix(filename, ".json"), "Fallback encoding should have .json extension")
			}
		})
	}
}

func TestHybridHashFilePathGetter_GetHashFilePath_InvalidPath(t *testing.T) {
	getter := NewHybridHashFilePathGetter()
	hashDir := testHashDir

	// Test with various invalid paths that should cause encoding errors
	invalidPaths := []string{
		"relative/path",           // Relative path
		"../path/traversal",       // Path traversal
		"/path/with/../traversal", // Path with traversal
		"/path/with/./current",    // Path with current directory
	}

	for _, invalidPath := range invalidPaths {
		t.Run("invalid_path_"+invalidPath, func(t *testing.T) {
			// These paths should fail at ResolvedPath creation or encoding stage
			resolvedPath, pathErr := common.NewResolvedPath(invalidPath)
			if pathErr != nil {
				// If ResolvedPath creation fails, that's expected
				return
			}

			result, err := getter.GetHashFilePath(hashDir, resolvedPath)

			// Should either fail at ResolvedPath creation or at encoding
			assert.Error(t, err)
			assert.Empty(t, result)
		})
	}
}

func TestHybridHashFilePathGetter_GetHashFilePath_Consistency(t *testing.T) {
	getter := NewHybridHashFilePathGetter()
	hashDir := testHashDir
	filePath := "/home/user/consistent.txt"

	resolvedPath, err := common.NewResolvedPath(filePath)
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
		result, err := getter.GetHashFilePath(hashDir, resolvedPath)
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
