package verification

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
)

// The four errors below appear inside the dependency- and shebang-verification
// failure body. Each StructuredMessage must render exactly the text Error()
// produced, while declaring library names, interpreter references and paths as
// Path. A name such as libkeyutils.so.1 contains a word the whole-value
// replacement reacts to, so a Path declaration is what keeps it visible.
var (
	_ errmsg.Structured = (*ErrDynLibDepsResolutionChanged)(nil)
	_ errmsg.Structured = (*ErrInterpreterRecordNotFound)(nil)
	_ errmsg.Structured = (*ErrInterpreterSymlinkRedirected)(nil)
	_ errmsg.Structured = (*ErrInterpreterPathMismatch)(nil)
)

func TestErrDynLibDepsResolutionChanged_StructuredMessage(t *testing.T) {
	e := &ErrDynLibDepsResolutionChanged{
		SOName:       "libkeyutils.so.1",
		RecordedPath: "/old/libkeyutils.so.1",
		ResolvedPath: "/new/libkeyutils.so.1",
	}

	assert.Equal(t,
		"dynamic library dependency resolution changed since record: libkeyutils.so.1\n"+
			"  recorded path: /old/libkeyutils.so.1\n"+
			"  resolved path: /new/libkeyutils.so.1\n"+
			"  please re-run 'record' command if this change is expected",
		e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "dynamic library dependency resolution changed since record: "},
		{Role: errmsg.RolePath, Text: "libkeyutils.so.1"},
		{Role: errmsg.RoleConstant, Text: "\n  recorded path: "},
		{Role: errmsg.RolePath, Text: "/old/libkeyutils.so.1"},
		{Role: errmsg.RoleConstant, Text: "\n  resolved path: "},
		{Role: errmsg.RolePath, Text: "/new/libkeyutils.so.1"},
		{Role: errmsg.RoleConstant, Text: "\n  please re-run 'record' command if this change is expected"},
	}, e.StructuredMessage().Segments())
}

func TestErrDynLibDepsResolutionChanged_StructuredMessage_EmptySOName(t *testing.T) {
	e := &ErrDynLibDepsResolutionChanged{ResolvedPath: "/new/libfoo.so"}

	assert.Equal(t,
		"dynamic library dependency resolution changed since record: <unknown>\n"+
			"  recorded path: \n"+
			"  resolved path: /new/libfoo.so\n"+
			"  please re-run 'record' command if this change is expected",
		e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "dynamic library dependency resolution changed since record: "},
		{Role: errmsg.RolePath, Text: "<unknown>"},
		{Role: errmsg.RoleConstant, Text: "\n  recorded path: "},
		{Role: errmsg.RolePath, Text: ""},
		{Role: errmsg.RoleConstant, Text: "\n  resolved path: "},
		{Role: errmsg.RolePath, Text: "/new/libfoo.so"},
		{Role: errmsg.RoleConstant, Text: "\n  please re-run 'record' command if this change is expected"},
	}, e.StructuredMessage().Segments())
}

// TestResolveDynLibDeps_WrapIsStructured pins that re-resolution wraps an
// analyzer failure in a structured error. The wrap keeps the cause's own
// structure (errmsg.Cause), so an ErrRecursionDepthExceeded raised while
// re-resolving reaches the report with its SOName declared as a Path instead of
// being flattened into one text segment. Reverting the wrap to fmt.Errorf makes
// this test fail because the result is no longer an *errmsg.Error.
func TestResolveDynLibDeps_WrapIsStructured(t *testing.T) {
	m, err := NewManagerForTest(tu.SafeTempDir(t), WithFileValidatorDisabled(), WithSkipHashDirectoryValidation())
	require.NoError(t, err)

	_, err = m.resolveDynLibDeps(filepath.Join(t.TempDir(), "missing"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to re-resolve ELF dynamic library dependencies for ")

	_, ok := errors.AsType[*errmsg.Error](err)
	assert.True(t, ok, "the wrap must be structured so a structured cause survives to the report")
}

func TestErrInterpreterRecordNotFound_StructuredMessage(t *testing.T) {
	e := &ErrInterpreterRecordNotFound{Path: "/usr/bin/python3"}

	assert.Equal(t, "interpreter record not found: /usr/bin/python3", e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "interpreter record not found: "},
		{Role: errmsg.RolePath, Text: "/usr/bin/python3"},
	}, e.StructuredMessage().Segments())
}

func TestErrInterpreterSymlinkRedirected_StructuredMessage(t *testing.T) {
	e := &ErrInterpreterSymlinkRedirected{
		RawPath:      "/bin/sh",
		RecordedPath: "/usr/bin/dash",
		ActualPath:   "/usr/bin/bash",
	}

	assert.Equal(t,
		"interpreter symlink redirected: /bin/sh was /usr/bin/dash at record time, now resolves to /usr/bin/bash",
		e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "interpreter symlink redirected: "},
		{Role: errmsg.RolePath, Text: "/bin/sh"},
		{Role: errmsg.RoleConstant, Text: " was "},
		{Role: errmsg.RolePath, Text: "/usr/bin/dash"},
		{Role: errmsg.RoleConstant, Text: " at record time, now resolves to "},
		{Role: errmsg.RolePath, Text: "/usr/bin/bash"},
	}, e.StructuredMessage().Segments())
}

func TestErrInterpreterPathMismatch_StructuredMessage(t *testing.T) {
	e := &ErrInterpreterPathMismatch{
		CommandName:  "python3",
		RecordedPath: "/usr/local/bin/python3",
		ActualPath:   "/usr/bin/python3",
	}

	assert.Equal(t,
		"interpreter path mismatch for python3: recorded /usr/local/bin/python3, actual /usr/bin/python3",
		e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "interpreter path mismatch for "},
		{Role: errmsg.RolePath, Text: "python3"},
		{Role: errmsg.RoleConstant, Text: ": recorded "},
		{Role: errmsg.RolePath, Text: "/usr/local/bin/python3"},
		{Role: errmsg.RoleConstant, Text: ", actual "},
		{Role: errmsg.RolePath, Text: "/usr/bin/python3"},
	}, e.StructuredMessage().Segments())
}
