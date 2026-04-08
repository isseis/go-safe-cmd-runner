//go:build test

package verification

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupManagerWithMockValidator creates a Manager wired to a MockFileValidator
// for unit-testing the shebang verification logic without touching real files.
func setupManagerWithMockValidator(t *testing.T, mockFV *mockFVForShebang) *Manager {
	t.Helper()
	m := &Manager{
		fileValidator: mockFV,
	}
	return m
}

// mockFVForShebang is a minimal FileValidator stub for shebang-verification tests.
type mockFVForShebang struct {
	records   map[string]*fileanalysis.Record
	verifyErr map[string]error
	schemaErr map[string]error // errors returned by LoadRecord (e.g. SchemaVersionMismatchError)
}

func newMockFVForShebang() *mockFVForShebang {
	return &mockFVForShebang{
		records:   make(map[string]*fileanalysis.Record),
		verifyErr: make(map[string]error),
		schemaErr: make(map[string]error),
	}
}

func (m *mockFVForShebang) setRecord(path string, record *fileanalysis.Record) {
	m.records[path] = record
}

func (m *mockFVForShebang) setVerifyErr(path string, err error) {
	m.verifyErr[path] = err
}

func (m *mockFVForShebang) SaveRecord(_ string, _ bool) (string, string, error) { return "", "", nil }
func (m *mockFVForShebang) Verify(path string) error {
	if err, ok := m.verifyErr[path]; ok {
		return err
	}
	return nil
}

func (m *mockFVForShebang) VerifyWithHash(path string) (string, error) {
	if err, ok := m.verifyErr[path]; ok {
		return "", err
	}
	return "sha256:abc", nil
}

func (m *mockFVForShebang) VerifyWithPrivileges(_ string, _ runnertypes.PrivilegeManager) error {
	return nil
}
func (m *mockFVForShebang) VerifyAndRead(_ string) ([]byte, error) { return nil, nil }
func (m *mockFVForShebang) VerifyAndReadWithPrivileges(_ string, _ runnertypes.PrivilegeManager) ([]byte, error) {
	return nil, nil
}

func (m *mockFVForShebang) LoadRecord(path string) (*fileanalysis.Record, error) {
	if err, ok := m.schemaErr[path]; ok {
		return nil, err
	}
	if rec, ok := m.records[path]; ok {
		return rec, nil
	}
	return nil, fileanalysis.ErrRecordNotFound
}

// --- Tests ---

// TestVerifyCommandShebangInterpreter_NilShebang verifies that a command whose
// record has no ShebangInterpreter results in a no-op (nil return).
func TestVerifyCommandShebangInterpreter_NilShebang(t *testing.T) {
	mockFV := newMockFVForShebang()
	mockFV.setRecord("/usr/bin/ls", &fileanalysis.Record{
		SchemaVersion:      fileanalysis.CurrentSchemaVersion,
		FilePath:           "/usr/bin/ls",
		ContentHash:        "sha256:abc",
		ShebangInterpreter: nil,
	})

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter("/usr/bin/ls", map[string]string{"PATH": "/usr/bin"})
	assert.NoError(t, err)
}

// TestVerifyCommandShebangInterpreter_DirectForm_OK verifies that the direct-form
// happy path (interpreter hash OK) returns nil.
func TestVerifyCommandShebangInterpreter_DirectForm_OK(t *testing.T) {
	interpPath := "/usr/bin/dash"
	mockFV := newMockFVForShebang()
	mockFV.setRecord("/usr/local/bin/deploy.sh", &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      "/usr/local/bin/deploy.sh",
		ContentHash:   "sha256:abc",
		ShebangInterpreter: &fileanalysis.ShebangInterpreterInfo{
			InterpreterPath: interpPath,
		},
	})
	// interpreter record exists (no verifyErr → Verify returns nil)

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter("/usr/local/bin/deploy.sh", map[string]string{})
	assert.NoError(t, err)
}

// TestVerifyCommandShebangInterpreter_EnvForm_OK verifies the env-form happy path.
func TestVerifyCommandShebangInterpreter_EnvForm_OK(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	// Create a real sh symlink target so EvalSymlinks works in lookPathInEnv.
	shPath, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)

	mockFV := newMockFVForShebang()
	mockFV.setRecord(filepath.Join(dir, "process.py"), &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      filepath.Join(dir, "process.py"),
		ContentHash:   "sha256:abc",
		ShebangInterpreter: &fileanalysis.ShebangInterpreterInfo{
			InterpreterPath: "/usr/bin/env",
			CommandName:     "sh",
			ResolvedPath:    shPath,
		},
	})

	m := setupManagerWithMockValidator(t, mockFV)
	// Provide PATH that contains sh so lookPathInEnv can find it.
	err = m.VerifyCommandShebangInterpreter(
		filepath.Join(dir, "process.py"),
		map[string]string{"PATH": "/usr/bin:/bin"},
	)
	assert.NoError(t, err)
}

