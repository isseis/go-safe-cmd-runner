//go:build darwin

package verification

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// findDyldCacheOnlyMachOBinary returns the path of a Mach-O binary whose
// LC_LOAD_DYLIB entries all point to the dyld shared cache (i.e., the libraries
// are absent on disk). Returns ("", false) if none found.
//
// /bin/ls is a canonical macOS Mach-O binary with only dyld-shared-cache deps.
func findDyldCacheOnlyMachOBinary(t *testing.T) (string, bool) {
	t.Helper()
	candidates := []string{"/bin/ls", "/bin/sh", "/bin/cat"}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// findNonDyldCacheMachOBinary returns the path of a Mach-O binary that has at
// least one LC_LOAD_DYLIB entry pointing to a file that exists on disk
// (i.e., a non-dyld-shared-cache library). Returns ("", false) when not found.
//
// Homebrew binaries typically link against Cellar frameworks that live on disk.
func findNonDyldCacheMachOBinary(t *testing.T) (string, bool) {
	t.Helper()

	// Common Homebrew binary locations.
	candidates := []string{
		"/opt/homebrew/bin/python3",
		"/usr/local/bin/python3",
		"/opt/homebrew/bin/git",
		"/usr/local/bin/git",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			resolved, err := filepath.EvalSymlinks(p)
			if err != nil {
				continue
			}
			return resolved, true
		}
	}
	return "", false
}

// TestVerify_MachODyldCacheOnly verifies that VerifyCommandDynLibDeps returns nil
// for a Mach-O binary whose all dynamic library dependencies are in the dyld
// shared cache (no on-disk libraries). Such a binary is treated equivalently to
// a static binary: no DynLibDeps record is required.
func TestVerify_MachODyldCacheOnly(t *testing.T) {
	cmdPath, found := findDyldCacheOnlyMachOBinary(t)
	if !found {
		t.Skip("no dyld-cache-only Mach-O binary found")
	}

	hashDir := commontesting.SafeTempDir(t)

	resolved, err := filepath.EvalSymlinks(cmdPath)
	if err != nil {
		t.Skipf("failed to resolve symlinks for %s: %v", cmdPath, err)
	}

	// Write a record with no DynLibDeps.
	getter := filevalidator.NewHybridHashFilePathGetter()
	store, err := fileanalysis.NewStore(hashDir, getter)
	require.NoError(t, err)
	resolvedPath, err := common.NewResolvedPath(resolved)
	require.NoError(t, err)
	err = store.Update(resolvedPath, func(record *fileanalysis.Record) error {
		record.ContentHash = "sha256:aabbcc"
		// DynLibDeps intentionally left nil
		return nil
	})
	require.NoError(t, err)

	m, err := NewManagerForTest(hashDir)
	require.NoError(t, err)

	verifyErr := m.VerifyCommandDynLibDeps(resolved)
	assert.NoError(t, verifyErr,
		"Mach-O binary with only dyld-shared-cache deps should not require DynLibDeps record")
}

// TestVerify_MachONoDynLibDeps verifies that VerifyCommandDynLibDeps returns
// ErrDynLibDepsRequired for a dynamically linked Mach-O binary that has a
// valid schema-14 record but no DynLibDeps snapshot.
func TestVerify_MachONoDynLibDeps(t *testing.T) {
	cmdPath, found := findNonDyldCacheMachOBinary(t)
	if !found {
		t.Skip("no non-dyld-cache Mach-O binary found")
	}

	hashDir := commontesting.SafeTempDir(t)

	// Write a record with no DynLibDeps.
	getter := filevalidator.NewHybridHashFilePathGetter()
	store, err := fileanalysis.NewStore(hashDir, getter)
	require.NoError(t, err)
	resolvedPath, err := common.NewResolvedPath(cmdPath)
	require.NoError(t, err)
	err = store.Update(resolvedPath, func(record *fileanalysis.Record) error {
		record.ContentHash = "sha256:aabbcc"
		// DynLibDeps intentionally left nil
		return nil
	})
	require.NoError(t, err)

	m, err := NewManagerForTest(hashDir)
	require.NoError(t, err)

	verifyErr := m.VerifyCommandDynLibDeps(cmdPath)
	require.Error(t, verifyErr)

	var errRequired *dynlib.ErrDynLibDepsRequired
	assert.ErrorAs(t, verifyErr, &errRequired,
		"Mach-O binary with non-dyld-cache deps without DynLibDeps should return ErrDynLibDepsRequired")
}

// TestVerify_MachOWithDynLibDeps verifies that VerifyCommandDynLibDeps returns
// nil for a Mach-O binary when a valid DynLibDeps snapshot is recorded and all
// library hashes match.
func TestVerify_MachOWithDynLibDeps(t *testing.T) {
	cmdPath, found := findNonDyldCacheMachOBinary(t)
	if !found {
		t.Skip("no non-dyld-cache Mach-O binary found")
	}

	hashDir := commontesting.SafeTempDir(t)

	// Use the `record` binary to generate a real hash record.
	recordBin, err := exec.LookPath("record")
	if err != nil {
		// Fall back to the build output.
		recordBin = "../../build/prod/record"
		if _, err2 := os.Stat(recordBin); err2 != nil {
			t.Skip("record binary not found; run 'make build' first")
		}
	}

	// Run `record` to populate the hash directory.
	cmd := exec.Command(recordBin, "--hash-dir", hashDir, cmdPath) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("record command failed (%v): %s", err, out)
	}

	m, err := NewManagerForTest(hashDir)
	require.NoError(t, err)

	verifyErr := m.VerifyCommandDynLibDeps(cmdPath)
	assert.NoError(t, verifyErr, "Mach-O binary with matching DynLibDeps should pass verification")
}

// TestVerify_MachOOldSchema verifies that VerifyCommandDynLibDeps skips dynlib
// verification (returns nil) when the stored record has a schema_version older
// than CurrentSchemaVersion. Old records predate Mach-O dynlib tracking and
// should not block execution.
func TestVerify_MachOOldSchema(t *testing.T) {
	cmdPath, found := findNonDyldCacheMachOBinary(t)
	if !found {
		t.Skip("no non-dyld-cache Mach-O binary found")
	}

	hashDir := commontesting.SafeTempDir(t)

	// Write a raw JSON record with schema_version = CurrentSchemaVersion - 1
	// so that Store.Load returns SchemaVersionMismatchError with Actual < Expected.
	getter := filevalidator.NewHybridHashFilePathGetter()
	resolvedPath, err := common.NewResolvedPath(cmdPath)
	require.NoError(t, err)
	resolvedHashDir, err := common.NewResolvedPath(hashDir)
	require.NoError(t, err)

	recordFilePath, err := getter.GetHashFilePath(resolvedHashDir, resolvedPath)
	require.NoError(t, err)

	oldSchema := fileanalysis.CurrentSchemaVersion - 1
	rawRecord := map[string]interface{}{
		"schema_version": oldSchema,
		"content_hash":   "sha256:aabbcc",
	}
	writeRawJSONRecord(t, recordFilePath, rawRecord)

	m, err := NewManagerForTest(hashDir)
	require.NoError(t, err)

	verifyErr := m.VerifyCommandDynLibDeps(cmdPath)
	assert.NoError(t, verifyErr,
		"old schema_version record should be skipped and not block Mach-O execution")
}

// writeRawJSONRecord writes a raw JSON object to the given path, creating
// parent directories as needed. Used to inject records with arbitrary schema versions.
func writeRawJSONRecord(t *testing.T, path string, record interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(record, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o750))
	require.NoError(t, os.WriteFile(path, data, 0o600))
}
