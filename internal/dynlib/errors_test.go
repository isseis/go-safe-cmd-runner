package dynlib

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
)

// The four errors below appear inside the dependency-verification failure body.
// Each StructuredMessage must render exactly the text Error() produced, while
// declaring library names and paths as Path and digests as Text. A library
// name such as libkeyutils.so.1 contains a word the whole-value replacement
// reacts to, so a Path declaration is what keeps it visible.
var (
	_ errmsg.Structured = (*ErrRecursionDepthExceeded)(nil)
	_ errmsg.Structured = (*ErrLibraryHashMismatch)(nil)
	_ errmsg.Structured = (*ErrEmptyLibraryPath)(nil)
	_ errmsg.Structured = (*ErrDynLibDepsRequired)(nil)
)

func TestErrRecursionDepthExceeded_StructuredMessage(t *testing.T) {
	e := &ErrRecursionDepthExceeded{SOName: "libkeyutils.so.1", Depth: 3, MaxDepth: 2}

	assert.Equal(t, "dependency resolution depth exceeded: libkeyutils.so.1 at depth 3 (max 2)", e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "dependency resolution depth exceeded: "},
		{Role: errmsg.RolePath, Text: "libkeyutils.so.1"},
		{Role: errmsg.RoleConstant, Text: " at depth "},
		{Role: errmsg.RoleText, Text: "3"},
		{Role: errmsg.RoleConstant, Text: " (max "},
		{Role: errmsg.RoleText, Text: "2"},
		{Role: errmsg.RoleConstant, Text: ")"},
	}, e.StructuredMessage().Segments())
}

func TestErrLibraryHashMismatch_StructuredMessage(t *testing.T) {
	e := &ErrLibraryHashMismatch{
		SOName:       "libkeyutils.so.1",
		Path:         "/usr/lib/x86_64-linux-gnu/libkeyutils.so.1",
		ExpectedHash: "sha256:aaaaaaaa",
		ActualHash:   "sha256:bbbbbbbb",
	}

	assert.Equal(t,
		"dynamic library hash mismatch: libkeyutils.so.1\n"+
			"  path: /usr/lib/x86_64-linux-gnu/libkeyutils.so.1\n"+
			"  expected hash: sha256:aaaaaaaa\n"+
			"  actual hash: sha256:bbbbbbbb\n"+
			"  please re-run 'record' command",
		e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "dynamic library hash mismatch: "},
		{Role: errmsg.RolePath, Text: "libkeyutils.so.1"},
		{Role: errmsg.RoleConstant, Text: "\n  path: "},
		{Role: errmsg.RolePath, Text: "/usr/lib/x86_64-linux-gnu/libkeyutils.so.1"},
		{Role: errmsg.RoleConstant, Text: "\n  expected hash: "},
		{Role: errmsg.RoleText, Text: "sha256:aaaaaaaa"},
		{Role: errmsg.RoleConstant, Text: "\n  actual hash: "},
		{Role: errmsg.RoleText, Text: "sha256:bbbbbbbb"},
		{Role: errmsg.RoleConstant, Text: "\n  please re-run 'record' command"},
	}, e.StructuredMessage().Segments())
}

func TestErrEmptyLibraryPath_StructuredMessage(t *testing.T) {
	e := &ErrEmptyLibraryPath{SOName: "libkeyutils.so.1"}

	assert.Equal(t,
		"incomplete record: empty path for library libkeyutils.so.1\n"+
			"  please re-run 'record' command",
		e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "incomplete record: empty path for library "},
		{Role: errmsg.RolePath, Text: "libkeyutils.so.1"},
		{Role: errmsg.RoleConstant, Text: "\n  please re-run 'record' command"},
	}, e.StructuredMessage().Segments())
}

func TestErrDynLibDepsRequired_StructuredMessage(t *testing.T) {
	e := &ErrDynLibDepsRequired{BinaryPath: "/usr/bin/curl"}

	assert.Equal(t,
		"dynamic library dependencies not recorded for binary: /usr/bin/curl\n"+
			"  please re-run 'record' command",
		e.Error())
	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "dynamic library dependencies not recorded for binary: "},
		{Role: errmsg.RolePath, Text: "/usr/bin/curl"},
		{Role: errmsg.RoleConstant, Text: "\n  please re-run 'record' command"},
	}, e.StructuredMessage().Segments())
}
