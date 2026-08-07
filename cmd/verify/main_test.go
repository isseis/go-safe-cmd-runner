// Tests in this file replace package-level variables (validatorFactory,
// mkdirAll, ensurePermissionCheckUID, toctouChecker) and the slog default
// logger, all of which are process-wide state. None of them may call
// t.Parallel().
package main

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/security"
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

// fakeDirPermChecker implements security.DirectoryPermChecker for testing.
// cmd/record/main_test.go defines an identical stub; it cannot be shared because
// it is an unexported type in a different main package, so this duplication is
// deliberate.
type fakeDirPermChecker struct {
	validateDirFn func(path string) error
}

func (f *fakeDirPermChecker) ValidateDirectoryPermissions(path string) error {
	return f.validateDirFn(path)
}

// overrideTOCTOUChecker installs checker as the directory permission checker for
// the duration of the test, restoring the previous value afterwards.
func overrideTOCTOUChecker(t *testing.T, checker security.DirectoryPermChecker) {
	t.Helper()
	original := toctouChecker
	toctouChecker = checker
	t.Cleanup(func() { toctouChecker = original })
}

// allowAllDirs returns a checker that reports no violation for any directory, so
// that a test exercising other behaviour does not depend on the permissions of
// the host's real directories.
func allowAllDirs() security.DirectoryPermChecker {
	return &fakeDirPermChecker{validateDirFn: func(string) error { return nil }}
}

// captureLogs redirects the default slog logger into a buffer for the duration
// of the test and returns the buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return buf
}

func TestRunRequiresAtLeastOneFile(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{}, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "at least one file path")
}

func TestRunProcessesMultipleFiles(t *testing.T) {
	overrideTOCTOUChecker(t, allowAllDirs())
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
	overrideTOCTOUChecker(t, allowAllDirs())
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
	overrideTOCTOUChecker(t, allowAllDirs())
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
	overrideTOCTOUChecker(t, allowAllDirs())
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

// TestRunTOCTOU_ContinuesWhenOnlyTargetDirViolates verifies that a violation
// confined to a target file's ancestor directories is not fail-closed. Only the
// hash directory is the root of trust; a target file sitting in a writable
// directory is precisely what verify exists to inspect, so verification
// continues and the violation stays a warning.
func TestRunTOCTOU_ContinuesWhenOnlyTargetDirViolates(t *testing.T) {
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
	// Report a violation only for the target file's own parent directory. The
	// hash directory and every ancestor stay clean regardless of how the host's
	// filesystem is configured.
	overrideTOCTOUChecker(t, &fakeDirPermChecker{validateDirFn: func(path string) error {
		if path == worldWritableDir {
			return fmt.Errorf("%w: directory %s is writable by others", security.ErrInvalidDirPermissions, path)
		}
		return nil
	}})

	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, targetFile}, stdout, stderr)

	assert.Equal(t, exitOK, exitCode, "verify should continue (exit 0) despite world-writable target directory")
	assert.NotEqual(t, exitUntrustedEnvironment, exitCode, "a target-side violation must not be fail-closed")
	require.Len(t, validator.calls, 1, "file should have been processed")
	assert.Contains(t, logs.String(), "level=WARN", "the target-side violation stays a warning")
	assert.Contains(t, logs.String(), worldWritableDir)
	assert.NotContains(t, logs.String(), "level=ERROR")
}

// TestRunFailsClosedOnHashDirViolation_ExplicitHashDir verifies the fail-closed
// path with the real permission checker and a hash directory this test made
// world-writable itself, so the assertion does not depend on the permissions of
// any pre-existing host directory.
func TestRunFailsClosedOnHashDirViolation_ExplicitHashDir(t *testing.T) {
	// Stated explicitly rather than relying on no other test having leaked a
	// stub: this test is only meaningful against the real checker.
	overrideTOCTOUChecker(t, nil)

	hashDir := tu.SafeTempDir(t)
	require.NoError(t, os.Chmod(hashDir, 0o777))
	t.Cleanup(func() {
		_ = os.Chmod(hashDir, 0o755)
	})

	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, "file1.txt"}, stdout, stderr)

	require.Equal(t, exitUntrustedEnvironment, exitCode)
	assert.Empty(t, validator.calls, "no file must be verified once the hash directory cannot be trusted")
	assert.Empty(t, stdout.String(), "verification must not start, so no progress output")
	assert.Contains(t, stderr.String(), "verification results cannot be trusted")
	assert.Contains(t, stderr.String(), "Fix directory permissions")
}

// TestRunProceedsWithRealCheckerOnCleanDirs is the canary for the real checker's
// happy path. Every other test that reaches checkDirPermissions either injects a
// stub or asserts the fail-closed outcome, so without this one a regression that
// made the real check report spurious violations — an unresolvable path falling
// back to a relative one, say — would turn every real invocation of verify into
// exit 3 with nothing verified, and the suite would stay green. Both directories
// are created by this test, so it does not depend on the host's layout.
func TestRunProceedsWithRealCheckerOnCleanDirs(t *testing.T) {
	overrideTOCTOUChecker(t, nil)

	hashDir := tu.SafeTempDir(t)
	targetDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(targetDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, targetFile}, stdout, stderr)

	require.Equal(t, exitOK, exitCode, "logs: %s", logs.String())
	require.Len(t, validator.calls, 1)
	assert.NotContains(t, logs.String(), "level=ERROR")
	assert.NotContains(t, stderr.String(), "cannot be trusted")
}

