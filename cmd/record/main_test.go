package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	elfanalyzertesting "github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer/testing"
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

func (f *fakeRecorder) SaveRecord(filePath string, force bool) (string, string, error) {
	f.calls = append(f.calls, recordCall{file: filePath, force: force})
	if err, ok := f.responses[filePath]; ok && err != nil {
		return "", "", err
	}
	return fmt.Sprintf("/hash/%s.json", filepath.Base(filePath)), "sha256:fakehash", nil
}

// testDeps returns a deps with the given recorder wired as the validatorFactory.
// Callers can override individual fields afterwards when needed.
func testDeps(recorder *fakeRecorder) deps {
	return deps{
		validatorFactory: func(hashDir string) (hashRecorder, error) {
			if recorder != nil {
				recorder.hashDir = hashDir
			}
			return recorder, nil
		},
		mkdirAll: os.MkdirAll,
	}
}

func TestRunRequiresAtLeastOneFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{}, testDeps(nil), stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "at least one file path")
}

func TestRunProcessesMultipleFiles(t *testing.T) {
	tempDir := commontesting.SafeTempDir(t)
	recorder := &fakeRecorder{responses: map[string]error{}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", tempDir, "file1.txt", "file2.txt"}, testDeps(recorder), stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Equal(t, tempDir, recorder.hashDir)
	require.Len(t, recorder.calls, 2)
	assert.Equal(t, []recordCall{{"file1.txt", false}, {"file2.txt", false}}, recorder.calls)
	assert.Contains(t, stdout.String(), "Processing 2 files...")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 0 failed")
	assert.Empty(t, stderr.String())
}

func TestRunReportsFailuresAndContinues(t *testing.T) {
	tempDir := commontesting.SafeTempDir(t)
	recorder := &fakeRecorder{responses: map[string]error{
		"bad.dat": errors.New("calculate hash failure"),
	}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-force", "-hash-dir", tempDir, "good1", "bad.dat", "good2"}, testDeps(recorder), stdout, stderr)

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
	tempDir := commontesting.SafeTempDir(t)
	recorder := &fakeRecorder{responses: map[string]error{}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", tempDir, "-file", "legacy.txt", "new.txt"}, testDeps(recorder), stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 2)
	assert.Equal(t, "legacy.txt", recorder.calls[0].file)
	assert.Contains(t, stderr.String(), "deprecated")
}

func TestRunUsesDefaultHashDirectoryWhenNotSpecified(t *testing.T) {
	// Pass a no-op mkdirAll to avoid filesystem access to the default hash directory in CI.
	d := deps{
		mkdirAll: func(_ string, _ os.FileMode) error { return nil },
	}

	stderr := &bytes.Buffer{}

	cfg, _, err := parseArgs([]string{"file1.txt"}, d, stderr)

	require.NoError(t, err)
	assert.Equal(t, cmdcommon.DefaultHashDirectory, cfg.hashDir)
	assert.Equal(t, []string{"file1.txt"}, cfg.files)
	assert.False(t, cfg.force)
	assert.False(t, cfg.debugInfo)
}

func TestParseArgsDebugInfoFlag(t *testing.T) {
	d := deps{
		mkdirAll: func(_ string, _ os.FileMode) error { return nil },
	}

	stderr := &bytes.Buffer{}

	cfg, _, err := parseArgs([]string{"--debug-info", "file1.txt"}, d, stderr)

	require.NoError(t, err)
	assert.True(t, cfg.debugInfo)
}

func TestRunWithSyscallAnalysis(t *testing.T) {
	tempDir := commontesting.SafeTempDir(t)
	recorder := &fakeRecorder{responses: map[string]error{}}

	// Create a static ELF file for testing
	staticELF := filepath.Join(tempDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, staticELF)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Syscall analysis is always enabled
	exitCode := run([]string{"-d", tempDir, staticELF}, testDeps(recorder), stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 1)
	assert.Equal(t, staticELF, recorder.calls[0].file)
	assert.Contains(t, stdout.String(), "OK")
}

func TestRunWithSyscallAnalysisSkipsNonELF(t *testing.T) {
	tempDir := commontesting.SafeTempDir(t)
	recorder := &fakeRecorder{responses: map[string]error{}}

	// Create a non-ELF file
	nonELF := filepath.Join(tempDir, "script.sh")
	err := os.WriteFile(nonELF, []byte("#!/bin/bash\necho hello"), 0o755)
	require.NoError(t, err)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Syscall analysis is always enabled but should skip non-ELF files without warning
	exitCode := run([]string{"-d", tempDir, nonELF}, testDeps(recorder), stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 1)
	// No warning should be printed for non-ELF files
	assert.NotContains(t, stderr.String(), "Syscall analysis failed")
}

// TestRunTOCTOU_ContinuesOnWorldWritableDir verifies that the record command
// continues processing even when the file's parent directory is world-writable.
// This validates AC-M2S-7: record warns but does not abort on TOCTOU violations.
func TestRunTOCTOU_ContinuesOnWorldWritableDir(t *testing.T) {
	// Create a world-writable directory with a target file
	worldWritableDir := commontesting.SafeTempDir(t)
	err := os.Chmod(worldWritableDir, 0o777)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Chmod(worldWritableDir, 0o755)
	})

	targetFile := filepath.Join(worldWritableDir, "target.txt")
	err = os.WriteFile(targetFile, []byte("hello"), 0o644)
	require.NoError(t, err)

	hashDir := commontesting.SafeTempDir(t)
	recorder := &fakeRecorder{responses: map[string]error{}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// record should continue (exit 0) despite the TOCTOU violation
	exitCode := run([]string{"-d", hashDir, targetFile}, testDeps(recorder), stdout, stderr)

	// record does NOT abort on TOCTOU violations — it only logs a warning
	assert.Equal(t, 0, exitCode, "record should continue (exit 0) despite world-writable directory")
	require.Len(t, recorder.calls, 1, "file should have been processed")
}
