//go:build linux

package safefileio

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

// inodeOf returns the (dev, ino) pair identifying the file at path, following
// symlinks (there should be none in these tests).
func inodeOf(t *testing.T, path string) (dev, ino uint64) {
	t.Helper()
	fi, err := os.Stat(path)
	require.NoError(t, err)
	stat, ok := fi.Sys().(*syscall.Stat_t)
	require.True(t, ok, "expected *syscall.Stat_t")
	return stat.Dev, stat.Ino
}

// TestMoveFileAnchored_RegressionSuccessfulMove verifies the untampered
// (regression) path: opening the source via SafeOpenFile and moving it with
// moveFileAnchored succeeds and the destination has the original content.
func TestMoveFileAnchored_RegressionSuccessfulMove(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	content := []byte("original content")
	require.NoError(t, os.WriteFile(srcPath, content, 0o600))

	fs := NewFileSystem(FileSystemConfig{})
	srcFile, err := fs.SafeOpenFile(srcPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer func() { _ = srcFile.Close() }()

	srcDev, srcIno := inodeOf(t, srcPath)

	require.NoError(t, moveFileAnchored(srcFile, srcPath, dstPath))

	got, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	dstDev, dstIno := inodeOf(t, dstPath)
	assert.Equal(t, srcDev, dstDev)
	assert.Equal(t, srcIno, dstIno, "destination must be the same inode that was opened")

	_, err = os.Stat(srcPath)
	assert.True(t, os.IsNotExist(err), "source path should be removed after a successful move")
}

// TestMoveFileAnchored_SourceReplacementFailsClosed covers the case where,
// once the source has been opened, the path is then replaced with a
// different inode (unlink + recreate, dropping the originally verified
// inode's nlink to 0): the move must fail closed rather than moving the
// wrong content.
//
// This is a hard Linux kernel constraint, not a design choice: a regular
// file (not opened with O_TMPFILE) that has been fully unlinked (nlink == 0)
// cannot be given a new name via /proc/self/fd/<n> (see may_linkat in the
// kernel). So the same replacement that this mechanism must detect also
// causes linkat itself to fail with ENOENT, before any rename is attempted.
// This scenario was originally expected to succeed by moving the
// pre-replacement inode, which turned out to be unachievable with this
// technique; see the design document's rationale on this kernel constraint
// for the corresponding design update.
func TestMoveFileAnchored_SourceReplacementFailsClosed(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	originalContent := []byte("verified content")
	require.NoError(t, os.WriteFile(srcPath, originalContent, 0o600))

	fs := NewFileSystem(FileSystemConfig{})
	srcFile, err := fs.SafeOpenFile(srcPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer func() { _ = srcFile.Close() }()

	// Simulate an attacker replacing the source path with a different inode
	// after the fd was obtained but before the move happens. This drops the
	// originally verified inode's nlink to 0.
	require.NoError(t, os.Remove(srcPath))
	replacedContent := []byte("attacker-controlled content")
	require.NoError(t, os.WriteFile(srcPath, replacedContent, 0o600))

	err = moveFileAnchored(srcFile, srcPath, dstPath)
	require.Error(t, err, "move must fail closed when the source was replaced after verification")

	_, statErr := os.Stat(dstPath)
	assert.True(t, os.IsNotExist(statErr), "destination must not be created on a failed move")

	// Nothing was moved or unlinked: the replacement content is still at
	// srcPath, untouched, since linkat failed before rename or unlink ran.
	got, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, replacedContent, got)
}

// TestMoveFileAnchored_RenameFailureCleansUpTemporaryLink verifies that when
// the rename step fails after the hard link was created, the temporary link
// is removed rather than leaked in the destination directory.
func TestMoveFileAnchored_RenameFailureCleansUpTemporaryLink(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0o600))

	fs := NewFileSystem(FileSystemConfig{})
	srcFile, err := fs.SafeOpenFile(srcPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer func() { _ = srcFile.Close() }()

	// Use a destination that is itself a non-empty directory so os.Rename
	// (temp name -> dstPath) fails deterministically.
	dstPath := filepath.Join(dir, "dst_is_dir")
	require.NoError(t, os.Mkdir(dstPath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dstPath, "keep.txt"), []byte("x"), 0o600))

	err = moveFileAnchored(srcFile, srcPath, dstPath)
	require.Error(t, err)

	// No leaked temporary link should remain in the destination directory.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		assert.NotContains(t, e.Name(), ".safefileio-move-", "temporary hard link must not leak on rename failure")
	}

	// Source must be untouched since the move did not complete.
	_, err = os.Stat(srcPath)
	assert.NoError(t, err, "source should still exist after a failed move")
}