// TestVerifyCommandShebangInterpreter_RecordNotFound verifies that a missing
// interpreter record results in ErrInterpreterRecordNotFound.
func TestVerifyCommandShebangInterpreter_RecordNotFound(t *testing.T) {
	interpPath := "/usr/bin/dash"
	mockFV := newMockFVForShebang()
	mockFV.setRecord("/usr/local/bin/script.sh", &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      "/usr/local/bin/script.sh",
		ContentHash:   "sha256:abc",
		ShebangInterpreter: &fileanalysis.ShebangInterpreterInfo{
			InterpreterPath: interpPath,
		},
	})
	// Make the interpreter Verify return ErrHashFileNotFound to simulate missing record.
	mockFV.setVerifyErr(interpPath, filevalidator.ErrHashFileNotFound)

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter("/usr/local/bin/script.sh", map[string]string{})
	require.Error(t, err)
	var notFound *ErrInterpreterRecordNotFound
	assert.True(t, errors.As(err, &notFound))
	assert.Equal(t, interpPath, notFound.Path)
}

// TestVerifyCommandShebangInterpreter_HashMismatch verifies that a hash mismatch
// on the interpreter is propagated.
func TestVerifyCommandShebangInterpreter_HashMismatch(t *testing.T) {
	interpPath := "/usr/bin/dash"
	mockFV := newMockFVForShebang()
	mockFV.setRecord("/usr/local/bin/script.sh", &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      "/usr/local/bin/script.sh",
		ContentHash:   "sha256:abc",
		ShebangInterpreter: &fileanalysis.ShebangInterpreterInfo{
			InterpreterPath: interpPath,
		},
	})
	mockFV.setVerifyErr(interpPath, filevalidator.ErrMismatch)

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter("/usr/local/bin/script.sh", map[string]string{})
	require.Error(t, err)
	assert.ErrorIs(t, err, filevalidator.ErrMismatch)
}

// TestVerifyCommandShebangInterpreter_PathMismatch verifies that when env PATH
// resolution finds a different binary than recorded, ErrInterpreterPathMismatch
// is returned.
func TestVerifyCommandShebangInterpreter_PathMismatch(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	bashPath, err := filepath.EvalSymlinks("/bin/bash")
	require.NoError(t, err)
	shPath, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)

	// Record says "sh" resolves to bash; env PATH will find sh.
	// If bash != sh (which is typical: bash is /usr/bin/bash, sh is /usr/bin/dash),
	// this should cause a mismatch.
	if bashPath == shPath {
		t.Skip("bash and sh resolve to the same binary on this system")
	}

	scriptPath := filepath.Join(dir, "process.py")
	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangInterpreter: &fileanalysis.ShebangInterpreterInfo{
			InterpreterPath: "/usr/bin/env",
			CommandName:     "sh",
			ResolvedPath:    bashPath, // recorded as bash, but PATH will resolve to sh
		},
	})

	m := setupManagerWithMockValidator(t, mockFV)
	err = m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"})
	require.Error(t, err)
	var mismatch *ErrInterpreterPathMismatch
	assert.True(t, errors.As(err, &mismatch))
}

// TestVerifyCommandShebangInterpreter_NoRecord verifies that a command with no
// record at all is silently skipped (returns nil).
func TestVerifyCommandShebangInterpreter_NoRecord(t *testing.T) {
	mockFV := newMockFVForShebang()
	// No record set — LoadRecord returns ErrRecordNotFound.

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter("/usr/bin/ls", map[string]string{"PATH": "/usr/bin"})
	assert.NoError(t, err)
}

// TestVerifyCommandShebangInterpreter_OldSchema verifies that a command whose
// record has a schema version older than CurrentSchemaVersion is rejected with
// SchemaVersionMismatchError. This covers the skip_standard_paths path where
// VerifyGroupFiles does not check the file but shebang verification must still
// enforce the schema version.
func TestVerifyCommandShebangInterpreter_OldSchema(t *testing.T) {
	mockFV := newMockFVForShebang()
	// Simulate a record that fails to load with SchemaVersionMismatchError (old schema).
	oldSchemaErr := &fileanalysis.SchemaVersionMismatchError{
		Actual:   fileanalysis.CurrentSchemaVersion - 1,
		Expected: fileanalysis.CurrentSchemaVersion,
	}
	mockFV.schemaErr["/usr/local/bin/script.sh"] = oldSchemaErr

	m := setupManagerWithMockValidator(t, mockFV)
	err := m.VerifyCommandShebangInterpreter("/usr/local/bin/script.sh", map[string]string{})
	require.Error(t, err)
	var schemaErr *fileanalysis.SchemaVersionMismatchError
	assert.True(t, errors.As(err, &schemaErr), "expected SchemaVersionMismatchError, got: %v", err)
}

