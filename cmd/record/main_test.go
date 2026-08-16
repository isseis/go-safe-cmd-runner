package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/security"
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

// newFailingPermChecker returns a newPermChecker that never produces a checker.
// cmd/verify gains the same stub in its own main package once it grows a
// newPermChecker field of its own, at which point the duplication between the
// two is intentional rather than something to lift into testutil/.
func newFailingPermChecker(err error) func() (security.DirectoryPermChecker, error) {
	return func() (security.DirectoryPermChecker, error) { return nil, err }
}

func TestRunRequiresAtLeastOneFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	// Nothing beyond arg parsing runs when parsing fails, so an empty deps is enough.
	exitCode := run([]string{}, deps{}, stdout, stderr)

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

	exitCode := run([]string{"-hash-dir", hashDir, "-file", legacyFile, newFile}, defaultDeps(), stdout, stderr)

	require.Equal(t, 0, exitCode)
	assert.Contains(t, stderr.String(), "deprecated")
}

func TestRunUsesDefaultHashDirectoryWhenNotSpecified(t *testing.T) {
	// parseArgs no longer touches the filesystem, so the default hash directory
	// can be exercised directly even where it does not exist (as in CI).
	stderr := &bytes.Buffer{}

	cfg, _, err := parseArgs([]string{"file1.txt"}, stderr)

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
	exitCode := run([]string{"-d", hashDir, targetFile}, defaultDeps(), stdout, stderr)

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
		if runtime.GOOS == "darwin" {
			// On modern macOS, system libraries like libSystem.B.dylib exist only
			// inside the dyld shared cache; the Mach-O dependency walker excludes
			// shared-cache libs from DynLibDeps by design (see
			// internal/dynlib/machodylib/doc.go), so a PATH-resolved "ls" has no
			// recordable dependency and the debug record stays empty regardless of
			// -debug-info. This assertion is inherently ELF/glibc-shaped.
			t.Skip("skipping: -debug-info's debug record needs a recorded dylib dependency, which macOS's shared-cache-only libSystem linkage does not provide for ls")
		}

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

// fixedPermChecker returns a newPermChecker that always yields checker.
func fixedPermChecker(checker security.DirectoryPermChecker) func() (security.DirectoryPermChecker, error) {
	return func() (security.DirectoryPermChecker, error) { return checker, nil }
}

// TestRunTOCTOU_NoViolation_Continues verifies that record continues with hash
// generation when no TOCTOU violations are detected in the hash directory.
func TestRunTOCTOU_NoViolation_Continues(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(hashDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := defaultDeps()
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(_ string) error { return nil }})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "OK", "hash generation should proceed without TOCTOU violations")
}

// TestRunTOCTOU_ViolationLogsErrorAndExits verifies that when a TOCTOU violation
// is detected, record logs ERROR (not WARN) with per-path violation details via
// slog, prints a generic permission-violation summary to stderr, and exits
// non-zero without generating hashes.
func TestRunTOCTOU_ViolationLogsErrorAndExits(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(hashDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := defaultDeps()
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(_ string) error {
		return errors.New("world-writable directory")
	}})

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

	d := defaultDeps()
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(_ string) error {
		return errors.New("world-writable directory")
	}})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-force", "-d", hashDir, targetFile}, d, stdout, stderr)

	assert.NotEqual(t, 0, exitCode, "record must exit non-zero even with --force on TOCTOU violation")
	assert.Contains(t, stderr.String(), "permission violation", "stderr must report permission violation despite --force")
	assert.NotContains(t, stdout.String(), "OK", "no hash should be generated even with --force")
}

// TestHashDirPermissions_0o700 verifies that newly created hash directories use
// 0o700 permissions (owner rwx only, no group/other access). The directory is
// now created by run, after the permission check, rather than by parseArgs.
func TestHashDirPermissions_0o700(t *testing.T) {
	base := tu.SafeTempDir(t)
	hashDir := filepath.Join(base, "hashes")
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, defaultDeps(), stdout, stderr)
	require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())

	info, err := os.Stat(hashDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "hash directory must be created with 0o700")
}

