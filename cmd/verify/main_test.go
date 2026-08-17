// Dependencies are injected per call through deps, so tests do not touch
// process-wide state for those. The slog default logger is the exception: it is
// process-wide, and tests that capture it (captureLogs) or assert on what was
// logged must not call t.Parallel().
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

// testDeps returns the production dependencies with validator installed in place
// of the real one, recording the hash directory it was asked to build for. Tests
// needing a stub permission checker overwrite newPermChecker on the result.
func testDeps(validator *fakeValidator) deps {
	d := defaultDeps()
	d.validatorFactory = func(hashDir string) (hashValidator, error) {
		validator.hashDir = hashDir
		return validator, nil
	}
	return d
}

// fixedPermChecker adapts a ready-made checker to the constructor seam deps
// exposes, so that tests inject through the same field production uses.
// cmd/record/main_test.go defines an identical helper; as with
// fakeDirPermChecker above, it is unexported in a different main package and so
// cannot be shared.
func fixedPermChecker(checker security.DirectoryPermChecker) func() (security.DirectoryPermChecker, error) {
	return func() (security.DirectoryPermChecker, error) { return checker, nil }
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

	exitCode := run([]string{}, defaultDeps(), stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), "at least one file path")
}

func TestRunProcessesMultipleFiles(t *testing.T) {
	tempDir := t.TempDir()
	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)
	d.newPermChecker = fixedPermChecker(allowAllDirs())

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", tempDir, "file1.txt", "file2.txt"}, d, stdout, stderr)

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
	d := testDeps(validator)
	d.newPermChecker = fixedPermChecker(allowAllDirs())

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", tempDir, "good.txt", "bad.txt", "later.txt"}, d, stdout, stderr)

	require.Equal(t, 1, exitCode)
	require.Len(t, validator.calls, 3)
	assert.Contains(t, stdout.String(), "[2/3] bad.txt: FAILED")
	assert.Contains(t, stdout.String(), "Summary: 2 succeeded, 1 failed")
	assert.Contains(t, stderr.String(), "Verification failed for bad.txt")
}

func TestRunWarnsWhenDeprecatedFlagUsed(t *testing.T) {
	tempDir := t.TempDir()
	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)
	d.newPermChecker = fixedPermChecker(allowAllDirs())

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", tempDir, "-file", "legacy.txt", "new.txt"}, d, stdout, stderr)

	require.Equal(t, 0, exitCode)
	require.Len(t, validator.calls, 2)
	assert.Equal(t, "legacy.txt", validator.calls[0].file)
	assert.Contains(t, stderr.String(), "deprecated")
}

func TestRunUsesDefaultHashDirectoryWhenNotSpecified(t *testing.T) {
	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)
	d.newPermChecker = fixedPermChecker(allowAllDirs())

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"file1.txt"}, d, stdout, stderr)

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

	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)
	// Report a violation only for the target file's own parent directory. The
	// hash directory and every ancestor stay clean regardless of how the host's
	// filesystem is configured.
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(path string) error {
		if path == worldWritableDir {
			return fmt.Errorf("%w: directory %s is writable by others", security.ErrInvalidDirPermissions, path)
		}
		return nil
	}})

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, targetFile}, d, stdout, stderr)

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
	hashDir := tu.SafeTempDir(t)
	require.NoError(t, os.Chmod(hashDir, 0o777))
	t.Cleanup(func() {
		_ = os.Chmod(hashDir, 0o755)
	})

	validator := &fakeValidator{responses: map[string]error{}}
	// The real checker, from defaultDeps: this test is only meaningful against it.
	d := testDeps(validator)

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, "file1.txt"}, d, stdout, stderr)

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
	hashDir := tu.SafeTempDir(t)
	targetDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(targetDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, targetFile}, d, stdout, stderr)

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
	badAncestor := tu.SafeTempDir(t)
	hashDir := filepath.Join(badAncestor, "a", "b", "hashes")
	require.NoError(t, os.MkdirAll(hashDir, 0o755))
	require.NoError(t, os.Chmod(badAncestor, 0o777))
	t.Cleanup(func() {
		_ = os.Chmod(badAncestor, 0o755)
	})

	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, "file1.txt"}, d, stdout, stderr)

	require.Equal(t, exitUntrustedEnvironment, exitCode)
	assert.Empty(t, validator.calls)
	assert.Contains(t, logs.String(), badAncestor, "the violation must name the directory that is actually world-writable")

	// The remediation must not name the checked path: hashDir's own permissions
	// are correct, so any instruction naming it sends the operator to fix a
	// directory that is already fine.
	var remediations int
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		if !strings.Contains(line, "level=ERROR") {
			continue
		}
		_, remediation, found := strings.Cut(line, "remediation=")
		require.True(t, found, "every fail-closed ERROR line carries a remediation: %s", line)
		assert.NotContains(t, remediation, hashDir,
			"the remediation must point at the directory named in the violation, not at the checked path")
		remediations++
	}
	require.NotZero(t, remediations, "at least one fail-closed ERROR line must have been logged")
}

// TestRunFailsClosedOnHashDirViolation_DefaultHashDir verifies that the same
// fail-closed decision is reached when the hash directory comes from the default
// rather than from -hash-dir.
func TestRunFailsClosedOnHashDirViolation_DefaultHashDir(t *testing.T) {
	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(path string) error {
		return fmt.Errorf("%w: directory %s is writable by others", security.ErrInvalidDirPermissions, path)
	}})

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"file1.txt"}, d, stdout, stderr)

	require.Equal(t, exitUntrustedEnvironment, exitCode)
	assert.Empty(t, validator.calls, "no file must be verified once the hash directory cannot be trusted")
}