// TestVerifyCommandShebangInterpreter_EnvForm_HashMismatch verifies that a hash
// mismatch on the env-form resolved_path binary (i.e. the interpreter found via
// PATH has been tampered with) is propagated as ErrMismatch.
// This complements TestVerifyCommandShebangInterpreter_HashMismatch which covers
// the direct-form interpreter path only.
func TestVerifyCommandShebangInterpreter_EnvForm_HashMismatch(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	shPath, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)

	scriptPath := filepath.Join(dir, "process.py")
	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangInterpreter: &fileanalysis.ShebangInterpreterInfo{
			InterpreterPath: "/usr/bin/env",
			CommandName:     "sh",
			ResolvedPath:    shPath,
		},
	})
	// env binary verifies OK, but resolved_path has been tampered (hash mismatch).
	mockFV.setVerifyErr(shPath, filevalidator.ErrMismatch)

	m := setupManagerWithMockValidator(t, mockFV)
	err = m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"})
	require.Error(t, err)
	assert.ErrorIs(t, err, filevalidator.ErrMismatch)
}

// TestVerifyCommandShebangInterpreter_EnvForm_ResolvedPathMissing verifies that
// a missing record for resolved_path is reported as ErrInterpreterRecordNotFound
// rather than being masked by a PATH mismatch error (Issue: wrong verification order).
func TestVerifyCommandShebangInterpreter_EnvForm_ResolvedPathMissing(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	shPath, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)

	scriptPath := filepath.Join(dir, "process.py")
	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangInterpreter: &fileanalysis.ShebangInterpreterInfo{
			InterpreterPath: "/usr/bin/env",
			CommandName:     "sh",
			ResolvedPath:    shPath,
		},
	})
	// env binary verifies OK, but resolved_path record is missing.
	mockFV.setVerifyErr(shPath, filevalidator.ErrHashFileNotFound)

	m := setupManagerWithMockValidator(t, mockFV)
	err = m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"})
	require.Error(t, err)
	var notFound *ErrInterpreterRecordNotFound
	assert.True(t, errors.As(err, &notFound), "expected ErrInterpreterRecordNotFound, got: %v", err)
	assert.Equal(t, shPath, notFound.Path)
}

// TestVerifyCommandShebangInterpreter_RelativePathCommandName verifies that when
// command_name contains a path separator (e.g. an absolute path used directly),
// the runtime resolver applies filepath.Abs before EvalSymlinks — mirroring the
// record-time behaviour in parser.go — so a legitimate script is not falsely
// rejected due to CWD differences.
//
// Concretely: lookPathInEnv returns the path unchanged when containsPathSeparator
// is true. If that path happens to be relative, skipping filepath.Abs before
// EvalSymlinks would produce a CWD-relative real path that differs from the
// absolute real path stored at record time. This test exercises that branch.
func TestVerifyCommandShebangInterpreter_RelativePathCommandName(t *testing.T) {
	dir := commontesting.SafeTempDir(t)
	shPath, err := filepath.EvalSymlinks("/bin/sh")
	require.NoError(t, err)

	// Create a symlink "python3" → sh inside dir so the binary exists there.
	linkPath := filepath.Join(dir, "python3")
	require.NoError(t, os.Symlink(shPath, linkPath))

	// record-time: resolved_path = EvalSymlinks(Abs(linkPath)) = shPath
	scriptPath := filepath.Join(dir, "process.py")
	mockFV := newMockFVForShebang()
	mockFV.setRecord(scriptPath, &fileanalysis.Record{
		SchemaVersion: fileanalysis.CurrentSchemaVersion,
		FilePath:      scriptPath,
		ContentHash:   "sha256:abc",
		ShebangInterpreter: &fileanalysis.ShebangInterpreterInfo{
			InterpreterPath: "/usr/bin/env",
			// Use the absolute linkPath as command_name: contains a separator,
			// so lookPathInEnv returns it directly without PATH lookup.
			CommandName:  linkPath,
			ResolvedPath: shPath,
		},
	})

	m := setupManagerWithMockValidator(t, mockFV)
	err = m.VerifyCommandShebangInterpreter(scriptPath, map[string]string{"PATH": "/usr/bin:/bin"})
	assert.NoError(t, err)
}
