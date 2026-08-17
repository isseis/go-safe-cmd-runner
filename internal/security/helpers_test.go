//go:build test

package security

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// skipIfRoot skips a test that relies on a permission denial, because mode 0o000
// does not produce EACCES for root.
func skipIfRoot(t *testing.T) {
	t.Helper()
	if syscall.Geteuid() == 0 {
		t.Skip("running as root: permission denial cannot be reproduced")
	}
}

// blockedPathUnder creates dir/<name>/inner, removes all access to dir/<name> and
// returns a path below it that therefore cannot be stat'ed.
func blockedPathUnder(t *testing.T, dir, name string) string {
	t.Helper()
	blocked := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Join(blocked, "inner"), 0o700))
	require.NoError(t, os.Chmod(blocked, 0o000))
	// Restore access so t.TempDir cleanup can remove the tree.
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
	return filepath.Join(blocked, "inner", "target")
}

// newBufferLogger returns a logger writing to the returned buffer.
func newBufferLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}
