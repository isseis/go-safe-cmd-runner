package security

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempRoot returns the test's temp dir with any symlinks resolved, so fixture
// paths built under it contain no unexpected symlink components that would skew
// resolver call counts.
func tempRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	return root
}

// TestResolveOperandPath_SymlinkTarget asserts the leaf symlink is followed to its
// target, so a safe-zone-looking path that points elsewhere is classified by the
// target (the cp evil $WORKDIR/link, link -> trust-critical case in Phase 3). The
// target is a real file with an absolute symlink so the expected resolved path is
// stable across platforms (a hard-coded /etc could itself be a symlink).
func TestResolveOperandPath_SymlinkTarget(t *testing.T) {
	root := tempRoot(t)
	target := filepath.Join(root, "elsewhere", "secret")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
	require.NoError(t, os.WriteFile(target, nil, 0o600))

	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(target, link)) // absolute target

	got, err := ResolveOperandPath(link, "", MaxSymlinkDepth)
	require.NoError(t, err)
	assert.Equal(t, target, got)
}

// TestResolveOperandPath_RelativeSymlinkTarget asserts an `ln -s` relative target
// resolves against the link's own parent directory, not against the supplied base.
func TestResolveOperandPath_RelativeSymlinkTarget(t *testing.T) {
	root := tempRoot(t)
	linkDir := filepath.Join(root, "sub")
	require.NoError(t, os.MkdirAll(linkDir, 0o755))
	target := filepath.Join(linkDir, "target")
	require.NoError(t, os.WriteFile(target, nil, 0o644))

	link := filepath.Join(linkDir, "link")
	require.NoError(t, os.Symlink("target", link)) // relative target

	otherBase := filepath.Join(root, "other")
	require.NoError(t, os.MkdirAll(otherBase, 0o755))

	// The operand is absolute, so base is irrelevant for the operand itself; the
	// relative target must still resolve against linkDir, not otherBase.
	got, err := ResolveOperandPath(link, otherBase, MaxSymlinkDepth)
	require.NoError(t, err)
	assert.Equal(t, target, got)

	// A relative operand, in contrast, does resolve against base.
	got2, err := ResolveOperandPath("target", linkDir, MaxSymlinkDepth)
	require.NoError(t, err)
	assert.Equal(t, target, got2)
}

// TestResolveOperandPath_NonexistentLeaf asserts a not-yet-created leaf folds onto
// its existing real parent, so a write destination classifies by that parent.
func TestResolveOperandPath_NonexistentLeaf(t *testing.T) {
	root := tempRoot(t)
	got, err := ResolveOperandPath(filepath.Join(root, "newfile"), "", MaxSymlinkDepth)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "newfile"), got)

	// Several non-existent trailing components fold together.
	got2, err := ResolveOperandPath(filepath.Join(root, "a", "b", "c"), "", MaxSymlinkDepth)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(root, "a", "b", "c"), got2)
}

// TestResolveOperandPath_Cycle asserts a true symlink cycle fails closed: the hop
// counter bounds the loop (no visited-node set, which would false-positive on
// legitimately repeated nodes).
func TestResolveOperandPath_Cycle(t *testing.T) {
	root := tempRoot(t)
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	require.NoError(t, os.Symlink(b, a))
	require.NoError(t, os.Symlink(a, b))

	_, err := ResolveOperandPath(a, "", MaxSymlinkDepth)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOperandResolution)
}

// TestResolveOperandPath_RepeatedSymlinkNode is the regression for the cycle
// false-positive: a single symlink that a path legitimately traverses more than
// once must resolve, not be rejected. With `link -> .` (a self-referential dir
// symlink), `link/link/link/x` resolves to the real `x` under root.
func TestResolveOperandPath_RepeatedSymlinkNode(t *testing.T) {
	root := tempRoot(t)
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(".", link)) // link -> its own parent (root)
	target := filepath.Join(root, "x")
	require.NoError(t, os.WriteFile(target, nil, 0o644))

	got, err := ResolveOperandPath(filepath.Join(link, "link", "link", "x"), "", MaxSymlinkDepth)
	require.NoError(t, err)
	assert.Equal(t, target, got)
}

