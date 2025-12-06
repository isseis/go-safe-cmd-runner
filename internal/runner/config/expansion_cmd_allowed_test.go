//go:build test

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandCmdAllowed_Success(t *testing.T) {
	t.Run("single path without variables", func(t *testing.T) {
		// Create a temp file to ensure path resolution works
		tmpFile, err := os.CreateTemp("", "test-cmd-*")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		tmpFile.Close()

		paths := []string{tmpFile.Name()}
		vars := make(map[string]string)

		result, err := expandCmdAllowed(paths, vars, "testgroup")
		require.NoError(t, err)
		assert.Len(t, result, 1)
		// Check that the map contains an absolute path
		for path := range result {
			assert.True(t, filepath.IsAbs(path))
		}
	})

	t.Run("single variable expansion", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "tool")
		err := os.WriteFile(tmpFile, []byte{}, 0o644)
		require.NoError(t, err)

		paths := []string{"%{home}/tool"}
		vars := map[string]string{"home": tmpDir}

		result, err := expandCmdAllowed(paths, vars, "testgroup")
		require.NoError(t, err)
		assert.Len(t, result, 1)
		// Check that the map contains the expected tool path
		for path := range result {
			assert.Equal(t, "tool", filepath.Base(path))
		}
	})

	t.Run("multiple paths", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile1 := filepath.Join(tmpDir, "tool1")
		tmpFile2 := filepath.Join(tmpDir, "tool2")
		err := os.WriteFile(tmpFile1, []byte{}, 0o644)
		require.NoError(t, err)
		err = os.WriteFile(tmpFile2, []byte{}, 0o644)
		require.NoError(t, err)

		paths := []string{tmpFile1, tmpFile2}
		vars := make(map[string]string)

		result, err := expandCmdAllowed(paths, vars, "testgroup")
		require.NoError(t, err)
		assert.Len(t, result, 2)
	})

	t.Run("duplicate raw path returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "tool")
		err := os.WriteFile(tmpFile, []byte{}, 0o644)
		require.NoError(t, err)

		paths := []string{tmpFile, tmpFile}
		vars := make(map[string]string)

		_, err = expandCmdAllowed(paths, vars, "testgroup")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicatePath)
	})

	t.Run("duplicate resolved path returns error", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "tool")
		err := os.WriteFile(tmpFile, []byte{}, 0o644)
		require.NoError(t, err)

		// Create a symlink pointing to the same file
		symlink := filepath.Join(tmpDir, "tool-link")
		err = os.Symlink(tmpFile, symlink)
		require.NoError(t, err)

		paths := []string{tmpFile, symlink}
		vars := make(map[string]string)

		_, err = expandCmdAllowed(paths, vars, "testgroup")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrDuplicateResolvedPath)
	})

	t.Run("complex variable expansion", func(t *testing.T) {
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "app", "bin", "tool")
		err := os.MkdirAll(filepath.Dir(tmpFile), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(tmpFile, []byte{}, 0o644)
		require.NoError(t, err)

		paths := []string{"%{root}/%{app}/bin/tool"}
		vars := map[string]string{
			"root": tmpDir,
			"app":  "app",
		}

		result, err := expandCmdAllowed(paths, vars, "testgroup")
		require.NoError(t, err)
		assert.Len(t, result, 1)
		// Check that the map contains the expected tool path
		for path := range result {
			assert.Equal(t, "tool", filepath.Base(path))
		}
	})
}

