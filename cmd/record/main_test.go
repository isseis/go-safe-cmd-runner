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
	"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer/testutil"
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
	elfanalyzertestutil.CreateStaticELFFile(t, staticELF)

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

// TestRunTOCTOU_FailsClosedOnWorldWritableDir verifies that the record command
// fails closed (non-zero exit, no hash generated) when the file's parent directory
// is world-writable. The hash DB is the root of trust — permission violations
// in ancestor directories must prevent hash record generation.
func TestRunTOCTOU_FailsClosedOnWorldWritableDir(t *testing.T) {
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

	// record must fail closed on TOCTOU violations — no hash generated, non-zero exit
	exitCode := run([]string{"-d", hashDir, targetFile}, testRunDeps(hashDir), stdout, stderr)

	assert.NotEqual(t, 0, exitCode, "record must fail closed (non-zero exit) on world-writable directory")
	assert.Contains(t, stderr.String(), "permission violation", "stderr must report permission violation")
	assert.NotContains(t, stdout.String(), "OK", "no hash file should have been generated")
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

// fakeDirPermChecker implements security.DirectoryPermChecker for testing.
type fakeDirPermChecker struct {
	validateDirFn func(path string) error
}

func (f *fakeDirPermChecker) ValidateDirectoryPermissions(path string) error {
	return f.validateDirFn(path)
}

// TestRunTOCTOU_NoViolation_Continues verifies that record continues with hash
// generation when no TOCTOU violations are detected in the hash directory.
func TestRunTOCTOU_NoViolation_Continues(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(hashDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := testRunDeps(hashDir)
	d.toctouChecker = &fakeDirPermChecker{validateDirFn: func(_ string) error { return nil }}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "OK", "hash generation should proceed without TOCTOU violations")
}

// TestRunTOCTOU_ViolationLogsErrorAndExits verifies that when a TOCTOU violation
// is detected, record logs ERROR (not WARN), prints the violation to stderr,
// and exits non-zero without generating hashes.
func TestRunTOCTOU_ViolationLogsErrorAndExits(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(hashDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := testRunDeps(hashDir)
	d.toctouChecker = &fakeDirPermChecker{validateDirFn: func(_ string) error {
		return errors.New("world-writable directory")
	}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	assert.NotEqual(t, 0, exitCode, "record must exit non-zero on TOCTOU violation")
	assert.Contains(t, stderr.String(), "permission violation", "stderr must report permission violation")
	assert.NotContains(t, stdout.String(), "OK", "no hash should be generated on violation")
}

// TestRunTOCTOU_ForceFlagDoesNotBypassViolation verifies that --force does NOT
// bypass TOCTOU permission violations. --force is for overwriting existing hash
// files only, not for overriding security checks.
func TestRunTOCTOU_ForceFlagDoesNotBypassViolation(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(hashDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := testRunDeps(hashDir)
	d.toctouChecker = &fakeDirPermChecker{validateDirFn: func(_ string) error {
		return errors.New("world-writable directory")
	}}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-force", "-d", hashDir, targetFile}, d, stdout, stderr)

	assert.NotEqual(t, 0, exitCode, "record must exit non-zero even with --force on TOCTOU violation")
	assert.Contains(t, stderr.String(), "permission violation", "stderr must report permission violation despite --force")
	assert.NotContains(t, stdout.String(), "OK", "no hash should be generated even with --force")
}

// TestHashDirPermissions_0o700 verifies that newly created hash directories use
// 0o700 permissions (owner rwx only, no group/other access).
func TestHashDirPermissions_0o700(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	// Remove the hashDir so parseArgs creates it fresh
	require.NoError(t, os.Remove(hashDir))

	d := testRunDeps(hashDir)
	// Restore real mkdirAll so we test actual permissions
	d.mkdirAll = os.MkdirAll

	stderr := &bytes.Buffer{}
	cfg, _, err := parseArgs([]string{"-d", hashDir, "dummy.txt"}, d, stderr)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	info, err := os.Stat(hashDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "hash directory must be created with 0o700")
}
