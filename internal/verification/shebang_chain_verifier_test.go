//go:build test

package verification

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyShebangInterpreter_ShebangChain_AbsoluteRef_SymlinkRedirected
// verifies that an absolute ref is re-resolved with EvalSymlinks and rejected
// when it points to a different binary than the recorded path.
func TestVerifyShebangInterpreter_ShebangChain_AbsoluteRef_SymlinkRedirected(t *testing.T) {
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
	err := m.verifyShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"}, map[string]string{})
	require.Error(t, err)

	var redirected *ErrInterpreterSymlinkRedirected
	assert.True(t, errors.As(err, &redirected))
	assert.Equal(t, rawRef, redirected.RawPath)
	assert.Equal(t, interpA, redirected.RecordedPath)
	assert.Equal(t, interpB, redirected.ActualPath)
}

// TestVerifyShebangInterpreter_ShebangChain_BareRef_PathMismatch verifies
// that a bare ref is re-resolved through PATH and rejected when runtime
// resolution differs from the recorded path.
func TestVerifyShebangInterpreter_ShebangChain_BareRef_PathMismatch(t *testing.T) {
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
	err := m.verifyShebangInterpreter(scriptPath, map[string]string{"PATH": runtimeDir}, map[string]string{})
	require.Error(t, err)

	var mismatch *ErrInterpreterPathMismatch
	assert.True(t, errors.As(err, &mismatch))
	assert.Equal(t, "python3", mismatch.CommandName)
	assert.Equal(t, recordedInterp, mismatch.RecordedPath)
	assert.Equal(t, runtimeInterp, mismatch.ActualPath)
}

// TestVerifyShebangInterpreter_ShebangChain_UnsupportedHashAlgorithm verifies
// that a dep hash with an unsupported algorithm prefix (e.g. "md5:") is rejected with
// ErrUnsupportedHashAlgorithm rather than ErrMismatch.
func TestVerifyShebangInterpreter_ShebangChain_UnsupportedHashAlgorithm(t *testing.T) {
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
	err := m.verifyShebangInterpreter(scriptPath, map[string]string{}, map[string]string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedHashAlgorithm)
}

// setupCacheScopeManager builds a Manager with real dynlib machinery (required
// because verifyDynLibDeps fails closed when the analyzers are nil) over a mock
// FileValidator, and returns it together with the mock.
func setupCacheScopeManager(t *testing.T) (*Manager, *mockFVForShebang) {
	t.Helper()
	mockFV := newMockFVForShebang()
	m := setupManagerWithMockValidator(t, mockFV)
	safeFS := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	m.dynlibVerifier = elfdynlib.NewDynLibVerifier(safeFS)
	m.elfDynLibAnalyzer = elfdynlib.NewDynLibAnalyzer(safeFS)
	m.machoDynLibAnalyzer = machodylib.NewMachODynLibAnalyzer(safeFS)
	m.safeFS = safeFS
	return m, mockFV
}

// TestVerifyCommandDependencies_RejectsFileReplacedBetweenCommands verifies that
// an interpreter replaced on disk between two commands of the same group is
// rejected on the second command: nothing verified for command 1 vouches for
// command 2.
//
// The rejection comes from the dynlib stage, which re-hashes every recorded dep
// from disk. That stage runs before the shebang stage inside every
// VerifyCommandDependencies call, which is why a stale dep-hash cache entry can
// no longer reach the shebang fast path — see
// TestVerifyInterpreterHash_CacheFastPathTrustsCallerScopedEntry for what that
// fast path does when it is reached.
func TestVerifyCommandDependencies_RejectsFileReplacedBetweenCommands(t *testing.T) {
	dir := tu.SafeTempDir(t)
	interpPath := tu.WriteExecutableFile(t, dir, "interp", []byte("#!/bin/sh\n"))

	m, mockFV := setupCacheScopeManager(t)

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

	// script1/script2 must exist on disk (as real shebang scripts) so the
	// elfDynLibAnalyzer can inspect them during re-resolution instead of
	// failing on a missing file.
	script1 := tu.WriteExecutableFile(t, dir, "script1.sh", []byte("#!/bin/sh\n"))
	script2 := tu.WriteExecutableFile(t, dir, "script2.sh", []byte("#!/bin/sh\n"))
	mockFV.setRecord(script1, makeRecord(script1))
	mockFV.setRecord(script2, makeRecord(script2))

	// Command 1: both stages pass and the interpreter hash is cached internally.
	require.NoError(t, m.VerifyCommandDependencies(script1, map[string]string{}))

	// Replace the interpreter on disk. script2's record still carries the
	// original hash, so the disk content no longer matches the record.
	require.NoError(t, os.WriteFile(interpPath, []byte("#!/bin/sh\n# replaced\n"), 0o755))

	// Command 2 must recompute from disk and reject the replacement.
	assert.Error(t, m.VerifyCommandDependencies(script2, map[string]string{}),
		"verification must detect the replaced interpreter on the second command")
}