// TestRunTOCTOU_ViolationLogsRemediationWithActualPath verifies that the ERROR
// log's remediation hint contains the actual violating path, not an unresolved
// string-concatenation template.
func TestRunTOCTOU_ViolationLogsRemediationWithActualPath(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(hashDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := defaultDeps()
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(path string) error {
		return fmt.Errorf("world-writable directory: %s", path)
	}})

	var logBuf bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, nil)))
	t.Cleanup(func() { slog.SetDefault(prevLogger) })

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	assert.NotEqual(t, 0, exitCode)
	logOutput := logBuf.String()
	assert.Contains(t, logOutput, "remediation=", "log must include a remediation hint")
	assert.Contains(t, logOutput, hashDir, "remediation hint must contain the actual violating path")
	assert.NotContains(t, logOutput, "'+v.Path+'", "remediation hint must not contain unresolved template syntax")
}

// TestRunFailsClosedWhenPermissionCheckUIDUnresolvable verifies that record
// resolves the permission check UID before it touches any file. record reads
// its target files through safefileio.SafeOpenFile, which runs no read-safety
// check, so a run creating new records would otherwise never resolve the UID
// and an unverifiable SUDO_UID would go undetected.
func TestRunFailsClosedWhenPermissionCheckUIDUnresolvable(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(hashDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("target content"), 0o644))

	d := defaultDeps()
	d.ensurePermissionCheckUID = func() error { return groupmembership.ErrSudoUIDUserNotFound }

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), groupmembership.ErrSudoUIDUserNotFound.Error())
	assert.Empty(t, stdout.String(), "no file must be processed once the UID cannot be resolved")

	entries, err := os.ReadDir(hashDir)
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"target.txt"}, names, "no hash record must be written")
}

// TestRecordDeclaresSudoUIDAwarePolicy verifies that this binary's init()
// declared SudoUIDAware as the process-wide permission check UID policy, and
// that under that policy a valid SUDO_UID is adopted when the real UID is 0,
// matching the resolvePermissionCheckUID behavior for a valid SUDO_UID; the
// existence-check seam is wired in a later phase of this task. This test only
// reads the process-wide default policy; it does not modify it, so it must
// not run in parallel with tests that do.
func TestRecordDeclaresSudoUIDAwarePolicy(t *testing.T) {
	require.Equal(t, groupmembership.SudoUIDAware, groupmembership.ProcessPermissionCheckUIDPolicy())

	deps := groupmembership.NewPermissionCheckUIDDepsForTesting()
	deps.Getenv = func(string) string { return "1000" }

	uid, err := groupmembership.ResolvePermissionCheckUID(
		groupmembership.ProcessPermissionCheckUIDPolicy(), 0, deps)
	require.NoError(t, err)
	assert.Equal(t, 1000, uid)
}

// TestRunTOCTOU_HashDirNotCreatedOnViolation verifies that a permission
// violation leaves the filesystem untouched: a hash directory that did not
// exist before the run does not exist after it. The directory's absence
// alone would not distinguish this from a failed creation, so the exit code is
// asserted too.
func TestRunTOCTOU_HashDirNotCreatedOnViolation(t *testing.T) {
	base := tu.SafeTempDir(t)
	hashDir := filepath.Join(base, "hashes")
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))
	require.NoDirExists(t, hashDir, "the hash directory must be absent before the run")

	d := defaultDeps()
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(_ string) error {
		return errors.New("world-writable directory")
	}})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "permission violation")
	assert.NoDirExists(t, hashDir, "no hash directory must be created when the check fails")
}

// TestRun_CreatesHashDirAfterPermissionCheckPasses verifies that a clean check
// lets the run create the hash directory and record hashes as before.
func TestRun_CreatesHashDirAfterPermissionCheckPasses(t *testing.T) {
	base := tu.SafeTempDir(t)
	hashDir := filepath.Join(base, "hashes")
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := defaultDeps()
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(_ string) error { return nil }})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())
	assert.DirExists(t, hashDir)
	assert.Contains(t, stdout.String(), "OK", "a hash record must still be generated")

	recordPath := extractHashFilePathFromStdout(t, stdout.String())
	assert.FileExists(t, recordPath)
}

