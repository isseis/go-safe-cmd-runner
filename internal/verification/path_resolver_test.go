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
	tempDir, err := os.MkdirTemp("", "path-resolver-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test directories in PATH
	dir1 := filepath.Join(tempDir, "dir1")
	dir2 := filepath.Join(tempDir, "dir2")
	err = os.MkdirAll(dir1, 0o755)
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
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)

	// Create a path with dir1 first, then dir2
	testPath := dir1 + string(os.PathListSeparator) + dir2
	os.Setenv("PATH", testPath)

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
		assert.Contains(t, err.Error(), "testdir is a directory")
		assert.ErrorIs(t, err, ErrCommandNotFound)
	})
}