// TestVerifyInterpreterHash_CacheFastPathTrustsCallerScopedEntry pins the
// semantics that make the cache's scope load-bearing: an entry present in the
// passed cache suppresses the disk re-hash entirely. This is safe only because
// VerifyCommandDependencies builds the map fresh per call and fills it solely
// from deps it verified in that same call — hence the companion assertion that
// an empty cache does detect the replacement.
func TestVerifyInterpreterHash_CacheFastPathTrustsCallerScopedEntry(t *testing.T) {
	dir := tu.SafeTempDir(t)
	interpPath := tu.WriteExecutableFile(t, dir, "interp", []byte("#!/bin/sh\n"))

	m, mockFV := setupCacheScopeManager(t)

	var sha256Hasher filevalidator.SHA256
	originalHash, err := m.computeHash(&sha256Hasher, interpPath)
	require.NoError(t, err)

	scriptPath := tu.WriteExecutableFile(t, dir, "script.sh", []byte("#!/bin/sh\n"))
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:script",
		ShebangChain:  []fileanalysis.ShebangChainEntry{{Ref: interpPath, Path: interpPath}},
		DynLibDeps:    []fileanalysis.LibEntry{{Path: interpPath, Hash: originalHash}},
	})

	// Replace the interpreter so that disk content and recorded hash diverge.
	require.NoError(t, os.WriteFile(interpPath, []byte("#!/bin/sh\n# replaced\n"), 0o755))

	// Empty cache: the hash is recomputed from disk and the mismatch is caught.
	assert.Error(t, m.verifyShebangInterpreter(scriptPath, map[string]string{}, map[string]string{}),
		"an empty cache must force a disk re-hash")

	// Cache carrying the recorded hash: the fast path is taken and the stale
	// entry is trusted. Only per-call scoping keeps this from being a hole.
	assert.NoError(t, m.verifyShebangInterpreter(scriptPath, map[string]string{},
		map[string]string{interpPath: originalHash}),
		"a cache entry matching the record must suppress the disk re-hash")
}

// TestVerifyCommandDependencies_ConcurrentCallsAreRaceFree verifies that a
// single Manager can serve concurrent verifications. The dep-hash cache used to
// be a Manager field written on every call; under -race, concurrent callers hit
// a data race on that map. Meaningful only under `go test -race`.
func TestVerifyCommandDependencies_ConcurrentCallsAreRaceFree(t *testing.T) {
	dir := tu.SafeTempDir(t)
	interpPath := tu.WriteExecutableFile(t, dir, "interp", []byte("#!/bin/sh\n"))

	m, mockFV := setupCacheScopeManager(t)

	var sha256Hasher filevalidator.SHA256
	originalHash, err := m.computeHash(&sha256Hasher, interpPath)
	require.NoError(t, err)

	const numScripts = 8
	scripts := make([]string, 0, numScripts)
	for i := range numScripts {
		scriptPath := tu.WriteExecutableFile(t, dir, fmt.Sprintf("concurrent%d.sh", i), []byte("#!/bin/sh\n"))
		mockFV.setRecord(scriptPath, &fileanalysis.Record{
			SchemaVersion: fileanalysis.CurrentSchemaVersion,
			FilePath:      scriptPath,
			ContentHash:   "sha256:script",
			ShebangChain:  []fileanalysis.ShebangChainEntry{{Ref: interpPath, Path: interpPath}},
			DynLibDeps:    []fileanalysis.LibEntry{{Path: interpPath, Hash: originalHash}},
		})
		scripts = append(scripts, scriptPath)
	}

	var wg sync.WaitGroup
	errs := make([]error, len(scripts))
	for i, scriptPath := range scripts {
		wg.Go(func() {
			errs[i] = m.VerifyCommandDependencies(scriptPath, map[string]string{})
		})
	}
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "concurrent verification of %s must succeed", scripts[i])
	}
}

// TestVerifyShebangInterpreter_ShebangChain_EmptyPath verifies that a
// shebang_chain entry with an empty path is rejected as a corrupted record
// rather than silently skipped (fail-closed).
func TestVerifyShebangInterpreter_ShebangChain_EmptyPath(t *testing.T) {
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
	err := m.verifyShebangInterpreter(scriptPath, map[string]string{}, map[string]string{})
	assert.Error(t, err, "empty shebang_chain path must be rejected, not silently skipped")
}

// TestVerifyShebangInterpreter_ShebangChain_EmptyRef verifies that a
// shebang_chain entry with an empty ref is rejected as a corrupted record
// (fail-closed). An empty ref skips the runtime symlink-redirection and
// PATH-resolution checks, which would allow an attacker to redirect /bin/sh
// to a different binary without detection.
func TestVerifyShebangInterpreter_ShebangChain_EmptyRef(t *testing.T) {
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
	err := m.verifyShebangInterpreter(scriptPath, map[string]string{}, map[string]string{})
	require.Error(t, err, "empty shebang_chain ref must be rejected (fail-closed)")
	assert.ErrorIs(t, err, ErrShebangChainEmptyRef)
}