// TestResolveOperandPath_DepthExceeded asserts a chain longer than maxHops fails
// closed, while the same chain resolves when the budget is sufficient.
func TestResolveOperandPath_DepthExceeded(t *testing.T) {
	root := tempRoot(t)
	target := filepath.Join(root, "target")
	require.NoError(t, os.WriteFile(target, nil, 0o644))

	prev := target
	const chainLen = 5
	for i := range chainLen {
		link := filepath.Join(root, fmt.Sprintf("l%d", i))
		require.NoError(t, os.Symlink(prev, link))
		prev = link
	}

	_, err := ResolveOperandPath(prev, "", 2)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOperandResolution)

	got, err := ResolveOperandPath(prev, "", MaxSymlinkDepth)
	require.NoError(t, err)
	assert.Equal(t, target, got)
}

// TestResolveOperandPath_MidChainLstatError asserts a non-ENOENT lstat failure
// fails closed (read-only resolution cannot recover a permission error).
func TestResolveOperandPath_MidChainLstatError(t *testing.T) {
	r := newOperandResolver(
		func(string) (fs.FileInfo, error) { return nil, os.ErrPermission },
		os.Readlink,
	)
	_, err := r.resolve("/some/abs/path", "", MaxSymlinkDepth)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrOperandResolution)
}

// TestMemoizationLinear is the falsifiable resolution-cost assertion: K operands
// sharing one parent chain of depth D resolve with D+K lstat calls when the memo
// folds the shared chain, versus K*(D+1) without it.
func TestMemoizationLinear(t *testing.T) {
	root := tempRoot(t)
	sharedDir := filepath.Join(root, "p1", "p2", "p3")
	require.NoError(t, os.MkdirAll(sharedDir, 0o755))

	const k = 5
	leaves := make([]string, k)
	for i := range k {
		leaf := filepath.Join(sharedDir, fmt.Sprintf("f%d", i))
		require.NoError(t, os.WriteFile(leaf, nil, 0o644))
		leaves[i] = leaf
	}
	d := len(splitAbs(sharedDir)) // depth of the shared parent chain below root

	// Memoized: one resolver shared across all operands.
	var lstatN, readlinkN int
	r := newOperandResolver(
		func(p string) (fs.FileInfo, error) { lstatN++; return os.Lstat(p) },
		func(p string) (string, error) { readlinkN++; return os.Readlink(p) },
	)
	for _, leaf := range leaves {
		_, err := r.resolve(leaf, "", MaxSymlinkDepth)
		require.NoError(t, err)
	}
	assert.Equal(t, d+k, lstatN, "memoized lstat calls should fold the shared parent chain to D+K")
	assert.Equal(t, 0, readlinkN, "no symlinks in the fixture")

	// Naive: a fresh resolver (empty memo) per operand re-walks the whole chain.
	var naiveN int
	for _, leaf := range leaves {
		nr := newOperandResolver(
			func(p string) (fs.FileInfo, error) { naiveN++; return os.Lstat(p) },
			os.Readlink,
		)
		_, err := nr.resolve(leaf, "", MaxSymlinkDepth)
		require.NoError(t, err)
	}
	assert.Equal(t, k*(d+1), naiveN, "without memo each operand re-walks D+1 nodes")
	assert.Less(t, lstatN, naiveN, "memoization must reduce the call count")
}

// TestMemoizationSymlinkParentNotFolded locks in the memo's documented scope: only
// existing non-symlink nodes are memoized, so a shared symlink parent is followed
// once per operand (readlink == K) rather than folded. Cost stays bounded and
// linear in K; it is not the symlink-free D+K of TestMemoizationLinear.
func TestMemoizationSymlinkParentNotFolded(t *testing.T) {
	root := tempRoot(t)
	realDir := filepath.Join(root, "real")
	require.NoError(t, os.MkdirAll(realDir, 0o755))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(realDir, link)) // absolute target

	const k = 5
	operands := make([]string, k)
	for i := range k {
		require.NoError(t, os.WriteFile(filepath.Join(realDir, fmt.Sprintf("f%d", i)), nil, 0o644))
		operands[i] = filepath.Join(link, fmt.Sprintf("f%d", i))
	}

	var lstatN, readlinkN int
	r := newOperandResolver(
		func(p string) (fs.FileInfo, error) { lstatN++; return os.Lstat(p) },
		func(p string) (string, error) { readlinkN++; return os.Readlink(p) },
	)
	for i, op := range operands {
		got, err := r.resolve(op, "", MaxSymlinkDepth)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(realDir, fmt.Sprintf("f%d", i)), got,
			"a symlink parent must resolve to the real target")
	}

	// The symlink node is never memoized, so it is read once per operand.
	assert.Equal(t, k, readlinkN, "shared symlink parent is followed once per operand")
	// Exact lstat budget: op0 walks root (depth d0) + link + real + f0 = d0+3;
	// each later operand replays root/real from the memo and only lstats link + its
	// leaf = 2. Total d0 + 3 + 2*(K-1).
	d0 := len(splitAbs(root))
	assert.Equal(t, d0+3+2*(k-1), lstatN, "symlink parent is re-walked but real nodes stay memoized")
}

