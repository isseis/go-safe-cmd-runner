//go:build test

package verification

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
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

// TestVerifyCommandShebangInterpreter_ShebangChain_UnsupportedHashAlgorithm verifies
// that a dep hash with an unsupported algorithm prefix (e.g. "md5:") is rejected with
// ErrUnsupportedHashAlgorithm rather than ErrMismatch.
func TestVerifyCommandShebangInterpreter_ShebangChain_UnsupportedHashAlgorithm(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	interpPath := commontesting.WriteExecutableFile(t, dir, "interp", []byte("#!/bin/sh\n"))
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
	dir := commontesting.SafeTempDir(t)

	// interpPath is a real file used as the shebang interpreter.
	interpPath := commontesting.WriteExecutableFile(t, dir, "interp", []byte("#!/bin/sh\n"))

	buildRecord := func(scriptPath string) *fileanalysis.Record {
		interpHash, err := computeSHA256PrefixedHash(interpPath)
		require.NoError(t, err)
		return &fileanalysis.Record{
			SchemaVersion: fileanalysis.CurrentSchemaVersion,
			FilePath:      scriptPath,
			ContentHash:   "sha256:script",
			ShebangChain:  []fileanalysis.ShebangChainEntry{{Ref: interpPath, Path: interpPath}},
			DynLibDeps:    []fileanalysis.LibEntry{{Path: interpPath, Hash: interpHash}},
		}
	}

	script1 := filepath.Join(dir, "script1.sh")
	script2 := filepath.Join(dir, "script2.sh")

	mockFV := newMockFVForShebang()
	mockFV.setRecord(script1, buildRecord(script1))

	m := setupManagerWithMockValidator(t, mockFV)
	// A real DynLibVerifier is required to exercise the cache path.
	safeFS := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	m.dynlibVerifier = elfdynlib.NewDynLibVerifier(safeFS)
	m.safeFS = safeFS

	// Command 1: dynlib verification populates the cache.
	require.NoError(t, m.VerifyCommandDynLibDeps(script1))
	require.NoError(t, m.VerifyCommandShebangInterpreter(script1, map[string]string{}))

	// Simulate the interpreter being replaced before the second command runs.
	require.NoError(t, os.WriteFile(interpPath, []byte("#!/bin/sh\n# replaced\n"), 0o755))

	// Rebuild the record for script2 so it reflects the current (pre-replacement) hash —
	// i.e., the hash no longer matches the file on disk after the write above.
	// Build the record with the OLD hash to simulate a tampered file.
	oldRecord := buildRecord(script2) // hash was computed before the write above? No:
	// Actually buildRecord calls computeSHA256PrefixedHash NOW (after replacement),
	// so we need to set the old hash manually.
	oldHash := m.verifiedDepHashes[interpPath] // captured from command 1's cache
	oldRecord.DynLibDeps[0].Hash = oldHash
	oldRecord.ShebangChain[0].Path = interpPath
	mockFV.setRecord(script2, oldRecord)

	// Command 2: VerifyCommandDynLibDeps must reset the cache, so shebang
	// verification for script2 recomputes the hash and detects the mismatch.
	_ = m.VerifyCommandDynLibDeps(script2) // will fail due to hash mismatch — that's expected
	err := m.VerifyCommandShebangInterpreter(script2, map[string]string{})
	// If the cache were not reset, verifyInterpreterHash would skip computeHash
	// and return nil (false negative). With the reset, it must recompute and detect
	// the mismatch.
	assert.Error(t, err, "shebang verification must detect replaced interpreter after cache reset")
}

// computeSHA256PrefixedHash returns "sha256:<hex>" for the file at path.
func computeSHA256PrefixedHash(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is test-controlled
	if err != nil {
		return "", err
	}
	defer f.Close() //nolint:errcheck
	var h filevalidator.SHA256
	raw, err := h.Sum(f)
	if err != nil {
		return "", err
	}
	return "sha256:" + raw, nil
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
