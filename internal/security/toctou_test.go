//go:build test

package security

import (
	"bytes"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
)

// TestCollectTOCTOUCheckDirs verifies that directory collection covers all required
// sources and performs deduplication.
func TestCollectTOCTOUCheckDirs(t *testing.T) {
	tests := []struct {
		name         string
		verifyFiles  []string
		commandPaths []string
		hashDir      string
		wantDirs     []string // sorted expected set (subset check)
		wantNotEmpty bool
	}{
		{
			name:         "empty inputs returns empty",
			verifyFiles:  []string{},
			commandPaths: []string{},
			hashDir:      "",
			wantDirs:     []string{},
		},
		{
			name:         "verify_files parent directories and all ancestors are included",
			verifyFiles:  []string{"/usr/bin/python3", "/etc/hosts"},
			commandPaths: []string{},
			hashDir:      "",
			wantDirs:     []string{"/", "/etc", "/usr", "/usr/bin"},
		},
		{
			name:         "command paths parent directories and all ancestors are included",
			verifyFiles:  []string{},
			commandPaths: []string{"/usr/bin/echo", "/usr/local/bin/tool"},
			hashDir:      "",
			wantDirs:     []string{"/", "/usr", "/usr/bin", "/usr/local", "/usr/local/bin"},
		},
		{
			name:         "hashDir itself is included with ancestors",
			verifyFiles:  []string{},
			commandPaths: []string{},
			hashDir:      "/var/lib/hashes",
			wantDirs:     []string{"/", "/var", "/var/lib", "/var/lib/hashes"},
		},
		{
			name:         "duplicates are removed",
			verifyFiles:  []string{"/usr/bin/python3", "/usr/bin/python2"},
			commandPaths: []string{"/usr/bin/echo"},
			hashDir:      "",
			wantDirs:     []string{"/", "/usr", "/usr/bin"},
		},
		{
			name:         "combined sources without duplicates",
			verifyFiles:  []string{"/usr/bin/python3"},
			commandPaths: []string{"/usr/bin/echo"},
			hashDir:      "/var/hashes",
			wantDirs:     []string{"/", "/usr", "/usr/bin", "/var", "/var/hashes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CollectTOCTOUCheckDirs(tt.verifyFiles, tt.commandPaths, tt.hashDir)

			// Sort both slices for comparison
			gotSet := make(map[string]struct{}, len(got))
			for _, d := range got {
				gotSet[d] = struct{}{}
			}

			// Verify all expected dirs are present
			sort.Strings(tt.wantDirs)
			for _, expected := range tt.wantDirs {
				assert.Contains(t, gotSet, expected, "expected directory %q not found in result", expected)
			}

			// Verify no duplicates
			assert.Equal(t, len(got), len(gotSet), "result should contain no duplicate directories")
		})
	}
}

// TestRunTOCTOUPermissionCheck_NoViolations verifies that clean directories
// produce no violations.
func TestRunTOCTOUPermissionCheck_NoViolations(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	err := os.Chmod(tmpDir, 0o755)
	require.NoError(t, err)

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := RunTOCTOUPermissionCheck(v, []string{tmpDir}, slog.Default())
	assert.Empty(t, result.Violations, "no violations expected for secure directory")
}

// TestRunTOCTOUPermissionCheck_ViolationDetected verifies that world-writable
// directories are detected as violations.
func TestRunTOCTOUPermissionCheck_ViolationDetected(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)

	// Make the directory world-writable (violates security policy)
	err := os.Chmod(tmpDir, 0o777)
	require.NoError(t, err)
	// Restore permissions after test so cleanup succeeds
	t.Cleanup(func() {
		_ = os.Chmod(tmpDir, 0o755)
	})

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := RunTOCTOUPermissionCheck(v, []string{tmpDir}, slog.Default())
	require.Len(t, result.Violations, 1, "expected exactly one violation for world-writable directory")
	assert.Equal(t, filepath.Clean(tmpDir), result.Violations[0].Path)
	assert.True(t, errors.Is(result.Violations[0].Err, ErrInvalidDirPermissions), "violation error should be about directory permissions")
}

// TestRunTOCTOUPermissionCheck_MultipleViolations verifies that multiple
// violations are all returned.
func TestRunTOCTOUPermissionCheck_MultipleViolations(t *testing.T) {
	dir1 := tu.SafeTempDir(t)
	dir2 := tu.SafeTempDir(t)

	for _, d := range []string{dir1, dir2} {
		err := os.Chmod(d, 0o777)
		require.NoError(t, err)
		dd := d
		t.Cleanup(func() {
			_ = os.Chmod(dd, 0o755)
		})
	}

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := RunTOCTOUPermissionCheck(v, []string{dir1, dir2}, slog.Default())
	assert.Len(t, result.Violations, 2, "expected two violations")
}