// TestTrustedPredicate is the differential for the Trusted predicate: the verdict
// depends on the injected RunAsIdent (not the live euid that owns the fixtures)
// and on the writability of origin's ancestors.
func TestTrustedPredicate(t *testing.T) {
	root := tempRoot(t)
	origin := filepath.Join(root, "work")
	require.NoError(t, os.MkdirAll(origin, 0o700))
	resolved := filepath.Join(origin, "file")
	trustedDirs := []string{root}

	r := newOperandResolver(os.Lstat, os.Readlink)
	euid := uint32(os.Geteuid())
	egid := uint32(os.Getgid())

	// An identity that does NOT own the fixtures: every ancestor is non-writable
	// (owned by the real euid, 0700; /tmp is sticky), so the operand is Trusted.
	identOther := risktypes.RunAsIdent{UID: euid + 1, GID: egid + 1}
	assert.True(t, r.isTrustedOperand(resolved, origin, trustedDirs, identOther),
		"foreign run-as over non-writable ancestors should be Trusted")

	// A trailing separator on origin must not make the ancestor check start at
	// origin itself (filepath.Dir of an uncleaned trailing-slash path returns the
	// dir itself); the verdict must match the no-slash case.
	assert.True(t, r.isTrustedOperand(resolved, origin+"/", trustedDirs, identOther),
		"a trailing separator on origin should not change the verdict")

	// The same call with an identity that owns the ancestors is not Trusted: the
	// owner could chmod to repoint the safe-zone anchor. The live euid is constant
	// across both calls, so only the injected identity changed the verdict.
	identSelf := risktypes.RunAsIdent{UID: euid, GID: egid}
	assert.False(t, r.isTrustedOperand(resolved, origin, trustedDirs, identSelf),
		"run-as owning the ancestors should not be Trusted")

	// Outside the trusted-directory allowlist is never Trusted.
	assert.False(t, r.isTrustedOperand("/etc/passwd", origin, trustedDirs, identOther),
		"a path outside the trusted dirs should not be Trusted")

	// Inside the trusted dirs but NOT inside the safe-zone origin: the predicate
	// must not Trust it (it would otherwise inspect an unrelated ancestor chain).
	sibling := filepath.Join(root, "sibling")
	assert.False(t, r.isTrustedOperand(sibling, origin, trustedDirs, identOther),
		"a path outside the safe-zone origin should not be Trusted")

	// A relative origin or resolved path fails closed (the ancestor ascent would
	// otherwise terminate at "." and skip real system ancestors).
	assert.False(t, r.isTrustedOperand("relative/file", origin, trustedDirs, identOther),
		"a relative resolved path should not be Trusted")
	assert.False(t, r.isTrustedOperand(resolved, "relative/origin", trustedDirs, identOther),
		"a relative origin should not be Trusted")

	// A root run-as is intentionally degenerate: root can write anywhere, so no
	// operand is Trusted regardless of ancestor ownership/permissions.
	identRoot := risktypes.RunAsIdent{UID: 0, GID: 0}
	assert.False(t, r.isTrustedOperand(resolved, origin, trustedDirs, identRoot),
		"a root run-as should never earn the safe-zone Low")

	// Making origin's parent world-writable without a sticky bit makes the foreign
	// identity able to repoint the anchor: no longer Trusted.
	require.NoError(t, os.Chmod(root, 0o777))
	t.Cleanup(func() { _ = os.Chmod(root, 0o700) })
	assert.False(t, r.isTrustedOperand(resolved, origin, trustedDirs, identOther),
		"a world-writable non-sticky ancestor should not be Trusted")
}
