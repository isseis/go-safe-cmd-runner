package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

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

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"file1.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Contains(t, validator.hashDir, "go-safe-cmd-runner/hashes")
	require.Len(t, validator.calls, 1)
	assert.Equal(t, "file1.txt", validator.calls[0].file)
}
