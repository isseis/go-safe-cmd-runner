//go:build test

package shebang_test

import (
	"os"
	"path/filepath"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/shebang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLookPathInEnv_AbsoluteEntry verifies that a command is found when PATH
// contains the absolute directory that holds it.
func TestLookPathInEnv_AbsoluteEntry(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	commontesting.WriteExecutableFile(t, dir, "mycmd", []byte("#!/bin/sh\n"))

	found, err := shebang.LookPathInEnv("mycmd", dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "mycmd"), found)
}

// TestLookPathInEnv_EmptyEntrySkipped verifies that an empty PATH entry (which
// would otherwise expand to "." / cwd) is skipped rather than resolved against
// the current working directory.
func TestLookPathInEnv_EmptyEntrySkipped(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	commontesting.WriteExecutableFile(t, dir, "mycmd", []byte("#!/bin/sh\n"))

	// Place the command in dir but pass an empty-entry PATH pointing elsewhere.
	// If empty entries were expanded to ".", the test would be cwd-dependent.
	// Instead, the entry must be skipped and ErrCommandNotFound returned.
	pathEnv := string(os.PathListSeparator) + dir // ":dir" — first entry is empty
	t.Chdir(dir)                                  // cwd = dir so "." would find mycmd

	// The command is only in dir; if the empty entry were treated as "." it
	// would still be found, but that would be cwd-dependent.  The point of this
	// test is that the empty entry is *skipped* — the command must be found via
	// the absolute dir entry that follows it, not via ".".
	found, err := shebang.LookPathInEnv("mycmd", pathEnv)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "mycmd"), found)
}

// TestLookPathInEnv_RelativeEntrySkipped verifies that a relative PATH entry
// (e.g. "bin" or "./bin") is skipped to avoid cwd-dependent resolution.
func TestLookPathInEnv_RelativeEntrySkipped(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	commontesting.WriteExecutableFile(t, dir, "mycmd", []byte("#!/bin/sh\n"))

	// Construct a PATH where only a relative entry names the dir.
	// We change cwd to dir's parent and use the base name as a relative entry.
	parent := filepath.Dir(dir)
	relEntry := filepath.Base(dir)
	t.Chdir(parent)

	_, err := shebang.LookPathInEnv("mycmd", relEntry)
	assert.ErrorIs(t, err, shebang.ErrCommandNotFound)
}

// TestLookPathInEnv_NotFound verifies ErrCommandNotFound when no PATH entry
// contains the command.
func TestLookPathInEnv_NotFound(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	_, err := shebang.LookPathInEnv("nonexistent_cmd_xyz", dir)
	assert.ErrorIs(t, err, shebang.ErrCommandNotFound)
}

// TestResolveEnvCommand_RelativeCommandRejected verifies that a command name
// containing a path separator (e.g. "./sh") is rejected even if the file
// exists, because the resolution would be cwd-dependent.
func TestResolveEnvCommand_RelativeCommandRejected(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	commontesting.WriteExecutableFile(t, dir, "sh", []byte("#!/bin/sh\n"))
	t.Chdir(dir)

	// "./sh" contains a separator; LookPathInEnv returns it as-is (relative).
	// ResolveEnvCommand must reject it rather than absolutizing via cwd.
	_, err := shebang.ResolveEnvCommand("./sh", dir)
	assert.ErrorIs(t, err, shebang.ErrCommandNotFound)
}
