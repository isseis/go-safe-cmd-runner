//go:build test

package filevalidator

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSaveRecord_ShebangDirect verifies that SaveRecord on a "#!/bin/sh" script
// records the ShebangInterpreter field and creates an independent record for
// the interpreter.
//
// Prerequisite: /bin/sh must exist. This is true on all Linux systems, so no
// explicit t.Skip is needed in practice.
func TestSaveRecord_ShebangDirect(t *testing.T) {
	hashDir := safeTempDir(t)
	scriptDir := safeTempDir(t)

	script := commontesting.WriteExecutableFile(t, scriptDir, "script.sh", []byte("#!/bin/sh\necho hello\n"))
	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(script, false)
	require.NoError(t, err)

	// Verify the script record has ShebangInterpreter set.
	record, err := validator.LoadRecord(script)
	require.NoError(t, err)
	require.NotNil(t, record.ShebangInterpreter)

	expectedInterpreter, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)
	assert.Equal(t, expectedInterpreter, record.ShebangInterpreter.InterpreterPath)
	assert.Empty(t, record.ShebangInterpreter.CommandName)
	assert.Empty(t, record.ShebangInterpreter.ResolvedPath)

	// Verify the interpreter also has its own independent record.
	interpRecord, err := validator.LoadRecord(expectedInterpreter)
	require.NoError(t, err)
	assert.Equal(t, expectedInterpreter, interpRecord.FilePath)
}

// TestSaveRecord_ShebangEnv verifies that SaveRecord on a "#!/usr/bin/env sh"
// script records all three ShebangInterpreter fields and creates independent
// records for both env and the resolved command.
//
// "sh" is used instead of "python3" because python3 is not guaranteed to be
// installed in all CI environments. The record/field structure is identical
// regardless of which command env resolves.
func TestSaveRecord_ShebangEnv(t *testing.T) {
	hashDir := safeTempDir(t)
	scriptDir := safeTempDir(t)

	script := commontesting.WriteExecutableFile(t, scriptDir, "script.py", []byte("#!/usr/bin/env sh\necho hello\n"))
	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(script, false)
	require.NoError(t, err)

	// Verify the script record has all three ShebangInterpreter fields.
	record, err := validator.LoadRecord(script)
	require.NoError(t, err)
	require.NotNil(t, record.ShebangInterpreter)

	expectedEnvPath, err := filepath.EvalSymlinks("/usr/bin/env")
	require.NoError(t, err)
	assert.Equal(t, expectedEnvPath, record.ShebangInterpreter.InterpreterPath)
	assert.Equal(t, "sh", record.ShebangInterpreter.CommandName)
	assert.NotEmpty(t, record.ShebangInterpreter.ResolvedPath)

	// Verify env has its own record.
	envRecord, err := validator.LoadRecord(expectedEnvPath)
	require.NoError(t, err)
	assert.Equal(t, expectedEnvPath, envRecord.FilePath)

	// Verify the resolved command has its own record.
	resolvedRecord, err := validator.LoadRecord(record.ShebangInterpreter.ResolvedPath)
	require.NoError(t, err)
	assert.Equal(t, record.ShebangInterpreter.ResolvedPath, resolvedRecord.FilePath)
}

// TestSaveRecord_ShebangELF verifies that ELF binaries have nil ShebangInterpreter.
func TestSaveRecord_ShebangELF(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	elfHeader := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00, 0x00}
	path := filepath.Join(dir, "fake_elf")
	require.NoError(t, os.WriteFile(path, elfHeader, 0o755))

	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(path, false)
	require.NoError(t, err)

	record, err := validator.LoadRecord(path)
	require.NoError(t, err)
	assert.Nil(t, record.ShebangInterpreter)
}

// TestSaveRecord_ShebangText verifies that plain text files (no shebang) have
// nil ShebangInterpreter.
func TestSaveRecord_ShebangText(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	path := filepath.Join(dir, "script.sh")
	require.NoError(t, os.WriteFile(path, []byte("echo hello\n"), 0o755))

	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(path, false)
	require.NoError(t, err)

	record, err := validator.LoadRecord(path)
	require.NoError(t, err)
	assert.Nil(t, record.ShebangInterpreter)
}

// TestSaveRecord_ShebangRecursive verifies that SaveRecord returns
// ErrRecursiveShebang when the interpreter itself is a shebang script.
func TestSaveRecord_ShebangRecursive(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	// Create a fake interpreter that is itself a shebang script.
	fakeInterp := commontesting.WriteExecutableFile(t, dir, "fake_interpreter", []byte("#!/bin/sh\necho wrapper\n"))

	// Create a script pointing to the fake interpreter.
	script := commontesting.WriteExecutableFile(t, dir, "script.sh",
		[]byte(fmt.Sprintf("#!%s\necho hello\n", fakeInterp)))

	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(script, false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrRecursiveShebang)
}

// TestSaveRecord_ShebangSymlink verifies that when the script is accessed via
// a symlink, the ShebangInterpreter is still populated correctly (the symlink is
// resolved before shebang analysis).
func TestSaveRecord_ShebangSymlink(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	script := commontesting.WriteExecutableFile(t, dir, "script.sh", []byte("#!/bin/sh\necho hello\n"))
	symlinkPath := filepath.Join(dir, "link_to_script.sh")
	require.NoError(t, os.Symlink(script, symlinkPath))

	validator, err := New(&SHA256{}, hashDir)
	require.NoError(t, err)

	// SaveRecord on the symlink — validatePath resolves it to the real path.
	_, _, err = validator.SaveRecord(symlinkPath, false)
	require.NoError(t, err)

	// The record is stored under the resolved (real) path.
	record, err := validator.LoadRecord(script)
	require.NoError(t, err)
	require.NotNil(t, record.ShebangInterpreter)

	expectedInterpreter, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)
	assert.Equal(t, expectedInterpreter, record.ShebangInterpreter.InterpreterPath)
}