// TestRunFailsClosedOnHashDirViolation_AncestorViolation drives the real checker
// with a clean hash directory nested under a world-writable ancestor. The
// checker validates every component from the root down, so the violation is
// attributed to a path whose own permissions are fine; this pins that the
// remediation points at the directory named in the violation rather than at the
// checked path, which would send the operator to chmod a correct directory.
func TestRunFailsClosedOnHashDirViolation_AncestorViolation(t *testing.T) {
	overrideTOCTOUChecker(t, nil)

	badAncestor := tu.SafeTempDir(t)
	hashDir := filepath.Join(badAncestor, "a", "b", "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	require.NoError(t, os.Chmod(badAncestor, 0o777))
	t.Cleanup(func() {
		_ = os.Chmod(badAncestor, 0o755)
	})

	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, "file1.txt"}, stdout, stderr)

	require.Equal(t, exitUntrustedEnvironment, exitCode)
	assert.Empty(t, validator.calls)
	assert.Contains(t, logs.String(), badAncestor, "the violation must name the directory that is actually world-writable")
	assert.NotContains(t, logs.String(), "chmod go-w "+hashDir,
		"the remediation must not tell the operator to chmod the hash directory, whose own permissions are correct")
}

// TestRunFailsClosedOnHashDirViolation_DefaultHashDir verifies that the same
// fail-closed decision is reached when the hash directory comes from the default
// rather than from -hash-dir.
func TestRunFailsClosedOnHashDirViolation_DefaultHashDir(t *testing.T) {
	overrideTOCTOUChecker(t, &fakeDirPermChecker{validateDirFn: func(path string) error {
		return fmt.Errorf("%w: directory %s is writable by others", security.ErrInvalidDirPermissions, path)
	}})

	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	originalMkdirAll := mkdirAll
	mkdirAll = func(string, os.FileMode) error { return nil }
	t.Cleanup(func() { mkdirAll = originalMkdirAll })

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"file1.txt"}, stdout, stderr)

	require.Equal(t, exitUntrustedEnvironment, exitCode)
	assert.Empty(t, validator.calls, "no file must be verified once the hash directory cannot be trusted")
}

// TestRunFailsClosedOnHashDirViolation_LogsErrorLevel verifies that each
// fail-closed violation is logged at ERROR level with the path and the
// remediation, in addition to (not instead of) the WARN line the shared check
// emits.
func TestRunFailsClosedOnHashDirViolation_LogsErrorLevel(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	overrideTOCTOUChecker(t, &fakeDirPermChecker{validateDirFn: func(path string) error {
		if path == hashDir {
			return fmt.Errorf("%w: directory %s is writable by others", security.ErrInvalidDirPermissions, path)
		}
		return nil
	}})

	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, "file1.txt"}, stdout, stderr)
	require.Equal(t, exitUntrustedEnvironment, exitCode)

	var errorLines, warnLines []string
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		switch {
		case strings.Contains(line, "level=ERROR"):
			errorLines = append(errorLines, line)
		case strings.Contains(line, "level=WARN"):
			warnLines = append(warnLines, line)
		}
	}

	require.Len(t, errorLines, 1, "one ERROR line per fail-closed violation")
	assert.Contains(t, errorLines[0], "path="+hashDir)
	assert.Contains(t, errorLines[0], "remediation=")
	require.Len(t, warnLines, 1, "the shared check's WARN line must remain alongside the ERROR line")
	assert.Contains(t, warnLines[0], "TOCTOU permission check violation")
}

// TestRunSkipsTargetSetCheckWhenHashDirViolates verifies the side-effect
// contract: once the hash directory side is fail-closed, the target file set is
// never checked, so no violation is reported for it even though the injected
// checker would report one.
func TestRunSkipsTargetSetCheckWhenHashDirViolates(t *testing.T) {
	hashDir := tu.SafeTempDir(t)
	targetDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(targetDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	// Violations on both sides: only the hash directory side may be reported.
	overrideTOCTOUChecker(t, &fakeDirPermChecker{validateDirFn: func(path string) error {
		if path == hashDir || path == targetDir {
			return fmt.Errorf("%w: directory %s is writable by others", security.ErrInvalidDirPermissions, path)
		}
		return nil
	}})

	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, targetFile}, stdout, stderr)

	require.Equal(t, exitUntrustedEnvironment, exitCode)
	assert.NotContains(t, logs.String(), targetDir, "the target file set must not be checked once the hash directory side fails closed")
	assert.Contains(t, logs.String(), hashDir)
}

// TestRunFailsClosedWhenPermissionCheckUIDUnresolvable verifies that verify
// resolves the permission check UID before it touches any file, so that an
// unverifiable SUDO_UID stops the run once at startup rather than producing a
// failure per file.
func TestRunFailsClosedWhenPermissionCheckUIDUnresolvable(t *testing.T) {
	tempDir := t.TempDir()
	validator := &fakeValidator{responses: map[string]error{}}
	cleanup := overrideValidatorFactory(t, validator)
	defer cleanup()

	original := ensurePermissionCheckUID
	ensurePermissionCheckUID = func() error { return groupmembership.ErrSudoUIDUserLookupFailed }
	t.Cleanup(func() { ensurePermissionCheckUID = original })

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", tempDir, "file1.txt", "file2.txt"}, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), groupmembership.ErrSudoUIDUserLookupFailed.Error())
	assert.Empty(t, validator.calls, "no file must be verified once the UID cannot be resolved")
	assert.Empty(t, stdout.String())
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