// TestAtomicMoveFileCore_EndToEndUsesFDAnchoredMove exercises the full public
// entry point (osFS.AtomicMoveFile -> atomicMoveFileCore), not just the lower
// level moveFileAnchored, so that fchmod/permission validation and the final
// destination validation that wrap the fd-anchored move are also covered.
func TestAtomicMoveFileCore_EndToEndUsesFDAnchoredMove(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	content := []byte("full pipeline content")
	require.NoError(t, os.WriteFile(srcPath, content, 0o644))

	srcDev, srcIno := inodeOf(t, srcPath)

	fs := NewFileSystem(FileSystemConfig{})
	require.NoError(t, fs.AtomicMoveFile(srcPath, dstPath, 0o644))

	got, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)

	dstDev, dstIno := inodeOf(t, dstPath)
	assert.Equal(t, srcDev, dstDev)
	assert.Equal(t, srcIno, dstIno, "destination must be the same inode fchmod/validation ran against")

	_, err = os.Stat(srcPath)
	assert.True(t, os.IsNotExist(err), "source path should be removed after a successful move")
}

// TestMoveFileAnchored_UnlinkSourceFailureReturnsErrorAfterSuccessfulRename
// covers the documented failure mode where rename to the destination
// succeeds but the trailing unlink of absSrc fails: per the design
// document's semantics for a failed source unlink, this is treated as an
// error (not a warned success) so the caller can investigate, even though
// the destination now holds the moved content.
func TestMoveFileAnchored_UnlinkSourceFailureReturnsErrorAfterSuccessfulRename(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping privilege test when running as root")
	}
	dir := tu.SafeTempDir(t)
	srcParent := filepath.Join(dir, "srcparent")
	require.NoError(t, os.Mkdir(srcParent, 0o755))
	srcPath := filepath.Join(srcParent, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	content := []byte("content")
	require.NoError(t, os.WriteFile(srcPath, content, 0o600))

	fs := NewFileSystem(FileSystemConfig{})
	srcFile, err := fs.SafeOpenFile(srcPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer func() { _ = srcFile.Close() }()

	// Remove write+execute permission on srcPath's parent directory so the
	// trailing os.Remove(absSrc) fails with EACCES, after linkat and rename
	// have already succeeded.
	require.NoError(t, os.Chmod(srcParent, 0o555))
	t.Cleanup(func() { _ = os.Chmod(srcParent, 0o755) })

	err = moveFileAnchored(srcFile, srcPath, dstPath)
	require.Error(t, err, "unlink(absSrc) failure must be surfaced as an error, not a silent success")

	// The destination is already populated with the moved content: rename
	// completed before the unlink failure occurred.
	got, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestLinkFileToTempName_RetriesOnNameCollision forces generateTempLinkName to
// return a colliding name first, then a free one, and verifies
// linkFileToTempName retries rather than failing on the first EEXIST.
func TestLinkFileToTempName_RetriesOnNameCollision(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0o600))

	fs := NewFileSystem(FileSystemConfig{})
	srcFile, err := fs.SafeOpenFile(srcPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer func() { _ = srcFile.Close() }()
	osFile, ok := srcFile.(*os.File)
	require.True(t, ok)

	// Pre-create a file at the name the first (forced) attempt will collide with.
	const collidingName = ".safefileio-move-collision-test"
	require.NoError(t, os.WriteFile(filepath.Join(dir, collidingName), []byte("existing"), 0o600))

	names := []string{collidingName, "safefileio-move-free-test"}
	call := 0
	origFunc := generateTempLinkName
	generateTempLinkName = func() (string, error) {
		name := names[call]
		call++
		return name, nil
	}
	t.Cleanup(func() { generateTempLinkName = origFunc })

	tmpPath, err := linkFileToTempName(osFile, dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, names[1]), tmpPath)
	assert.Equal(t, 2, call, "expected exactly one retry after the collision")

	// Clean up the created hard link.
	_ = os.Remove(tmpPath)
}

