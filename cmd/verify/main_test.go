package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type verifyCall struct {
	file string
}

type fakeValidator struct {
	responses map[string]error
	calls     []verifyCall
	hashDir   string
}

func (f *fakeValidator) Verify(filePath string) error {
	f.calls = append(f.calls, verifyCall{file: filePath})
	if err, ok := f.responses[filePath]; ok {
		return err
	}
	return nil
}

func overrideValidatorFactory(t *testing.T, validator *fakeValidator) func() {
	t.Helper()
	originalFactory := validatorFactory
	validatorFactory = func(hashDir string) (hashValidator, error) {
		validator.hashDir = hashDir
		return validator, nil
	}
	return func() {
		validatorFactory = originalFactory
	}
}

func TestRunRequiresAtLeastOneFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{}, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "at least one file path")
}

func TestRunProcessesMultipleFiles(t *testing.T) {
	tempDir := t.TempDir()
	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", tempDir, "file1.txt", "file2.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, tempDir, validator.hashDir)
	require.Len(t, validator.calls, 2)
	assert.Equal(t, []verifyCall{{"file1.txt"}, {"file2.txt"}}, validator.calls)
	assert.Contains(t, stdout.String(), "Verifying 2 files...")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 0 failed")
	assert.Empty(t, stderr.String())
}

func TestRunReportsFailuresAndContinues(t *testing.T) {
	tempDir := t.TempDir()
	validator := &fakeValidator{responses: map[string]error{
		"bad.txt": errors.New("hash mismatch"),
	}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", tempDir, "good.txt", "bad.txt", "later.txt"}, stdout, stderr)

	require.Equal(t, 1, exitCode)
	require.Len(t, validator.calls, 3)
	assert.Contains(t, stdout.String(), "[2/3] bad.txt: FAILED")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 1 failed")
	assert.Contains(t, stderr.String(), "Verification failed for bad.txt")
}

func TestRunWarnsWhenDeprecatedFlagUsed(t *testing.T) {
	tempDir := t.TempDir()
	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", tempDir, "-file", "legacy.txt", "new.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, validator.calls, 2)
	assert.Equal(t, "legacy.txt", validator.calls[0].file)
	assert.Contains(t, stderr.String(), "deprecated")
}

func TestParseArgsInvalidHashDir(t *testing.T) {
	tempDir := t.TempDir()
	noWriteDir := filepath.Join(tempDir, "no_write")
	require.NoError(t, os.Mkdir(noWriteDir, 0o400))
	defer os.Chmod(noWriteDir, 0o755)

	invalidHashDir := filepath.Join(noWriteDir, "hashes")

	cfg, _, err := parseArgs([]string{"-hash-dir", invalidHashDir, "file.txt"}, bytes.NewBuffer(nil))

	assert.Nil(t, cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, errEnsureHashDir)
}

func TestRunUsesDefaultHashDirectoryWhenNotSpecified(t *testing.T) {
	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	// Override mkdirAll to avoid permission issues in CI
	originalMkdirAll := mkdirAll
	mkdirAll = func(_ string, _ os.FileMode) error {
		return nil
	}
	defer func() { mkdirAll = originalMkdirAll }()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"file1.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, cmdcommon.DefaultHashDirectory, validator.hashDir)
	require.Len(t, validator.calls, 1)
	assert.Equal(t, "file1.txt", validator.calls[0].file)
}

// TestRunTOCTOU_ContinuesOnWorldWritableDir verifies that the verify command
// continues processing even when the file's parent directory is world-writable.
// This validates AC-M2S-7: verify warns but does not abort on TOCTOU violations.
func TestRunTOCTOU_ContinuesOnWorldWritableDir(t *testing.T) {
	// Create a world-writable directory with a target file
	worldWritableDir := tu.SafeTempDir(t)
	err := os.Chmod(worldWritableDir, 0o777)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chmod(worldWritableDir, 0o755)
	})

	targetFile := filepath.Join(worldWritableDir, "target.txt")
	err = os.WriteFile(targetFile, []byte("hello"), 0o644)
	require.NoError(t, err)

	hashDir := tu.SafeTempDir(t)
	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// verify should continue (exit 0) despite the TOCTOU violation
	exitCode := run([]string{"-hash-dir", hashDir, targetFile}, stdout, stderr)

	// verify does NOT abort on TOCTOU violations — it only logs a warning
	assert.Equal(t, 0, exitCode, "verify should continue (exit 0) despite world-writable directory")
	require.Len(t, validator.calls, 1, "file should have been processed")
}

// TestVerifyDeclaresSudoUIDAwarePolicy verifies that this binary's init()
// declared SudoUIDAware as the process-wide permission check UID policy, and
// that under that policy a valid SUDO_UID is adopted when the real UID is 0,
// matching the resolvePermissionCheckUID behavior for a valid SUDO_UID; the
// existence-check seam is wired in a later phase of this task. This test only
// reads the process-wide default policy; it does not modify it, so it must
// not run in parallel with tests that do.
func TestVerifyDeclaresSudoUIDAwarePolicy(t *testing.T) {
	require.Equal(t, groupmembership.SudoUIDAware, groupmembership.ProcessPermissionCheckUIDPolicy())

	deps := groupmembership.NewPermissionCheckUIDDepsForTesting()
	deps.Getenv = func(string) string { return "1000" }

	uid, err := groupmembership.ResolvePermissionCheckUID(
		groupmembership.ProcessPermissionCheckUIDPolicy(), 0, deps)
	require.NoError(t, err)
	assert.Equal(t, 1000, uid)
}
