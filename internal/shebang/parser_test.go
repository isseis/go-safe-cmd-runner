//go:build test

package shebang_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/shebang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// realFS returns a production FileSystem for use in tests.
func realFS() safefileio.FileSystem {
	return safefileio.NewFileSystem(safefileio.FileSystemConfig{})
}

// writeScript creates a script file with the given content in a temp dir.
func writeScript(t *testing.T, content string) string {
	t.Helper()
	return commontesting.WriteExecutableFile(t, commontesting.SafeTempDir(t), "test_script", []byte(content))
}

// writeBinaryFile creates a binary file with the given bytes in a temp dir.
func writeBinaryFile(t *testing.T, data []byte) string {
	t.Helper()
	return commontesting.WriteExecutableFile(t, commontesting.SafeTempDir(t), "test_binary", data)
}

// --- Parse tests ---

func TestParse_DirectForm(t *testing.T) {
	path := writeScript(t, "#!/bin/sh\necho hello\n")
	info, err := shebang.Parse(path, realFS())
	require.NoError(t, err)
	require.NotNil(t, info)

	// /bin/sh -> /usr/bin/dash (resolved via EvalSymlinks)
	expected, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)
	assert.Equal(t, expected, info.InterpreterPath)
	assert.Empty(t, info.CommandName)
	assert.Empty(t, info.ResolvedPath)
}

func TestParse_DirectFormWithArgs(t *testing.T) {
	path := writeScript(t, "#!/bin/bash -e\necho hello\n")
	info, err := shebang.Parse(path, realFS())
	require.NoError(t, err)
	require.NotNil(t, info)

	expected, err := filepath.EvalSymlinks("/bin/bash")
	require.NoError(t, err)
	assert.Equal(t, expected, info.InterpreterPath)
	assert.Empty(t, info.CommandName)
	assert.Empty(t, info.ResolvedPath)
}

func TestParse_SpaceAfterShebang(t *testing.T) {
	path := writeScript(t, "#! /bin/sh\necho hello\n")
	info, err := shebang.Parse(path, realFS())
	require.NoError(t, err)
	require.NotNil(t, info)

	expected, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)
	assert.Equal(t, expected, info.InterpreterPath)
}

func TestParse_EnvForm(t *testing.T) {
	// Use "sh" as the env command because it is guaranteed to be present on any
	// Linux system.  "python3" would be a more realistic choice for #!/usr/bin/env
	// scripts, but it is not installed in all CI environments.
	path := writeScript(t, "#!/usr/bin/env sh\necho hello\n")
	info, err := shebang.Parse(path, realFS())
	require.NoError(t, err)
	require.NotNil(t, info)

	// InterpreterPath = EvalSymlinks("/usr/bin/env")
	expectedEnvPath, err := filepath.EvalSymlinks("/usr/bin/env")
	require.NoError(t, err)
	assert.Equal(t, expectedEnvPath, info.InterpreterPath)

	assert.Equal(t, "sh", info.CommandName)

	// ResolvedPath = EvalSymlinks(LookPath("sh"))
	require.NotEmpty(t, info.ResolvedPath)
	assert.True(t, filepath.IsAbs(info.ResolvedPath))
}

func TestParse_NotShebang_ELF(t *testing.T) {
	// ELF magic bytes: \x7fELF
	elfHeader := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	path := writeBinaryFile(t, elfHeader)
	info, err := shebang.Parse(path, realFS())
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestParse_NotShebang_Text(t *testing.T) {
	path := writeScript(t, "echo hello\n")
	info, err := shebang.Parse(path, realFS())
	require.NoError(t, err)
	assert.Nil(t, info)
}

func TestParse_ErrEmptyInterpreterPath(t *testing.T) {
	path := writeScript(t, "#!\n")
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrEmptyInterpreterPath)
}

func TestParse_ErrEmptyInterpreterPath_Whitespace(t *testing.T) {
	path := writeScript(t, "#!  \n")
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrEmptyInterpreterPath)
}

func TestParse_ErrInterpreterNotAbsolute(t *testing.T) {
	path := writeScript(t, "#!python3\n")
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrInterpreterNotAbsolute)
}

func TestParse_ErrMissingEnvCommand(t *testing.T) {
	path := writeScript(t, "#!/usr/bin/env\n")
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrMissingEnvCommand)
}

func TestParse_ErrEnvFlagNotSupported(t *testing.T) {
	path := writeScript(t, "#!/usr/bin/env -S python3\n")
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrEnvFlagNotSupported)
}

func TestParse_ErrEnvAssignmentNotSupported(t *testing.T) {
	path := writeScript(t, "#!/usr/bin/env PYTHONPATH=. python3\n")
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrEnvAssignmentNotSupported)
}

func TestParse_ErrCommandNotFound(t *testing.T) {
	path := writeScript(t, "#!/usr/bin/env nonexistent_cmd_xyz_123\n")
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrCommandNotFound)
}

func TestParse_ErrShebangLineTooLong(t *testing.T) {
	// Create a shebang line longer than 256 bytes without a newline.
	// "#!" is 2 bytes, then we need 254+ more bytes without '\n'.
	longLine := "#!" + strings.Repeat("x", 300) // no newline at all
	path := writeBinaryFile(t, []byte(longLine))
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrShebangLineTooLong)
}

func TestParse_ErrShebangCR(t *testing.T) {
	path := writeBinaryFile(t, []byte("#!/bin/sh\r\n"))
	_, err := shebang.Parse(path, realFS())
	assert.ErrorIs(t, err, shebang.ErrShebangCR)
}

// --- IsShebangScript tests ---

func TestIsShebangScript_True(t *testing.T) {
	path := writeScript(t, "#!/bin/sh\necho hello\n")
	result, err := shebang.IsShebangScript(path, realFS())
	require.NoError(t, err)
	assert.True(t, result)
}

func TestIsShebangScript_False_ELF(t *testing.T) {
	elfHeader := []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01, 0x01, 0x00}
	path := writeBinaryFile(t, elfHeader)
	result, err := shebang.IsShebangScript(path, realFS())
	require.NoError(t, err)
	assert.False(t, result)
}

func TestIsShebangScript_False_Text(t *testing.T) {
	path := writeScript(t, "echo hello\n")
	result, err := shebang.IsShebangScript(path, realFS())
	require.NoError(t, err)
	assert.False(t, result)
}

func TestIsShebangScript_False_Empty(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	path := filepath.Join(dir, "empty_file")
	require.NoError(t, os.WriteFile(path, []byte{}, 0o755))
	result, err := shebang.IsShebangScript(path, realFS())
	require.NoError(t, err)
	assert.False(t, result)
}

func TestIsShebangScript_False_OneByte(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	path := filepath.Join(dir, "one_byte_file")
	require.NoError(t, os.WriteFile(path, []byte("#"), 0o755))
	result, err := shebang.IsShebangScript(path, realFS())
	require.NoError(t, err)
	assert.False(t, result)
}
