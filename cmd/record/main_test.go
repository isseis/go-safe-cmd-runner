package main

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordCall struct {
	file  string
	force bool
}

type fakeRecorder struct {
	responses map[string]error
	calls     []recordCall
	hashDir   string
}

func (f *fakeRecorder) Record(filePath string, force bool) (string, error) {
	f.calls = append(f.calls, recordCall{file: filePath, force: force})
	if err, ok := f.responses[filePath]; ok && err != nil {
		return "", err
	}
	return fmt.Sprintf("/hash/%s.json", filepath.Base(filePath)), nil
}

func overrideValidatorFactory(t *testing.T, recorder *fakeRecorder) func() {
	t.Helper()
	originalFactory := validatorFactory
	validatorFactory = func(hashDir string) (hashRecorder, error) {
		recorder.hashDir = hashDir
		return recorder, nil
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
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", tempDir, "file1.txt", "file2.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, tempDir, recorder.hashDir)
	require.Len(t, recorder.calls, 2)
	assert.Equal(t, []recordCall{{"file1.txt", false}, {"file2.txt", false}}, recorder.calls)
	assert.Contains(t, stdout.String(), "Processing 2 files...")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 0 failed")
	assert.Empty(t, stderr.String())
}

func TestRunReportsFailuresAndContinues(t *testing.T) {
	tempDir := t.TempDir()
	recorder := &fakeRecorder{responses: map[string]error{
		"bad.dat": errors.New("calculate hash failure"),
	}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-force", "-hash-dir", tempDir, "good1", "bad.dat", "good2"}, stdout, stderr)

	require.Equal(t, 1, exitCode)
	require.Len(t, recorder.calls, 3)
	assert.True(t, recorder.calls[0].force)
	assert.True(t, recorder.calls[1].force)
	assert.True(t, recorder.calls[2].force)
	assert.Contains(t, stdout.String(), "[2/3] bad.dat: FAILED")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 1 failed")
	assert.Contains(t, stderr.String(), "Error recording hash for bad.dat")
}

func TestRunWarnsWhenDeprecatedFlagUsed(t *testing.T) {
	tempDir := t.TempDir()
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", tempDir, "-file", "legacy.txt", "new.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 2)
	assert.Equal(t, "legacy.txt", recorder.calls[0].file)
	assert.Contains(t, stderr.String(), "deprecated")
}

func TestRunUsesDefaultHashDirectoryWhenNotSpecified(t *testing.T) {
	recorder := &fakeRecorder{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, recorder)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"file1.txt"}, stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Contains(t, recorder.hashDir, "go-safe-cmd-runner/hashes")
	require.Len(t, recorder.calls, 1)
	assert.Equal(t, "file1.txt", recorder.calls[0].file)
	assert.False(t, recorder.calls[0].force)
}