// TestRunTOCTOUPermissionCheck_EmptyDirs verifies that an empty directory list
// produces no violations.
func TestRunTOCTOUPermissionCheck_EmptyDirs(t *testing.T) {
	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := RunTOCTOUPermissionCheck(v, []string{}, slog.Default())
	assert.Empty(t, result.Violations)
}

// errGetwdFailed is the failure injected into the working-directory lookup.
var errGetwdFailed = errors.New("getwd unavailable")

// skipIfRoot skips a test that relies on a permission denial, because mode 0o000
// does not produce EACCES for root.
func skipIfRoot(t *testing.T) {
	t.Helper()
	if syscall.Geteuid() == 0 {
		t.Skip("running as root: permission denial cannot be reproduced")
	}
}

// blockedPathUnder creates dir/<name>/inner, removes all access to dir/<name> and
// returns a path below it that therefore cannot be stat'ed.
func blockedPathUnder(t *testing.T, dir, name string) string {
	t.Helper()
	blocked := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Join(blocked, "inner"), 0o700))
	require.NoError(t, os.Chmod(blocked, 0o000))
	// Restore access so t.TempDir cleanup can remove the tree.
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o700) })
	return filepath.Join(blocked, "inner", "target")
}

// newBufferLogger returns a logger writing to the returned buffer.
func newBufferLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})), buf
}

// TestResolvePathForCheck_FullyExistingPathResolvesSymlinks verifies that a path
// whose every component exists comes back as the real path behind the symlink.
func TestResolvePathForCheck_FullyExistingPathResolvesSymlinks(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	realDir := filepath.Join(tmpDir, "real")
	require.NoError(t, os.MkdirAll(filepath.Join(realDir, "sub"), 0o700))
	link := filepath.Join(tmpDir, "link")
	require.NoError(t, os.Symlink(realDir, link))

	got, err := ResolvePathForCheck(filepath.Join(link, "sub"))

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(realDir, "sub"), got)
}

// TestResolvePathForCheck_PartiallyExistingPath verifies that resolution stops at
// the deepest existing ancestor and joins the missing remainder lexically.
func TestResolvePathForCheck_PartiallyExistingPath(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	missing := filepath.Join(tmpDir, "not", "created")
	_, statErr := os.Lstat(filepath.Join(tmpDir, "not"))
	require.ErrorIs(t, statErr, fs.ErrNotExist, "the two trailing levels must be absent for this test to mean anything")

	got, err := ResolvePathForCheck(missing)

	require.NoError(t, err)
	assert.Equal(t, missing, got)
}

// TestResolvePathForCheck_SymlinkedAncestorOfMissingPath verifies that a path that
// does not exist yet is resolved through a symlinked ancestor, so the directories
// that will really contain it are the ones handed to the permission check.
func TestResolvePathForCheck_SymlinkedAncestorOfMissingPath(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	realDir := filepath.Join(tmpDir, "real")
	require.NoError(t, os.Mkdir(realDir, 0o700))
	link := filepath.Join(tmpDir, "link")
	require.NoError(t, os.Symlink(realDir, link))

	got, err := ResolvePathForCheck(filepath.Join(link, "missing", "deep"))

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(realDir, "missing", "deep"), got)
}

// TestResolvePathForCheck_UnreadableAncestorReturnsErrPathResolution verifies that a
// failure other than "does not exist" is reported while still yielding a checkable
// path, so the caller fails closed rather than skipping the path.
func TestResolvePathForCheck_UnreadableAncestorReturnsErrPathResolution(t *testing.T) {
	skipIfRoot(t)
	tmpDir := tu.SafeTempDir(t)
	target := blockedPathUnder(t, tmpDir, "blocked")

	// The redundant separator is carried through untouched: the failure path must not
	// clean the path either, since cleaning is what collapses ".." onto the wrong tree.
	raw := strings.Replace(target, "/inner/", "/inner//", 1)

	got, err := ResolvePathForCheck(raw)

	require.ErrorIs(t, err, ErrPathResolution)
	assert.Equal(t, raw, got, "the absolute path must still be returned, uncleaned")
}

