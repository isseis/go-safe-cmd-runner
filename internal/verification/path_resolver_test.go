package verification

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathResolver_ResolvePath(t *testing.T) {
	// Create a temporary directory for our test
	tempDir := t.TempDir()

	// Create test directories in PATH
	dir1 := filepath.Join(tempDir, "dir1")
	dir2 := filepath.Join(tempDir, "dir2")
	err := os.MkdirAll(dir1, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(dir2, 0o755)
	require.NoError(t, err)

	// Create test files:
	// - dir1/testcmd (directory, not executable)
	// - dir2/testcmd (regular file, executable)
	err = os.Mkdir(filepath.Join(dir1, "testcmd"), 0o755)
	require.NoError(t, err)

	execPath := filepath.Join(dir2, "testcmd")
	err = os.WriteFile(execPath, []byte("#!/bin/sh\necho hello\n"), 0o755)
	require.NoError(t, err)

	// Set up PATH with both directories
	// Create a path with dir1 first, then dir2
	testPath := dir1 + string(os.PathListSeparator) + dir2
	t.Setenv("PATH", testPath)

	// Create a new PathResolver with our test PATH
	resolver := NewPathResolver(testPath, nil, false)

	t.Run("finds executable in second PATH directory when first is a directory", func(t *testing.T) {
		resolved, err := resolver.ResolvePath("testcmd")
		require.NoError(t, err)
		assert.Equal(t, execPath, resolved)
	})

	t.Run("returns error when command not found in PATH", func(t *testing.T) {
		_, err := resolver.ResolvePath("nonexistent-command")
		assert.ErrorIs(t, err, ErrCommandNotFound)
	})

	t.Run("returns error when command is a directory in all PATH entries", func(t *testing.T) {
		// Create a directory with the same name in both PATH directories
		err = os.Mkdir(filepath.Join(dir2, "testdir"), 0o755)
		require.NoError(t, err)

		_, err = resolver.ResolvePath("testdir")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrCommandNotFound)
	})
}

// Additional tests for PathResolver methods
func TestPathResolver_ShouldSkipVerification(t *testing.T) {
	t.Run("skip_standard_paths_enabled", func(t *testing.T) {
		resolver := NewPathResolver("/usr/bin:/bin", nil, true)

		// Test various standard paths that should be skipped
		testCases := []string{
			"/usr/bin/ls",
			"/bin/sh",
			"/usr/sbin/init",
			"/sbin/mount",
		}

		for _, path := range testCases {
			shouldSkip := resolver.ShouldSkipVerification(path)
			assert.True(t, shouldSkip, "Path %s should be skipped when skipStandardPaths is enabled", path)
		}
	})

	t.Run("skip_standard_paths_disabled", func(t *testing.T) {
		resolver := NewPathResolver("/usr/bin:/bin", nil, false)

		// Test that standard paths are not skipped
		testCases := []string{
			"/usr/bin/ls",
			"/bin/sh",
			"/usr/sbin/init",
		}

		for _, path := range testCases {
			shouldSkip := resolver.ShouldSkipVerification(path)
			assert.False(t, shouldSkip, "Path %s should not be skipped when skipStandardPaths is disabled", path)
		}
	})

	t.Run("non_standard_paths", func(t *testing.T) {
		resolver := NewPathResolver("/usr/bin:/bin", nil, true)

		// Test non-standard paths (should never be skipped regardless of setting)
		testCases := []string{
			"/home/user/custom/command",
			"/opt/app/bin/tool",
			"/tmp/temp_command",
		}

		for _, path := range testCases {
			shouldSkip := resolver.ShouldSkipVerification(path)
			assert.False(t, shouldSkip, "Non-standard path %s should not be skipped", path)
		}
	})
}

func TestPathResolver_CanAccessDirectory(t *testing.T) {
	resolver := NewPathResolver("/usr/bin:/bin", nil, false)

	t.Run("accessible_directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		canAccess := resolver.canAccessDirectory(tmpDir)
		assert.True(t, canAccess)
	})

	t.Run("non_existent_directory", func(t *testing.T) {
		canAccess := resolver.canAccessDirectory("/non/existent/directory")
		assert.False(t, canAccess)
	})

	t.Run("non_directory_path", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test_file")
		err := os.WriteFile(filePath, []byte("test"), 0o644)
		require.NoError(t, err)

		canAccess := resolver.canAccessDirectory(filePath)
		assert.False(t, canAccess)
	})
}

func TestPathResolver_NoCommandValidation(t *testing.T) {
	// This test documents that ResolvePath intentionally does NOT perform
	// command allowlist validation. Validation is the caller's responsibility.

	t.Run("ResolvePath succeeds regardless of allowlist", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create an executable file
		execPath := filepath.Join(tempDir, "test_command")
		err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755)
		require.NoError(t, err)

		// Create PathResolver with a security validator that would reject this command
		// (if validation were performed, which it should NOT be)
		testPath := tempDir
		resolver := NewPathResolver(testPath, nil, false)

		// ResolvePath should succeed - it only resolves paths, doesn't validate
		resolved, err := resolver.ResolvePath(execPath)
		require.NoError(t, err)
		assert.Equal(t, execPath, resolved)

		// Note: Command allowlist validation is performed by the caller
		// (GroupExecutor) using security.ValidateCommandAllowed(), which
		// checks both global patterns and group-level cmd_allowed.
	})
}

func TestPathResolver_ValidateAndCacheCommand(t *testing.T) {
	t.Run("successful_validation_and_caching", func(t *testing.T) {
		tempDir := t.TempDir()
		execPath := filepath.Join(tempDir, "test_cmd")
		err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755)
		require.NoError(t, err)

		resolver := NewPathResolver(tempDir, nil, false)

		path, err := resolver.validateAndCacheCommand(execPath, "test_cmd")

		assert.NoError(t, err)
		assert.Equal(t, execPath, path)

		// Test that subsequent calls use cache
		path2, err2 := resolver.validateAndCacheCommand(execPath, "test_cmd")
		assert.NoError(t, err2)
		assert.Equal(t, path, path2)
	})

	t.Run("command_validation_failure", func(t *testing.T) {
		resolver := NewPathResolver("/nonexistent", nil, false)

		path, err := resolver.validateAndCacheCommand("/nonexistent/command", "nonexistent_command")

		assert.Error(t, err)
		assert.Empty(t, path)
		assert.ErrorIs(t, err, ErrCommandNotFound)
	})

	t.Run("directory_instead_of_command", func(t *testing.T) {
		tempDir := t.TempDir()
		dirPath := filepath.Join(tempDir, "test_dir")
		err := os.MkdirAll(dirPath, 0o755)
		require.NoError(t, err)

		resolver := NewPathResolver(tempDir, nil, false)

		path, err := resolver.validateAndCacheCommand(dirPath, "test_dir")

		assert.Error(t, err)
		assert.Empty(t, path)
		assert.ErrorIs(t, err, ErrCommandNotFound)
		assert.Contains(t, err.Error(), "is a directory")
	})
}