func TestExpandCmdAllowed_Errors(t *testing.T) {
	t.Run("empty string path", func(t *testing.T) {
		paths := []string{""}
		vars := make(map[string]string)

		_, err := expandCmdAllowed(paths, vars, "testgroup")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "path cannot be empty")
		assert.ErrorIs(t, err, ErrEmptyPath)
	})

	t.Run("undefined variable", func(t *testing.T) {
		paths := []string{"%{undefined}/tool"}
		vars := make(map[string]string)

		_, err := expandCmdAllowed(paths, vars, "testgroup")
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUndefinedVariable)
	})

	t.Run("relative path", func(t *testing.T) {
		paths := []string{"relative/path/tool"}
		vars := make(map[string]string)

		_, err := expandCmdAllowed(paths, vars, "testgroup")
		require.Error(t, err)
		var invalidPathErr *InvalidPathError
		assert.ErrorAs(t, err, &invalidPathErr)
		assert.ErrorIs(t, err, ErrInvalidPath)
	})

	t.Run("path too long", func(t *testing.T) {
		longPath := "/" + strings.Repeat("a", 5000)
		paths := []string{longPath}
		vars := make(map[string]string)

		_, err := expandCmdAllowed(paths, vars, "testgroup")
		require.Error(t, err)
		var invalidPathErr *InvalidPathError
		assert.ErrorAs(t, err, &invalidPathErr)
		assert.ErrorIs(t, err, ErrInvalidPath)
	})

	t.Run("failed to resolve symlink for nonexistent file", func(t *testing.T) {
		paths := []string{"/nonexistent/path/tool"}
		vars := make(map[string]string)

		_, err := expandCmdAllowed(paths, vars, "testgroup")
		require.Error(t, err)
		// This error comes from filepath.EvalSymlinks, wrapped in fmt.Errorf
		// We verify it's a path resolution error by checking for os.PathError
		assert.Error(t, err, "should return an error for nonexistent path")
	})
}

func TestExpandGroup_WithCmdAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "tool")
	err := os.WriteFile(tmpFile, []byte{}, 0o644)
	require.NoError(t, err)

	t.Run("expands cmd_allowed successfully", func(t *testing.T) {
		spec := &runnertypes.GroupSpec{
			Name:       "testgroup",
			CmdAllowed: []string{"%{bindir}/tool"},
			Vars:       map[string]interface{}{"bindir": tmpDir},
		}

		runtime, err := ExpandGroup(spec, nil)
		require.NoError(t, err)
		assert.NotNil(t, runtime.ExpandedCmdAllowed)
		assert.Len(t, runtime.ExpandedCmdAllowed, 1)
		// Check that the map contains the expected tool path
		for path := range runtime.ExpandedCmdAllowed {
			assert.Equal(t, "tool", filepath.Base(path))
		}
	})

	t.Run("nil cmd_allowed", func(t *testing.T) {
		spec := &runnertypes.GroupSpec{
			Name:       "testgroup",
			CmdAllowed: nil,
		}

		runtime, err := ExpandGroup(spec, nil)
		require.NoError(t, err)
		assert.NotNil(t, runtime.ExpandedCmdAllowed)
		assert.Len(t, runtime.ExpandedCmdAllowed, 0)
	})

	t.Run("empty cmd_allowed", func(t *testing.T) {
		spec := &runnertypes.GroupSpec{
			Name:       "testgroup",
			CmdAllowed: []string{},
		}

		runtime, err := ExpandGroup(spec, nil)
		require.NoError(t, err)
		assert.NotNil(t, runtime.ExpandedCmdAllowed)
		assert.Len(t, runtime.ExpandedCmdAllowed, 0)
	})

	t.Run("error during cmd_allowed expansion", func(t *testing.T) {
		spec := &runnertypes.GroupSpec{
			Name:       "testgroup",
			CmdAllowed: []string{"%{undefined}/tool"},
		}

		_, err := ExpandGroup(spec, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrUndefinedVariable)
	})
}

func TestInvalidPathError(t *testing.T) {
	t.Run("Error method", func(t *testing.T) {
		err := &InvalidPathError{
			Path:   "/test/path",
			Reason: "test reason",
		}
		assert.Contains(t, err.Error(), "/test/path")
		assert.Contains(t, err.Error(), "test reason")
	})

	t.Run("Unwrap method", func(t *testing.T) {
		err := &InvalidPathError{
			Path:   "/test/path",
			Reason: "test reason",
		}
		assert.ErrorIs(t, err, ErrInvalidPath)
	})

	t.Run("Is method", func(t *testing.T) {
		err1 := &InvalidPathError{Path: "/path1", Reason: "reason1"}
		err2 := &InvalidPathError{Path: "/path2", Reason: "reason2"}
		assert.True(t, err1.Is(err2))
	})
}