// TestRunTOCTOU_ChecksAncestorsWhenHashDirMissing verifies that a hash
// directory that does not exist yet does not cause the permission check to be
// skipped: the ancestors it would be created under are checked instead.
func TestRunTOCTOU_ChecksAncestorsWhenHashDirMissing(t *testing.T) {
	base := tu.SafeTempDir(t)
	hashDir := filepath.Join(base, "hashes")
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	var checked []string
	d := defaultDeps()
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(path string) error {
		checked = append(checked, path)
		return nil
	}})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())
	assert.Contains(t, checked, hashDir, "the missing hash directory itself is collected")
	assert.Contains(t, checked, base, "the existing ancestor the hash directory would be created under must be checked")
}

// TestRunTOCTOU_ReportsViolationBehindSymlinkedAncestor verifies that the
// directories checked for a not-yet-existing hash directory are the ones on the
// far side of a symlinked ancestor, not the ones the link sits in. The fake
// checker only rejects the real path, so an implementation
// that appended the missing components lexically would never see it and the run
// would succeed.
func TestRunTOCTOU_ReportsViolationBehindSymlinkedAncestor(t *testing.T) {
	base := tu.SafeTempDir(t)
	realDir := filepath.Join(base, "real")
	nested := filepath.Join(realDir, "nested")
	require.NoError(t, os.MkdirAll(nested, 0o700))
	link := filepath.Join(base, "link")
	require.NoError(t, os.Symlink(realDir, link))

	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	hashDir := filepath.Join(link, "nested", "hashes")

	d := defaultDeps()
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(path string) error {
		if path == nested {
			return fmt.Errorf("%w: world-writable directory: %s", security.ErrInvalidDirPermissions, path)
		}
		return nil
	}})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "permission violation")
	assert.NoDirExists(t, filepath.Join(nested, "hashes"))
}

// TestRun_ReportsHashDirCreationFailure verifies that a failure to create the
// hash directory exits non-zero and says so on stderr.
func TestRun_ReportsHashDirCreationFailure(t *testing.T) {
	base := tu.SafeTempDir(t)
	hashDir := filepath.Join(base, "hashes")
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := defaultDeps()
	d.mkdirAll = func(_ string, _ os.FileMode) error { return errors.New("disk is full") }

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), errEnsureHashDir.Error())
	assert.Contains(t, stderr.String(), "disk is full")
	assert.NotContains(t, stdout.String(), "OK", "no hash record must be generated")
}

// TestRun_RejectsHashDirCreationUnderStickyWorldWritableParent verifies that a
// hash directory that would be created in a sticky world-writable directory is
// refused. The sticky bit protects entries that already exist, not a name
// nobody has claimed, so the shared check's sticky exception must not carry
// over to a directory record is about to create.
func TestRun_RejectsHashDirCreationUnderStickyWorldWritableParent(t *testing.T) {
	base := tu.SafeTempDir(t)
	// t.TempDir is 0o700, so the sticky world-writable directory has to be made
	// here; without it the creation site would be safe and the test would pass
	// for the wrong reason.
	parent := filepath.Join(base, "shared")
	require.NoError(t, os.Mkdir(parent, 0o700))
	require.NoError(t, os.Chmod(parent, os.ModeSticky|0o777))

	hashDir := filepath.Join(parent, "hashes")
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, defaultDeps(), stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "world-writable")
	assert.Contains(t, stderr.String(), "before running record", "stderr must say how to proceed")
	assert.NoDirExists(t, hashDir)
	assert.NotContains(t, stdout.String(), "OK")
}

// TestRun_AllowsExistingHashDirUnderStickyWorldWritableParent is the pair of the
// test above: the same layout passes once the hash directory exists, which is
// what confines the rule to the case where record would create it.
func TestRun_AllowsExistingHashDirUnderStickyWorldWritableParent(t *testing.T) {
	base := tu.SafeTempDir(t)
	parent := filepath.Join(base, "shared")
	require.NoError(t, os.Mkdir(parent, 0o700))
	require.NoError(t, os.Chmod(parent, os.ModeSticky|0o777))

	hashDir := filepath.Join(parent, "hashes")
	require.NoError(t, os.Mkdir(hashDir, 0o700))
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, defaultDeps(), stdout, stderr)

	require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "OK")
}

