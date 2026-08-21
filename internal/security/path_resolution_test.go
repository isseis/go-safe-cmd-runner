//go:build test

package security

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
)

// errGetwdFailed is the failure injected into the working-directory lookup.
var errGetwdFailed = errors.New("getwd unavailable")

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
// working directory cannot be read, which is reachable only through the test override.
func TestResolvePathForCheck_AbsConversionFailure(t *testing.T) {
	original := getwdOverride
	getwdOverride = func() (string, error) { return "", errGetwdFailed }
	t.Cleanup(func() { getwdOverride = original })

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

	logger, rec := tu.NewRecordingLogger()
	resolved, failures := ResolveAllForCheck([]string{firstBad, good, secondBad}, logger)

	assert.Equal(t, 2, failures)
	require.Len(t, resolved, 3)
	warnings := rec.RecordsAtLevel(slog.LevelWarn)
	require.Len(t, warnings, 2)
	assert.Equal(t, []any{firstBad, secondBad}, []any{warnings[0].Attrs["path"], warnings[1].Attrs["path"]},
		"each warning names the path it is about, and the good path produces none")
	// The failed entries must still carry a checkable path; a dropped path would be
	// silently exempted from the permission check.
	assert.Equal(t, []string{firstBad, good, secondBad}, resolved)
}

// TestResolveAllForCheck_NoWarnOnSuccessfulResolution verifies that the normal path
// logs nothing.
func TestResolveAllForCheck_NoWarnOnSuccessfulResolution(t *testing.T) {
	tmpDir := tu.SafeTempDir(t)
	require.NoError(t, os.Mkdir(filepath.Join(tmpDir, "a"), 0o700))

	logger, rec := tu.NewRecordingLogger()
	resolved, failures := ResolveAllForCheck([]string{filepath.Join(tmpDir, "a"), filepath.Join(tmpDir, "b")}, logger)

	assert.Zero(t, failures)
	assert.Len(t, resolved, 2)
	assert.Empty(t, rec.Records())
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