// TestLinkFileToTempName_ExhaustsAttempts verifies that persistent name
// collisions produce a clear error instead of retrying forever.
func TestLinkFileToTempName_ExhaustsAttempts(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0o600))

	fs := NewFileSystem(FileSystemConfig{})
	srcFile, err := fs.SafeOpenFile(srcPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer func() { _ = srcFile.Close() }()
	osFile, ok := srcFile.(*os.File)
	require.True(t, ok)

	const collidingName = ".safefileio-move-always-collides"
	require.NoError(t, os.WriteFile(filepath.Join(dir, collidingName), []byte("existing"), 0o600))

	origFunc := generateTempLinkName
	generateTempLinkName = func() (string, error) { return collidingName, nil }
	t.Cleanup(func() { generateTempLinkName = origFunc })

	_, err = linkFileToTempName(osFile, dir)
	require.Error(t, err)
}

// TestLinkFileToTempName_NonEEXISTErrorIsNotRetried verifies that a linkat
// failure other than EEXIST (e.g. EPERM from fs.protected_hardlinks, or
// ETXTBSY) is returned immediately rather than being retried as if it were a
// name collision. Real EPERM/ETXTBSY conditions depend on privilege/fs state
// that isn't reliably reproducible in a test sandbox, so linkatFunc is
// stubbed to force the failure deterministically.
func TestLinkFileToTempName_NonEEXISTErrorIsNotRetried(t *testing.T) {
	dir := tu.SafeTempDir(t)
	srcPath := filepath.Join(dir, "src.txt")
	require.NoError(t, os.WriteFile(srcPath, []byte("content"), 0o600))

	fs := NewFileSystem(FileSystemConfig{})
	srcFile, err := fs.SafeOpenFile(srcPath, os.O_RDONLY, 0)
	require.NoError(t, err)
	defer func() { _ = srcFile.Close() }()
	osFile, ok := srcFile.(*os.File)
	require.True(t, ok)

	calls := 0
	origLinkat := linkatFunc
	linkatFunc = func(_ int, _ string, _ int, _ string, _ int) error {
		calls++
		return unix.EPERM
	}
	t.Cleanup(func() { linkatFunc = origLinkat })

	_, err = linkFileToTempName(osFile, dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, unix.EPERM)
	assert.Equal(t, 1, calls, "a non-EEXIST linkat error must not be retried")
}

// openat2SyscallFunc is the shape of the raw openat2 system call, used to type
// the test stubs installed below.
type openat2SyscallFunc func(dirfd int, pathname string, how *openHow) (int, error)

// stubOpenat2 installs an openat2 stub and restores the previous value when
// the test ends. It returns the value it displaced so a stub can delegate the
// opens it does not care about to the real system call.
func stubOpenat2(t *testing.T, stub func(realCall openat2SyscallFunc) openat2SyscallFunc) {
	t.Helper()
	realCall := openat2SyscallFunc(openat2SyscallOverride)
	t.Cleanup(func() { openat2SyscallOverride = realCall })
	openat2SyscallOverride = stub(realCall)
}

// TestOpenat2_RetriesOnEINTR verifies that EINTR from the kernel is retried
// until the call goes through, rather than surfacing to the caller. EINTR
// arrives from the Go runtime's asynchronous preemption signal, which a test
// cannot provoke on demand, so the raw system call is stubbed to produce it.
//
// The stub returns EINTR more than once on purpose: a "retry exactly once"
// implementation would satisfy a single-EINTR test while still leaking EINTR
// under the repeated signals the unbounded loop exists to absorb.
func TestOpenat2_RetriesOnEINTR(t *testing.T) {
	const interruptions = 3

	dir := tu.SafeTempDir(t)
	filePath := filepath.Join(dir, "target.txt")
	const content = "eintr-retry-content"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0o600))

	fs := newFileSystemForRoute(t, openRoutes[0])

	calls := 0
	stubOpenat2(t, func(realCall openat2SyscallFunc) openat2SyscallFunc {
		return func(dirfd int, pathname string, how *openHow) (int, error) {
			// Count only opens of the file under test: unrelated opens
			// elsewhere in the process would make the count unstable.
			if pathname != filePath {
				return realCall(dirfd, pathname, how)
			}
			calls++
			if calls <= interruptions {
				return -1, syscall.EINTR
			}
			return realCall(dirfd, pathname, how)
		}
	})

	file, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
	require.NoError(t, err, "EINTR must not reach the caller")
	t.Cleanup(func() { _ = file.Close() })

	got, err := io.ReadAll(file)
	require.NoError(t, err)
	assert.Equal(t, content, string(got))
	assert.Equal(t, interruptions+1, calls, "every EINTR must be retried, not just the first")
}

