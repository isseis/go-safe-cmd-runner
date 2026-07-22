//go:build test

package verification

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyCommandShebangInterpreter_ShebangChain_AbsoluteRef_SymlinkRedirected
// verifies that an absolute ref is re-resolved with EvalSymlinks and rejected
// when it points to a different binary than the recorded path.
func TestVerifyCommandShebangInterpreter_ShebangChain_AbsoluteRef_SymlinkRedirected(t *testing.T) {
	dir := tu.SafeTempDir(t)
	interpA := tu.WriteExecutableFile(t, dir, "interp_a", []byte("#!/bin/sh\n"))
	interpB := tu.WriteExecutableFile(t, dir, "interp_b", []byte("#!/bin/sh\n"))

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
	dir := tu.SafeTempDir(t)
	recordedDir := filepath.Join(dir, "recorded")
	runtimeDir := filepath.Join(dir, "runtime")
	require.NoError(t, os.MkdirAll(recordedDir, 0o755))
	require.NoError(t, os.MkdirAll(runtimeDir, 0o755))

	recordedInterp := tu.WriteExecutableFile(t, recordedDir, "python3", []byte("#!/bin/sh\n"))
	runtimeInterp := tu.WriteExecutableFile(t, runtimeDir, "python3", []byte("#!/bin/sh\n"))
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

// TestVerifyCommandShebangInterpreter_ShebangChain_UnsupportedHashAlgorithm verifies
// that a dep hash with an unsupported algorithm prefix (e.g. "md5:") is rejected with
// ErrUnsupportedHashAlgorithm rather than ErrMismatch.
func TestVerifyCommandShebangInterpreter_ShebangChain_UnsupportedHashAlgorithm(t *testing.T) {
	dir := tu.SafeTempDir(t)
	interpPath := tu.WriteExecutableFile(t, dir, "interp", []byte("#!/bin/sh\n"))
	scriptPath := filepath.Join(dir, "script.sh")

	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangChain: []fileanalysis.ShebangChainEntry{{
			Ref:  interpPath,
			Path: interpPath,
		}},
		DynLibDeps: []fileanalysis.LibEntry{{
			Path: interpPath,
			Hash: "md5:d41d8cd98f00b204e9800998ecf8427e",
		}},
	})

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedHashAlgorithm)
}

// TestVerifyCommandDynLibDeps_ResetsDepHashCacheBetweenCommands verifies that
// the per-command dep-hash cache introduced for deduplication is reset before
// each VerifyCommandDynLibDeps call. Without the reset, a file replaced between
// two commands in the same group would pass shebang verification for the second
// command using the stale cached hash from the first command.
func TestVerifyCommandDynLibDeps_ResetsDepHashCacheBetweenCommands(t *testing.T) {
	dir := tu.SafeTempDir(t)
	interpPath := tu.WriteExecutableFile(t, dir, "interp", []byte("#!/bin/sh\n"))

	mockFV := newMockFVForShebang()
	m := setupManagerWithMockValidator(t, mockFV)
	// A real DynLibVerifier and safeFS are required to exercise the cache path.
	safeFS := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	m.dynlibVerifier = elfdynlib.NewDynLibVerifier(safeFS)
	m.safeFS = safeFS

	// Capture the hash of the original interpreter before it is replaced.
	// m.computeHash is the same code path used by verifyInterpreterHash at verify time.
	var sha256Hasher filevalidator.SHA256
	originalHash, err := m.computeHash(&sha256Hasher, interpPath)
	require.NoError(t, err)

	makeRecord := func(scriptPath string) *fileanalysis.Record {
		return &fileanalysis.Record{
			SchemaVersion: fileanalysis.CurrentSchemaVersion,
			FilePath:      scriptPath,
			ContentHash:   "sha256:script",
			ShebangChain:  []fileanalysis.ShebangChainEntry{{Ref: interpPath, Path: interpPath}},
			DynLibDeps:    []fileanalysis.LibEntry{{Path: interpPath, Hash: originalHash}},
		}
	}

	script1 := filepath.Join(dir, "script1.sh")
	script2 := filepath.Join(dir, "script2.sh")

	mockFV.setRecord(script1, makeRecord(script1))

	// Command 1: dynlib verification passes and populates the cache.
	require.NoError(t, m.VerifyCommandDynLibDeps(script1, nil))
	require.NoError(t, m.VerifyCommandShebangInterpreter(script1, map[string]string{}))

	// Replace the interpreter on disk. script2's record still carries the
	// original hash, so the disk content no longer matches the record.
	require.NoError(t, os.WriteFile(interpPath, []byte("#!/bin/sh\n# replaced\n"), 0o755))
	mockFV.setRecord(script2, makeRecord(script2))

	// Command 2: VerifyCommandDynLibDeps resets the cache before re-verifying,
	// so verifyInterpreterHash must recompute the hash from disk and detect the
	// mismatch rather than reusing the stale cache entry from command 1.
	_ = m.VerifyCommandDynLibDeps(script2, nil) // expected to fail — hash mismatch
	err = m.VerifyCommandShebangInterpreter(script2, map[string]string{})
	assert.Error(t, err, "shebang verification must detect replaced interpreter after cache reset")
}

// TestVerifyCommandShebangInterpreter_ShebangChain_EmptyPath verifies that a
// shebang_chain entry with an empty path is rejected as a corrupted record
// rather than silently skipped (fail-closed).
func TestVerifyCommandShebangInterpreter_ShebangChain_EmptyPath(t *testing.T) {
	dir := tu.SafeTempDir(t)
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

// TestVerifyCommandShebangInterpreter_ShebangChain_EmptyRef verifies that a
// shebang_chain entry with an empty ref is rejected as a corrupted record
// (fail-closed). An empty ref skips the runtime symlink-redirection and
// PATH-resolution checks, which would allow an attacker to redirect /bin/sh
// to a different binary without detection.
func TestVerifyCommandShebangInterpreter_ShebangChain_EmptyRef(t *testing.T) {
	dir := tu.SafeTempDir(t)
	interpPath := tu.WriteExecutableFile(t, dir, "interp", []byte("#!/bin/sh\n"))
	scriptPath := filepath.Join(dir, "script.sh")

	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangChain: []fileanalysis.ShebangChainEntry{{
			Ref:  "",
			Path: interpPath,
		}},
	})

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{})
	require.Error(t, err, "empty shebang_chain ref must be rejected (fail-closed)")
	assert.ErrorIs(t, err, ErrShebangChainEmptyRef)
}