// TestResolvePathForCheck_DotDotEscapingMissingComponentsIsRejected pins the one case
// a lexical join cannot handle: a ".." inside the not-yet-existing remainder pops back
// above the missing components onto a symlink no walk has resolved. Joining it would
// hand the check a path through that unresolved link and report success.
func TestResolvePathForCheck_DotDotEscapingMissingComponentsIsRejected(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	realDir := filepath.Join(tmpDir, "real")
	require.NoError(t, os.Mkdir(realDir, 0o700))
	require.NoError(t, os.Symlink(realDir, filepath.Join(tmpDir, "link")))

	// tmpDir/missing does not exist, so the walk stops at tmpDir and "missing/../link/x"
	// is the remainder. filepath.Join would collapse it to tmpDir/link/x.
	got, err := ResolvePathForCheck(tmpDir + "/missing/../link/x")

	require.ErrorIs(t, err, ErrPathResolution)
	assert.ErrorIs(t, err, ErrInvalidPath)
	assert.Equal(t, tmpDir, got, "the deepest existing ancestor is still checkable")
	assert.NotContains(t, got, "link", "the unresolved symlink must not appear in the checkable path")
}

// TestResolvePathForCheck_EmptyPathIsRejected verifies that "" is rejected rather than
// silently resolving to the process working directory. Callers use "" to mean "no path
// configured"; resolving it would turn a skip into a full check of an unrelated tree.
func TestResolvePathForCheck_EmptyPathIsRejected(t *testing.T) {
	got, err := ResolvePathForCheck("")

	require.ErrorIs(t, err, ErrInvalidPath)
	assert.Empty(t, got)
}

// TestResolvePathForCheck_RelativePathUsesWorkingDirectory verifies that a relative
// path is anchored to the process working directory.
func TestResolvePathForCheck_RelativePathUsesWorkingDirectory(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "child"), 0o700))
	t.Chdir(tmpDir)

	got, err := ResolvePathForCheck("child")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "child"), got)
}

// TestResolvePathForCheck_AbsConversionFailure covers the branch taken when the
// working directory cannot be read, which is reachable only through the test seam.
func TestResolvePathForCheck_AbsConversionFailure(t *testing.T) {
	original := getwdHook
	getwdHook = func() (string, error) { return "", errGetwdFailed }
	t.Cleanup(func() { getwdHook = original })

	got, err := ResolvePathForCheck("some/relative/path")

	require.ErrorIs(t, err, ErrPathResolution)
	assert.ErrorIs(t, err, errGetwdFailed)
	assert.Equal(t, "some/relative/path", got, "the input path is returned unchanged when it cannot be made absolute")
}

// TestResolveAllForCheck_WarnsOncePerFailure verifies the failure count, one WARN per
// failure, and that every input keeps a checkable path in the result.
func TestResolveAllForCheck_WarnsOncePerFailure(t *testing.T) {
	skipIfRoot(t)
	tmpDir := tu.SafeTempDir(t)
	firstBad := blockedPathUnder(t, tmpDir, "blocked-one")
	secondBad := blockedPathUnder(t, tmpDir, "blocked-two")
	good := filepath.Join(tmpDir, "good")
	require.NoError(t, os.Mkdir(good, 0o700))

	logger, buf := newBufferLogger()
	resolved, failures := ResolveAllForCheck([]string{firstBad, good, secondBad}, logger)

	assert.Equal(t, 2, failures)
	require.Len(t, resolved, 3)
	assert.Equal(t, 2, strings.Count(buf.String(), "level=WARN"))
	// The failed entries must still carry a checkable path; a dropped path would be
	// silently exempted from the permission check.
	assert.Equal(t, []string{firstBad, good, secondBad}, resolved)
}

// TestResolveAllForCheck_NoWarnOnSuccessfulResolution verifies that the normal path
// logs nothing.
func TestResolveAllForCheck_NoWarnOnSuccessfulResolution(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "a"), 0o700))

	logger, buf := newBufferLogger()
	resolved, failures := ResolveAllForCheck([]string{filepath.Join(tmpDir, "a"), filepath.Join(tmpDir, "b")}, logger)

	assert.Zero(t, failures)
	assert.Len(t, resolved, 2)
	assert.Empty(t, buf.String())
}

// TestClassifyCheckTarget verifies the three skip reasons, including the precedence
// of an unexpanded variable reference over relativeness.
func TestClassifyCheckTarget(t *testing.T) {
	tests := []struct {
		name string
		path string
		want CheckSkipReason
	}{
		{name: "absolute path is checkable", path: "/usr/bin/echo", want: CheckSkipNone},
		{name: "absolute path with variable reference", path: "/opt/%{APP}/bin", want: CheckSkipVariableReference},
		{name: "relative path", path: "bin/echo", want: CheckSkipRelative},
		{name: "relative path with variable reference", path: "%{APP}/bin", want: CheckSkipVariableReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ClassifyCheckTarget(tt.path))
		})
	}
}