// TestOpenat2_NonEINTRErrnoMapping verifies that adding the EINTR retry left
// the existing errno mapping intact: every other errno is still translated to
// the sentinel it was translated to before.
func TestOpenat2_NonEINTRErrnoMapping(t *testing.T) {
	tests := []struct {
		name  string
		errno syscall.Errno
		want  error
	}{
		{name: "ELOOP", errno: syscall.ELOOP, want: ErrIsSymlink},
		{name: "EEXIST", errno: syscall.EEXIST, want: ErrFileExists},
		{name: "ENOENT", errno: syscall.ENOENT, want: os.ErrNotExist},
	}

	fs := newFileSystemForRoute(t, openRoutes[0])

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The target need not exist: the stub answers for it before the
			// kernel is reached.
			filePath := filepath.Join(tu.SafeTempDir(t), "target.txt")

			calls := 0
			stubOpenat2(t, func(realCall openat2SyscallFunc) openat2SyscallFunc {
				return func(dirfd int, pathname string, how *openHow) (int, error) {
					if pathname != filePath {
						return realCall(dirfd, pathname, how)
					}
					calls++
					return -1, tt.errno
				}
			})

			_, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.want)
			assert.Equal(t, 1, calls, "an errno other than EINTR must not be retried")
		})
	}
}

// TestOpenat2Mode pins the rule that decides openHow.mode. The O_DIRECTORY row
// is the one a caller-level test cannot reach cheaply: O_TMPFILE's constant
// includes O_DIRECTORY, so an intersection test rather than an equality test
// would misread a plain directory open as creating and leak perm into a mode
// the kernel would reject.
func TestOpenat2Mode(t *testing.T) {
	const perm = os.FileMode(0o640)

	tests := []struct {
		name string
		flag int
		want uint64
	}{
		{name: "read_only", flag: os.O_RDONLY, want: 0},
		{name: "write_only_no_create", flag: os.O_WRONLY, want: 0},
		{name: "directory", flag: os.O_RDONLY | unix.O_DIRECTORY, want: 0},
		{name: "create", flag: os.O_CREATE | os.O_WRONLY, want: uint64(perm)},
		{name: "create_excl", flag: os.O_CREATE | os.O_EXCL | os.O_WRONLY, want: uint64(perm)},
		{name: "tmpfile", flag: unix.O_TMPFILE | os.O_RDWR, want: uint64(perm)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, openat2Mode(tt.flag, perm))
		})
	}
}

// TestOpenat2_PassesModeOnlyForCreatingOpens verifies the kernel-side half of
// the mode contract by reading openHow.mode as it is handed to the system
// call: zero unless the open may create a file.
//
// The O_TMPFILE row matters because the kernel does NOT reject a zero mode
// there -- it creates a mode 0000 file -- so a wrong rule would diverge from
// the fallback path silently rather than failing.
func TestOpenat2_PassesModeOnlyForCreatingOpens(t *testing.T) {
	const perm = os.FileMode(0o640)

	dir := tu.SafeTempDir(t)
	existingPath := filepath.Join(dir, "existing.txt")
	require.NoError(t, os.WriteFile(existingPath, []byte("content"), 0o600))
	createdPath := filepath.Join(dir, "created.txt")

	fs := newFileSystemForRoute(t, openRoutes[0])

	modes := map[string][]uint64{}
	stubOpenat2(t, func(realCall openat2SyscallFunc) openat2SyscallFunc {
		return func(dirfd int, pathname string, how *openHow) (int, error) {
			switch pathname {
			case existingPath, createdPath, dir:
				modes[pathname] = append(modes[pathname], how.mode)
			}
			return realCall(dirfd, pathname, how)
		}
	})

	readFile, err := fs.SafeOpenFile(existingPath, os.O_RDONLY, perm)
	require.NoError(t, err)
	require.NoError(t, readFile.Close())

	createdFile, err := fs.SafeOpenFile(createdPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, perm)
	require.NoError(t, err)
	require.NoError(t, createdFile.Close())

	tmpFile, err := fs.SafeOpenFile(dir, unix.O_TMPFILE|os.O_RDWR, perm)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	// Assert the stub actually saw each open before asserting its mode: a
	// missing key would otherwise read back as the zero value and let the
	// "mode is 0" assertions pass without observing anything.
	require.Equal(t, []uint64{0}, modes[existingPath], "an open without O_CREATE must pass mode 0")
	require.Equal(t, []uint64{uint64(perm)}, modes[createdPath], "a creating open must pass the requested permission bits")
	require.Equal(t, []uint64{uint64(perm)}, modes[dir], "an O_TMPFILE open creates a file and must pass the requested permission bits")
}
