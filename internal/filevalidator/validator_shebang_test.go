//go:build test

package filevalidator

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSaveRecord_ShebangDirect verifies that SaveRecord on a "#!/bin/sh" script
// records shebang_chain entries and creates an independent record for
// the interpreter.
//
// Prerequisite: /bin/sh must exist. This is true on all Linux systems, so no
// explicit t.Skip is needed in practice.
func TestSaveRecord_ShebangDirect(t *testing.T) {
	hashDir := safeTempDir(t)
	scriptDir := safeTempDir(t)

	script := commontesting.WriteExecutableFile(t, scriptDir, "script.sh", []byte("#!/bin/sh\necho hello\n"))
	validator, err := New(&SHA256{}, hashDir, ValidatorConfig{})
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(script, false)
	require.NoError(t, err)

	// Verify the script record has shebang_chain set.
	record, err := validator.LoadRecord(script)
	require.NoError(t, err)
	require.Len(t, record.ShebangChain, 1)

	assert.Equal(t, "/bin/sh", record.ShebangChain[0].Ref)
	expectedInterpreter, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)
	assert.Equal(t, expectedInterpreter, record.ShebangChain[0].Path)

	// Verify the interpreter binary itself is included in deps.
	depPaths := make([]string, 0, len(record.DynLibDeps))
	for _, dep := range record.DynLibDeps {
		depPaths = append(depPaths, dep.Path)
	}
	assert.Contains(t, depPaths, expectedInterpreter, "deps should include the shebang interpreter binary")
}

// TestSaveRecord_ShebangEnv verifies that SaveRecord on a "#!/usr/bin/env sh"
// script records shebang_chain entries and creates independent
// records for both env and the resolved command.
//
// "sh" is used instead of "python3" because python3 is not guaranteed to be
// installed in all CI environments. The record/field structure is identical
// regardless of which command env resolves.
func TestSaveRecord_ShebangEnv(t *testing.T) {
	// Pin PATH so shebang.Parse (record time) resolves "sh" from the same
	// directories as the assertions below; prevents flakiness when the ambient
	// PATH differs between CI and dev environments.
	t.Setenv("PATH", "/usr/bin:/bin")

	hashDir := safeTempDir(t)
	scriptDir := safeTempDir(t)

	script := commontesting.WriteExecutableFile(t, scriptDir, "script.py", []byte("#!/usr/bin/env sh\necho hello\n"))
	validator, err := New(&SHA256{}, hashDir, ValidatorConfig{})
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(script, false)
	require.NoError(t, err)

	// Verify the script record has env and command entries in shebang_chain.
	record, err := validator.LoadRecord(script)
	require.NoError(t, err)
	require.Len(t, record.ShebangChain, 2)

	assert.Equal(t, "/usr/bin/env", record.ShebangChain[0].Ref)
	expectedEnvPath, err := filepath.EvalSymlinks("/usr/bin/env")
	require.NoError(t, err)
	assert.Equal(t, expectedEnvPath, record.ShebangChain[0].Path)
	assert.Equal(t, "sh", record.ShebangChain[1].Ref)

	// Compute expected resolved path from the pinned PATH — deterministic.
	shFound, err := exec.LookPath("sh")
	require.NoError(t, err)
	expectedResolvedPath, err := filepath.EvalSymlinks(shFound)
	require.NoError(t, err)
	assert.Equal(t, expectedResolvedPath, record.ShebangChain[1].Path)

	// Verify both shebang binaries are included in deps.
	depPaths := make([]string, 0, len(record.DynLibDeps))
	for _, dep := range record.DynLibDeps {
		depPaths = append(depPaths, dep.Path)
	}
	assert.Contains(t, depPaths, expectedEnvPath)
	assert.Contains(t, depPaths, record.ShebangChain[1].Path)
}

// TestSaveRecord_ShebangELF verifies that ELF binaries have no shebang_chain entries.
func TestSaveRecord_ShebangELF(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	elfHeader := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00, 0x00}
	path := filepath.Join(dir, "fake_elf")
	require.NoError(t, os.WriteFile(path, elfHeader, 0o755))

	validator, err := New(&SHA256{}, hashDir, ValidatorConfig{})
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(path, false)
	require.NoError(t, err)

	record, err := validator.LoadRecord(path)
	require.NoError(t, err)
	assert.Empty(t, record.ShebangChain)
}

// TestSaveRecord_ShebangText verifies that plain text files (no shebang) have
// no shebang_chain entries.
func TestSaveRecord_ShebangText(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	path := filepath.Join(dir, "script.sh")
	require.NoError(t, os.WriteFile(path, []byte("echo hello\n"), 0o755))

	validator, err := New(&SHA256{}, hashDir, ValidatorConfig{})
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(path, false)
	require.NoError(t, err)

	record, err := validator.LoadRecord(path)
	require.NoError(t, err)
	assert.Empty(t, record.ShebangChain)
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

	validator, err := New(&SHA256{}, hashDir, ValidatorConfig{})
	require.NoError(t, err)

	_, _, err = validator.SaveRecord(script, false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrRecursiveShebang)
}

// TestSaveRecord_InterpreterForce verifies that repeated shebang recording keeps
// the main script record consistent without requiring independent interpreter records.
func TestSaveRecord_InterpreterForce(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	interpPath, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)

	validator, err := New(&SHA256{}, hashDir, ValidatorConfig{})
	require.NoError(t, err)

	// Record script A.
	scriptA := commontesting.WriteExecutableFile(t, dir, "a.sh", []byte("#!/bin/sh\necho A\n"))
	_, _, err = validator.SaveRecord(scriptA, false)
	require.NoError(t, err)

	// Record script B with force=false.
	scriptB := commontesting.WriteExecutableFile(t, dir, "b.sh", []byte("#!/bin/sh\necho B\n"))
	_, _, err = validator.SaveRecord(scriptB, false)
	require.NoError(t, err, "second SaveRecord(force=false) must succeed")

	// Record script B again with force=true.
	_, _, err = validator.SaveRecord(scriptB, true)
	require.NoError(t, err, "SaveRecord(force=true) must succeed")

	// Script record must still contain the interpreter binary in deps after force re-record.
	scriptRecord, err := validator.LoadRecord(scriptB)
	require.NoError(t, err)
	depPaths := make([]string, 0, len(scriptRecord.DynLibDeps))
	for _, dep := range scriptRecord.DynLibDeps {
		depPaths = append(depPaths, dep.Path)
	}
	assert.Contains(t, depPaths, interpPath)
}

// TestSaveRecord_ShebangSymlink verifies that when the script is accessed via
// a symlink, shebang_chain is still populated correctly (the symlink is
// resolved before shebang analysis).
func TestSaveRecord_ShebangSymlink(t *testing.T) {
	hashDir := safeTempDir(t)
	dir := safeTempDir(t)

	script := commontesting.WriteExecutableFile(t, dir, "script.sh", []byte("#!/bin/sh\necho hello\n"))
	symlinkPath := filepath.Join(dir, "link_to_script.sh")
	require.NoError(t, os.Symlink(script, symlinkPath))

	validator, err := New(&SHA256{}, hashDir, ValidatorConfig{})
	require.NoError(t, err)

	// SaveRecord on the symlink — validatePath resolves it to the real path.
	_, _, err = validator.SaveRecord(symlinkPath, false)
	require.NoError(t, err)

	// The record is stored under the resolved (real) path.
	record, err := validator.LoadRecord(script)
	require.NoError(t, err)
	require.Len(t, record.ShebangChain, 1)

	expectedInterpreter, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)
	assert.Equal(t, expectedInterpreter, record.ShebangChain[0].Path)
}