// TestRunTOCTOUPermissionCheck_CountsCheckedAndSkipped pins the counting rule: a
// directory that violates the policy was inspected, so it counts as checked, while a
// directory that does not exist counts as skipped.
func TestRunTOCTOUPermissionCheck_CountsCheckedAndSkipped(t *testing.T) {
	cleanDir := tu.SafeTempDir(t)
	require.NoError(t, os.Chmod(cleanDir, 0o755))

	violatingDir := tu.SafeTempDir(t)
	require.NoError(t, os.Chmod(violatingDir, 0o777))
	t.Cleanup(func() { _ = os.Chmod(violatingDir, 0o755) })

	missingDir := filepath.Join(cleanDir, "missing")

	v, err := NewDirectoryPermChecker()
	require.NoError(t, err)

	result := RunTOCTOUPermissionCheck(v, []string{cleanDir, missingDir, violatingDir}, slog.Default())

	assert.Equal(t, 2, result.Checked)
	assert.Equal(t, 1, result.Skipped)
	assert.Len(t, result.Violations, 1)
}

// TestDeepestExistingAncestor verifies that the walk stops at the path itself when it
// exists, and at its deepest existing ancestor otherwise.
func TestDeepestExistingAncestor(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	existing := filepath.Join(tmpDir, "here")
	require.NoError(t, os.Mkdir(existing, 0o700))

	got, err := DeepestExistingAncestor(existing)
	require.NoError(t, err)
	assert.Equal(t, existing, got)

	got, err = DeepestExistingAncestor(filepath.Join(existing, "not", "created"))
	require.NoError(t, err)
	assert.Equal(t, existing, got)

	got, err = DeepestExistingAncestor("relative/path")
	require.ErrorIs(t, err, ErrInvalidPath, "a relative path has no ancestor to walk")
	assert.Empty(t, got)
}

// TestResolvePathForCheck_DotDotAfterSymlinkFollowsTheLink verifies that a ".."
// following a symlink component is resolved the way the kernel resolves it. Cleaning
// the path lexically first would collapse it to the tree the link sits in, and the
// permission check would then inspect directories the target is not under.
func TestResolvePathForCheck_DotDotAfterSymlinkFollowsTheLink(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	linkTarget := filepath.Join(tmpDir, "a", "real")
	require.NoError(t, os.MkdirAll(linkTarget, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "b"), 0o700))
	require.NoError(t, os.Symlink(linkTarget, filepath.Join(tmpDir, "b", "link")))

	// Built by concatenation, not filepath.Join: Join cleans, which would collapse
	// the ".." before the function under test ever sees it.
	// b/link/.. is a/, not b/.
	got, err := ResolvePathForCheck(tmpDir + "/b/link/../sibling")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "a", "sibling"), got)
}

// TestResolvePathForCheck_DanglingSymlinkAncestorFails pins the choice of os.Lstat
// over os.Stat when looking for the deepest existing ancestor. With os.Stat the
// broken link would count as absent, resolution would succeed against its parent,
// and the check would silently inspect the tree the link sits in instead of the one
// it points at.
func TestResolvePathForCheck_DanglingSymlinkAncestorFails(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	dangling := filepath.Join(tmpDir, "dangling")
	require.NoError(t, os.Symlink(filepath.Join(tmpDir, "nowhere"), dangling))

	got, err := ResolvePathForCheck(filepath.Join(dangling, "child"))

	require.ErrorIs(t, err, ErrPathResolution)
	assert.Equal(t, filepath.Join(dangling, "child"), got, "a checkable path is still returned")
}

// TestResolvePathForCheck_DotDotAfterSymlinkInRelativePath is the relative-path
// counterpart of TestResolvePathForCheck_DotDotAfterSymlinkFollowsTheLink. Making
// the path absolute must not clean it either, or the ".." is collapsed before the
// walk ever sees it.
func TestResolvePathForCheck_DotDotAfterSymlinkInRelativePath(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	linkTarget := filepath.Join(tmpDir, "a", "real")
	require.NoError(t, os.MkdirAll(linkTarget, 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "b"), 0o700))
	require.NoError(t, os.Symlink(linkTarget, filepath.Join(tmpDir, "b", "link")))
	t.Chdir(tmpDir)

	got, err := ResolvePathForCheck("b/link/../sibling")

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmpDir, "a", "sibling"), got)
}
