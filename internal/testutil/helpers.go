//go:build test || performance || integration

// Package tu provides shared helper functions for tests.
package tu

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

// StringPtrOrNil returns nil for an empty string, otherwise a pointer to s.
func StringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// SafeTempDir returns a temporary directory path with symlinks resolved.
func SafeTempDir(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	realPath, err := filepath.EvalSymlinks(tempDir)
	require.NoError(t, err, "Failed to resolve symlinks in temp dir")
	return realPath
}

// WalkEntries returns the sorted relative paths and modes of every entry under
// root, so two calls can be compared to prove no filesystem entry was added,
// removed, or had its mode changed. WalkDir's DirEntry is lstat-derived, so a
// symlink is reported as a symlink rather than as its target, and FileMode
// already encodes the entry type alongside the permission bits.
//
// Note that the snapshot covers the shape of the subtree, not file contents: a
// write into an existing file leaves it unchanged.
func WalkEntries(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		entries = append(entries, rel+"/"+info.Mode().String())
		return nil
	})
	require.NoError(t, err, "failed to walk subtree at %s", root)
	slices.Sort(entries)
	return entries
}

// WriteExecutableFile writes an executable test file and returns its full path.
func WriteExecutableFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, content, 0o755)) // #nosec G306 -- executable bit is intentional for test scripts
	return path
}
