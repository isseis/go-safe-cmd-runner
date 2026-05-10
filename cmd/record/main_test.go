package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	elfanalyzertesting "github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer/testing"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
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
}

func (f *fakeRecorder) SaveRecord(filePath string, force bool) (string, string, error) {
	f.calls = append(f.calls, recordCall{file: filePath, force: force})
	if err, ok := f.responses[filePath]; ok && err != nil {
		return "", "", err
	}
	return fmt.Sprintf("/hash/%s.json", filepath.Base(filePath)), "sha256:fakehash", nil
}

// testDeps returns a deps suitable for tests that need to exercise run() setup
// (arg parsing, TOCTOU check, deprecated warning). The validatorFactory creates
// a real Validator rooted at the given hashDir; callers that only need to test
// processFiles() behavior should call processFiles() directly instead.
func testRunDeps(hashDir string) deps {
	d := defaultDeps()
	d.mkdirAll = func(path string, perm os.FileMode) error {
		if path == hashDir {
			return nil // already created by the test
		}
		return os.MkdirAll(path, perm)
	}
	return d
}

func TestRunRequiresAtLeastOneFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// validatorFactory is never called when arg parsing fails, so it can be nil.
	exitCode := run([]string{}, deps{mkdirAll: os.MkdirAll}, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "at least one file path")
}

func TestProcessFiles_MultipleFiles(t *testing.T) {
	recorder := &fakeRecorder{responses: map[string]error{}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cfg := &recordConfig{files: []string{"file1.txt", "file2.txt"}}

	exitCode := processFiles(recorder, cfg, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 2)
	assert.Equal(t, []recordCall{{"file1.txt", false}, {"file2.txt", false}}, recorder.calls)
	assert.Contains(t, stdout.String(), "Processing 2 files...")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 0 failed")
	assert.Empty(t, stderr.String())
}

func TestProcessFiles_ReportsFailuresAndContinues(t *testing.T) {
	recorder := &fakeRecorder{responses: map[string]error{
		"bad.dat": errors.New("calculate hash failure"),
	}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cfg := &recordConfig{files: []string{"good1", "bad.dat", "good2"}, force: true}

	exitCode := processFiles(recorder, cfg, stdout, stderr)

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
	hashDir := tu.SafeTempDir(t)
	legacyFile := filepath.Join(hashDir, "legacy.txt")
	newFile := filepath.Join(hashDir, "new.txt")
	require.NoError(t, os.WriteFile(legacyFile, []byte("legacy content"), 0o644))
	require.NoError(t, os.WriteFile(newFile, []byte("new content"), 0o644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, "-file", legacyFile, newFile}, testRunDeps(hashDir), stdout, stderr)

	require.Equal(t, 0, exitCode)
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

func TestProcessFiles_WithELF(t *testing.T) {
	tempDir := tu.SafeTempDir(t)
	recorder := &fakeRecorder{responses: map[string]error{}}

	staticELF := filepath.Join(tempDir, "static.elf")
	elfanalyzertesting.CreateStaticELFFile(t, staticELF)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cfg := &recordConfig{files: []string{staticELF}}

	exitCode := processFiles(recorder, cfg, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 1)
	assert.Equal(t, staticELF, recorder.calls[0].file)
	assert.Contains(t, stdout.String(), "OK")
}

func TestProcessFiles_SkipsNonELF(t *testing.T) {
	tempDir := tu.SafeTempDir(t)
	recorder := &fakeRecorder{responses: map[string]error{}}

	nonELF := filepath.Join(tempDir, "script.sh")
	err := os.WriteFile(nonELF, []byte("#!/bin/bash\necho hello"), 0o755)
	require.NoError(t, err)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	cfg := &recordConfig{files: []string{nonELF}}

	exitCode := processFiles(recorder, cfg, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, recorder.calls, 1)
	assert.NotContains(t, stderr.String(), "Syscall analysis failed")
}

// TestRunTOCTOU_ContinuesOnWorldWritableDir verifies that the record command
// continues processing even when the file's parent directory is world-writable.
// Ensures record warns but does not abort on TOCTOU violations.
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

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// record should continue (exit 0) despite the TOCTOU violation
	exitCode := run([]string{"-d", hashDir, targetFile}, testRunDeps(hashDir), stdout, stderr)

	// record does NOT abort on TOCTOU violations — it only logs a warning
	assert.Equal(t, 0, exitCode, "record should continue (exit 0) despite world-writable directory")
	assert.Contains(t, stdout.String(), "OK", "file should have been processed")
}

func extractHashFilePathFromStdout(t *testing.T, output string) string {
	t.Helper()
	idx := strings.LastIndex(output, "OK (")
	require.NotEqual(t, -1, idx, "stdout must contain successful output line")

	rest := output[idx+len("OK ("):]
	end := strings.Index(rest, ")")
	require.NotEqual(t, -1, end, "stdout must include closing parenthesis for hash path")

	return rest[:end]
}

func TestRun_DebugInfoFlag_ControlsDebugFieldOmitEmpty(t *testing.T) {
	target, err := exec.LookPath("ls")
	if err != nil {
		t.Skip("skipping: ls command not found in PATH")
	}

	t.Run("debug field omitted by default", func(t *testing.T) {
		hashDir := tu.SafeTempDir(t)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		exitCode := run([]string{"-d", hashDir, target}, defaultDeps(), stdout, stderr)
		require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())

		recordPath := extractHashFilePathFromStdout(t, stdout.String())
		recordBytes, readErr := os.ReadFile(recordPath)
		require.NoError(t, readErr)
		assert.NotContains(t, string(recordBytes), "\"debug\"", "debug must be omitted without -debug-info")
	})

	t.Run("debug field is emitted with debug-info", func(t *testing.T) {
		hashDir := tu.SafeTempDir(t)
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		exitCode := run([]string{"-d", hashDir, "-debug-info", target}, defaultDeps(), stdout, stderr)
		require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())

		recordPath := extractHashFilePathFromStdout(t, stdout.String())
		recordBytes, readErr := os.ReadFile(recordPath)
		require.NoError(t, readErr)
		assert.Contains(t, string(recordBytes), "\"debug\"", "debug must be emitted with -debug-info")
	})
}

func TestRun_ReRecordOldSchemaWithoutForce(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(hashDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	seedValidator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
	require.NoError(t, err)
	recordPath, _, err := seedValidator.SaveRecord(targetFile, false)
	require.NoError(t, err)

	data, err := os.ReadFile(recordPath)
	require.NoError(t, err)

	var oldRecord map[string]any
	require.NoError(t, json.Unmarshal(data, &oldRecord))
	oldRecord["schema_version"] = fileanalysis.CurrentSchemaVersion - 1

	updated, err := json.MarshalIndent(oldRecord, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(recordPath, updated, 0o600))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, defaultDeps(), stdout, stderr)
	require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())

	validator, err := filevalidator.New(&filevalidator.SHA256{}, hashDir, filevalidator.ValidatorConfig{})
	require.NoError(t, err)
	recorded, err := validator.LoadRecord(targetFile)
	require.NoError(t, err)
	assert.Equal(t, fileanalysis.CurrentSchemaVersion, recorded.SchemaVersion)
}