// TestRun_RejectsExistingStickyWorldWritableHashDir verifies that the same rule
// applies to the hash directory itself, not only to the directory it would be
// created in: a hash record for a file that record has not processed yet is a name
// nobody has claimed, so anyone able to write to the hash directory can
// pre-plant one and have verify trust it. The sticky bit protects the entries
// already in the directory and says nothing about that name.
//
// The directory is made sticky deliberately. Without the sticky bit the shared
// permission check rejects a world-writable directory on its own, and the test
// would pass without record's rule existing at all; with it, the shared check
// passes the directory (asserted below) and only record's own rule can produce
// the refusal, which is what the message assertions pin.
func TestRun_RejectsExistingStickyWorldWritableHashDir(t *testing.T) {
	base := tu.SafeTempDir(t)
	hashDir := filepath.Join(base, "hashes")
	require.NoError(t, os.Mkdir(hashDir, 0o700))
	require.NoError(t, os.Chmod(hashDir, os.ModeSticky|0o777))
	t.Cleanup(func() { _ = os.Chmod(hashDir, 0o700) })

	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	// The other layer, alone, leaves this directory untouched.
	checker, err := security.NewDirectoryPermChecker()
	require.NoError(t, err)
	require.NoError(t, checker.ValidateDirectoryPermissions(hashDir),
		"the shared check must accept this directory, or the refusal below proves nothing about record's own rule")

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, defaultDeps(), stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "is world-writable")
	assert.Contains(t, stderr.String(), "pre-plant")
	assert.NotContains(t, stderr.String(), "permission violation in hash directory",
		"the refusal must come from record's rule, not from the shared check")
	assert.NotContains(t, stdout.String(), "OK")

	entries, err := os.ReadDir(hashDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no hash record must be written into a world-writable hash directory")
}

// TestRun_CreatesHashDirBeforeSubdirectories verifies the ordering the hash
// directory's mode depends on: everything built underneath it uses os.MkdirAll
// with a wider mode of its own, so had one of those run first it would have
// created the hash directory on the way and left it at 0o755.
//
// The ordering is asserted at the moment of creation rather than by reading the
// resulting mode alone, because a restrictive umask would mask a subdirectory
// builder's 0o755 down to 0o700 and quietly disarm that check.
func TestRun_CreatesHashDirBeforeSubdirectories(t *testing.T) {
	base := tu.SafeTempDir(t)
	hashDir := filepath.Join(base, "hashes")
	cacheDir := filepath.Join(hashDir, libcCacheSubDir)
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	sawHashDirCreation := false
	d := defaultDeps()
	d.mkdirAll = func(path string, perm os.FileMode) error {
		if path == hashDir {
			sawHashDirCreation = true
			assert.NoDirExists(t, cacheDir, "the hash directory must be created before anything underneath it")
		}
		return os.MkdirAll(path, perm)
	}

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)
	require.Equal(t, 0, exitCode, "stderr: %s", stderr.String())

	require.True(t, sawHashDirCreation, "run must create the hash directory itself")
	require.DirExists(t, cacheDir, "the libc cache subdirectory must have been built under the hash directory")

	info, err := os.Stat(hashDir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), info.Mode().Perm(), "the hash directory must keep its own mode, not inherit a subdirectory's")
}

