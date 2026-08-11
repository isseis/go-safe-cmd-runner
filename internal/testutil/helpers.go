//go:build test || performance || integration

// Package tu provides shared helper functions for tests.
package tu

import (
	"os"
	"path/filepath"
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

// WriteExecutableFile writes an executable test file and returns its full path.
func WriteExecutableFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, content, 0o755)) // #nosec G306 -- executable bit is intentional for test scripts
	return path
}