// TestRunFailsClosedOnHashDirViolation_LogsErrorLevel verifies that each
// fail-closed violation is logged at ERROR level with the path and the
// remediation, in addition to (not instead of) the WARN line the shared check
// emits.
func TestRunFailsClosedOnHashDirViolation_LogsErrorLevel(t *testing.T) {
	hashDir := tu.SafeTempDir(t)

	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(path string) error {
		if path == hashDir {
			return fmt.Errorf("%w: directory %s is writable by others", security.ErrInvalidDirPermissions, path)
		}
		return nil
	}})

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, "file1.txt"}, d, stdout, stderr)
	require.Equal(t, exitUntrustedEnvironment, exitCode)

	var errorLines, warnLines []string
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
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

	validator := &fakeValidator{responses: map[string]error{}}
	d := testDeps(validator)
	// Violations on both sides: only the hash directory side may be reported.
	d.newPermChecker = fixedPermChecker(&fakeDirPermChecker{validateDirFn: func(path string) error {
		if path == hashDir || path == targetDir {
			return fmt.Errorf("%w: directory %s is writable by others", security.ErrInvalidDirPermissions, path)
		}
		return nil
	}})

	logs := captureLogs(t)
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-hash-dir", hashDir, targetFile}, d, stdout, stderr)

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
	d := testDeps(validator)
	d.ensurePermissionCheckUID = func() error { return groupmembership.ErrSudoUIDUserLookupFailed }

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	exitCode := run([]string{"-d", tempDir, "file1.txt", "file2.txt"}, d, stdout, stderr)

	require.Equal(t, 1, exitCode)
	assert.Contains(t, stderr.String(), groupmembership.ErrSudoUIDUserLookupFailed.Error())
	assert.Empty(t, validator.calls, "no file must be verified once the UID cannot be resolved")
	assert.Empty(t, stdout.String())
}

// TestRunCreatesNoFilesystemEntries verifies that verify creates nothing on
// disk, whether the hash directory it is given exists or not. The production
// dependencies are used unchanged — a stub validator would hide the creation
// that happens inside the validator itself, which is the half a deleted mkdirAll
// call does not cover.
//
// Only the missing_hash_dir case can fail today: both creation paths this
// commit removed created the hash directory itself, so restoring either leaves
// an existing one untouched. The existing_hash_dir case is a standing guard for
// a write placed inside a hash directory that is already there, which is why the
// whole parent subtree is compared rather than only its top level.
func TestRunCreatesNoFilesystemEntries(t *testing.T) {
	tests := []struct {
		name          string
		hashDirExists bool
	}{
		{name: "existing_hash_dir", hashDirExists: true},
		{name: "missing_hash_dir", hashDirExists: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parent := tu.SafeTempDir(t)
			hashDir := filepath.Join(parent, "hashes")
			if tc.hashDirExists {
				require.NoError(t, os.Mkdir(hashDir, 0o700))
			}
			targetDir := tu.SafeTempDir(t)
			targetFile := filepath.Join(targetDir, "target.txt")
			require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

			before := tu.WalkEntries(t, parent)

			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			exitCode := run([]string{"-hash-dir", hashDir, targetFile}, defaultDeps(), stdout, stderr)

			// Every early return of run also creates nothing, so the subtree
			// assertion below proves nothing on its own. These two say the run
			// reached the validator and verified the file, which is where the
			// creation this test denies would have happened.
			require.Equal(t, exitVerificationFailed, exitCode, "no hash record exists, so verification must fail: %s", stderr.String())
			assert.Contains(t, stdout.String(), "[1/1] "+targetFile, "the run must have reached per-file verification")

			after := tu.WalkEntries(t, parent)
			assert.Equal(t, before, after, "verify must not create anything under the hash directory's parent")
		})
	}
}

// TestRunResolvesMissingHashDirUnderSymlinkedAncestor covers the combination
// that stopping the hash directory creation made reachable: the directory does
// not exist and one of its ancestors is a symlink. Resolving with
// filepath.EvalSymlinks fails outright on an absent path, and the unresolved
// path left behind makes the Lstat-based hierarchy check reject the symlinked
// ancestor as "not a directory" — a fail-closed exit whose remediation tells
// the operator to fix permissions on a directory that is fine.
//
// The hash directory is reached through the symlink deliberately; tu.SafeTempDir
// resolves symlinks, so the other tests in this file cannot reach this path.
func TestRunResolvesMissingHashDirUnderSymlinkedAncestor(t *testing.T) {
	root := tu.SafeTempDir(t)
	realDir := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(realDir, 0o700))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(realDir, link))
	// Absent, and named through the symlink.
	hashDir := filepath.Join(link, "hashes")

	targetDir := tu.SafeTempDir(t)
	targetFile := filepath.Join(targetDir, "target.txt")
	require.NoError(t, os.WriteFile(targetFile, []byte("hello"), 0o644))

	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := run([]string{"-hash-dir", hashDir, targetFile}, defaultDeps(), stdout, stderr)

	require.Equal(t, exitVerificationFailed, exitCode,
		"a missing hash directory is an ordinary verification failure, not an untrusted environment: %s", stderr.String())
	assert.NotContains(t, stderr.String(), "permission violation in hash directory",
		"the symlinked ancestor must not be reported as a permission violation")
	assert.Contains(t, stdout.String(), "[1/1] "+targetFile, "the run must have reached per-file verification")
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

	uidDeps := groupmembership.NewPermissionCheckUIDDepsForTesting()
	uidDeps.Getenv = func(string) string { return "1000" }

	uid, err := groupmembership.ResolvePermissionCheckUID(
		groupmembership.ProcessPermissionCheckUIDPolicy(), 0, uidDeps)
	require.NoError(t, err)
	assert.Equal(t, 1000, uid)
}