// TestRun_RefusesUnresolvableHashDirPath covers a hash directory path that
// resolution refuses: a ".." in the part of the path that does not exist yet.
// Resolution hands back the deepest existing ancestor for that failure, so
// treating it as a warning would check one directory's permissions and then
// create the hash directory in a completely different tree -- including trees
// the check would have rejected outright.
func TestRun_RefusesUnresolvableHashDirPath(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
	}{
		{name: "world_writable_parent", mode: 0o777},
		{name: "sticky_world_writable_parent", mode: os.ModeSticky | 0o777},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := tu.SafeTempDir(t)
			elsewhere := filepath.Join(base, "elsewhere")
			require.NoError(t, os.Mkdir(elsewhere, 0o700))
			require.NoError(t, os.Chmod(elsewhere, tc.mode))

			targetFile := filepath.Join(base, "target.txt")
			require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

			// Written literally rather than with filepath.Join, which would clean the
			// ".." away before record ever sees it.
			hashDir := base + "/gone/../elsewhere/hashes"

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			exitCode := run([]string{"-d", hashDir, targetFile}, defaultDeps(), stdout, stderr)

			require.Equal(t, 1, exitCode)
			assert.Contains(t, stderr.String(), "cannot resolve hash directory")
			assert.NoDirExists(t, filepath.Join(elsewhere, "hashes"), "nothing must be created when the path could not be resolved")
			assert.NotContains(t, stdout.String(), "OK")
		})
	}
}

// TestCheckHashDirWriteSafety_RefusesWhenSiteIsUnusable covers the refusals
// that guard the write itself. Each is reachable in production only through a
// race against the resolution done moments earlier, so they are driven directly
// here rather than through run.
func TestCheckHashDirWriteSafety_RefusesWhenSiteIsUnusable(t *testing.T) {
	newLogger := func(buf *bytes.Buffer) *slog.Logger {
		return slog.New(slog.NewTextHandler(buf, nil))
	}

	t.Run("relative path has no determinable creation site", func(t *testing.T) {
		logs := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		assert.False(t, checkHashDirWriteSafety("relative/hashes", newLogger(logs), stderr))
		assert.Contains(t, stderr.String(), "cannot determine where hash directory")
	})

	t.Run("dangling symlink ancestor is not a usable creation site", func(t *testing.T) {
		// The deepest existing ancestor is the link itself, since existence is
		// tested with Lstat so that a link to nowhere surfaces instead of the tree
		// the link happens to sit in being checked in its place.
		base := tu.SafeTempDir(t)
		link := filepath.Join(base, "link")
		require.NoError(t, os.Symlink(filepath.Join(base, "missing"), link))

		logs := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		assert.False(t, checkHashDirWriteSafety(filepath.Join(link, "hashes"), newLogger(logs), stderr))
		assert.Contains(t, stderr.String(), "which is not a directory")
	})

	t.Run("unreadable ancestor leaves existence unknown", func(t *testing.T) {
		if syscall.Geteuid() == 0 {
			t.Skip("skipping: root is not denied by chmod 0o000")
		}
		base := tu.SafeTempDir(t)
		closed := filepath.Join(base, "closed")
		require.NoError(t, os.Mkdir(closed, 0o700))
		require.NoError(t, os.Chmod(closed, 0o000))
		t.Cleanup(func() { _ = os.Chmod(closed, 0o700) })

		logs := &bytes.Buffer{}
		stderr := &bytes.Buffer{}

		assert.False(t, checkHashDirWriteSafety(filepath.Join(closed, "hashes"), newLogger(logs), stderr))
		assert.Contains(t, stderr.String(), "cannot determine whether hash directory")
	})
}

// TestRun_ExitsWithoutPanicWhenCheckerInitFails verifies that a permission
// checker that cannot be built ends the run with exit code 1 and an error on
// stderr rather than a panic.
//
// run is called in this process, so a panic would take the test binary down
// with it: the substantive evidence that none happened is that run returned a
// value at all. The assertion on "goroutine " is a secondary check that no
// stack trace reached the user's stderr.
func TestRun_ExitsWithoutPanicWhenCheckerInitFails(t *testing.T) {
	base := tu.SafeTempDir(t)
	hashDir := filepath.Join(base, "hashes")
	targetFile := filepath.Join(base, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	d := defaultDeps()
	d.newPermChecker = newFailingPermChecker(errors.New("checker setup failed"))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-d", hashDir, targetFile}, d, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "checker setup failed")
	assert.NotContains(t, stderr.String(), "goroutine ", "no stack trace must reach stderr")
	assert.NoDirExists(t, hashDir, "nothing must be created once the checker cannot be built")
}
