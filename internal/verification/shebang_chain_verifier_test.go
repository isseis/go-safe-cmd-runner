//go:build test

package verification

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyCommandShebangInterpreter_ShebangChain_AbsoluteRef_SymlinkRedirected
// verifies that an absolute ref is re-resolved with EvalSymlinks and rejected
// when it points to a different binary than the recorded path.
func TestVerifyCommandShebangInterpreter_ShebangChain_AbsoluteRef_SymlinkRedirected(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	interpA := commontesting.WriteExecutableFile(t, dir, "interp_a", []byte("#!/bin/sh\n"))
	interpB := commontesting.WriteExecutableFile(t, dir, "interp_b", []byte("#!/bin/sh\n"))

	rawRef := filepath.Join(dir, "sh")
	require.NoError(t, os.Symlink(interpA, rawRef))

	scriptPath := filepath.Join(dir, "script.sh")
	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangChain: []fileanalysis.ShebangChainEntry{{
			Ref:  rawRef,
			Path: interpA,
		}},
	})

	// Simulate post-record tampering where the symlink now points elsewhere.
	require.NoError(t, os.Remove(rawRef))
	require.NoError(t, os.Symlink(interpB, rawRef))

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"})
	require.Error(t, err)

	var redirected *ErrInterpreterSymlinkRedirected
	assert.True(t, errors.As(err, &redirected))
	assert.Equal(t, rawRef, redirected.RawPath)
	assert.Equal(t, interpA, redirected.RecordedPath)
	assert.Equal(t, interpB, redirected.ActualPath)
}

// TestVerifyCommandShebangInterpreter_ShebangChain_BareRef_PathMismatch verifies
// that a bare ref is re-resolved through PATH and rejected when runtime
// resolution differs from the recorded path.
func TestVerifyCommandShebangInterpreter_ShebangChain_BareRef_PathMismatch(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	recordedDir := filepath.Join(dir, "recorded")
	runtimeDir := filepath.Join(dir, "runtime")
	require.NoError(t, os.MkdirAll(recordedDir, 0o755))
	require.NoError(t, os.MkdirAll(runtimeDir, 0o755))

	recordedInterp := commontesting.WriteExecutableFile(t, recordedDir, "python3", []byte("#!/bin/sh\n"))
	runtimeInterp := commontesting.WriteExecutableFile(t, runtimeDir, "python3", []byte("#!/bin/sh\n"))
	require.NotEqual(t, recordedInterp, runtimeInterp)

	scriptPath := filepath.Join(dir, "script.py")
	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangChain: []fileanalysis.ShebangChainEntry{{
			Ref:  "python3",
			Path: recordedInterp,
		}},
	})

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": runtimeDir})
	require.Error(t, err)

	var mismatch *ErrInterpreterPathMismatch
	assert.True(t, errors.As(err, &mismatch))
	assert.Equal(t, "python3", mismatch.CommandName)
	assert.Equal(t, recordedInterp, mismatch.RecordedPath)
	assert.Equal(t, runtimeInterp, mismatch.ActualPath)
}

// TestVerifyCommandShebangInterpreter_ShebangChain_EmptyPath verifies that a
// shebang_chain entry with an empty path is rejected as a corrupted record
// rather than silently skipped (fail-closed).
func TestVerifyCommandShebangInterpreter_ShebangChain_EmptyPath(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	scriptPath := filepath.Join(dir, "script.sh")

	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangChain: []fileanalysis.ShebangChainEntry{{
			Ref:  "/bin/sh",
			Path: "",
		}},
	})

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{})
	assert.Error(t, err, "empty shebang_chain path must be rejected, not silently skipped")
}
