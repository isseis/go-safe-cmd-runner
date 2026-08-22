//go:build linux

package safefileio

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

	dirFd := dirFdForTest(t, fs, dir)
	require.NoError(t, moveFileAnchored(srcFile, dirFd, "src.txt", dirFd, "dst.txt"))

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
	dirFd := dirFdForTest(t, fs, dir)

	// Simulate an attacker replacing the source path with a different inode
	// after the fd was obtained but before the move happens. This drops the
	// originally verified inode's nlink to 0.
	require.NoError(t, os.Remove(srcPath))
	replacedContent := []byte("attacker-controlled content")
	require.NoError(t, os.WriteFile(srcPath, replacedContent, 0o600))

	err = moveFileAnchored(srcFile, dirFd, "src.txt", dirFd, "dst.txt")
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

	// Use a destination that is itself a non-empty directory so the rename
	// (temp name -> destination) fails deterministically.
	dstPath := filepath.Join(dir, "dst_is_dir")
	require.NoError(t, os.Mkdir(dstPath, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dstPath, "keep.txt"), []byte("x"), 0o600))

	dirFd := dirFdForTest(t, fs, dir)
	err = moveFileAnchored(srcFile, dirFd, "src.txt", dirFd, "dst_is_dir")
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
//
// It runs on both routes because the fd acquisition differs between them:
// openDirNoSymlinks and openFileAt reach openat2 on one and the openat-based
// fallback on the other, and the fallback is production code wherever openat2 is
// missing (Linux 5.5 and older, restrictive seccomp profiles, every non-Linux
// platform).
func TestAtomicMoveFileCore_EndToEndUsesFDAnchoredMove(t *testing.T) {
	for _, route := range openRoutes {
		t.Run(route.name, func(t *testing.T) {
			dir := tu.SafeTempDir(t)
			srcPath := filepath.Join(dir, "src.txt")
			dstPath := filepath.Join(dir, "dst.txt")
			content := []byte("full pipeline content")
			require.NoError(t, os.WriteFile(srcPath, content, 0o644))

			srcDev, srcIno := inodeOf(t, srcPath)

			fs := newFileSystemForRoute(t, route)
			require.NoError(t, fs.AtomicMoveFile(srcPath, dstPath, 0o644))

			got, err := os.ReadFile(dstPath)
			require.NoError(t, err)
			assert.Equal(t, content, got)

			dstDev, dstIno := inodeOf(t, dstPath)
			assert.Equal(t, srcDev, dstDev)
			assert.Equal(t, srcIno, dstIno, "destination must be the same inode fchmod/validation ran against")

			_, err = os.Stat(srcPath)
			assert.True(t, os.IsNotExist(err), "source path should be removed after a successful move")
		})
	}
}

// TestAtomicMoveFile_MovesIntoUnreadableDirectory pins the permission a
// directory fd costs. The path-based code this replaced only ever needed search
// permission on the parent directories, so a write-only drop directory worked;
// anchoring the move to an fd must not quietly turn that into a read
// requirement. On Linux that is what O_PATH buys.
func TestAtomicMoveFile_MovesIntoUnreadableDirectory(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root bypasses directory permission checks, so this cannot fail here")
	}

	for _, route := range openRoutes {
		t.Run(route.name, func(t *testing.T) {
			dir := tu.SafeTempDir(t)
			srcPath := filepath.Join(dir, "src.txt")
			content := []byte("dropped content")
			require.NoError(t, os.WriteFile(srcPath, content, 0o600))

			// Write and search, no read: entries can be created and looked up by
			// name, but the directory cannot be listed or opened for reading.
			dropDir := filepath.Join(dir, "drop")
			require.NoError(t, os.Mkdir(dropDir, 0o700))
			require.NoError(t, os.Chmod(dropDir, 0o300))
			t.Cleanup(func() { _ = os.Chmod(dropDir, 0o700) })

			fs := newFileSystemForRoute(t, route)
			dstPath := filepath.Join(dropDir, "dst.txt")
			require.NoError(t, fs.AtomicMoveFile(srcPath, dstPath, 0o600))

			// Reading back needs the permission the move did not: restore it
			// first, so this assertion cannot be confused with the one above.
			require.NoError(t, os.Chmod(dropDir, 0o700))
			got, err := os.ReadFile(dstPath)
			require.NoError(t, err)
			assert.Equal(t, content, got)
		})
	}
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
	srcDirFd := dirFdForTest(t, fs, srcParent)
	dstDirFd := dirFdForTest(t, fs, dir)

	// Remove write permission on srcPath's parent directory so the trailing
	// unlink of the source fails with EACCES, after linkat and rename have
	// already succeeded. The directory fd was opened before this, exactly as the
	// production path opens it up front; a held fd does not exempt the unlink
	// from the permission check.
	require.NoError(t, os.Chmod(srcParent, 0o555))
	t.Cleanup(func() { _ = os.Chmod(srcParent, 0o755) })

	err = moveFileAnchored(srcFile, srcDirFd, "src.txt", dstDirFd, "dst.txt")
	require.Error(t, err, "unlink(absSrc) failure must be surfaced as an error, not a silent success")

	// The destination is already populated with the moved content: rename
	// completed before the unlink failure occurred.
	got, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

// TestMoveOpenFileCore_MoveFailureAfterRenameIsDestinationCommitted covers the
// second way a call can fail with the destination already replaced: the move
// removes the source entry after the rename, so its own error can arrive once
// the destination is committed. Nothing outside the move can tell which side of
// the rename such an error came from, so the move marks it and
// moveOpenFileCore translates the mark -- a failure reported as ordinary would
// tell the caller nothing happened when the previous content is in fact gone.
//
// It is Linux-only for the same reason as the moveFileAnchored-level test it
// mirrors: only there does the move remove the source as a separate step after
// the rename.
func TestMoveOpenFileCore_MoveFailureAfterRenameIsDestinationCommitted(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root is exempt from the directory permission check that makes the source unlink fail")
	}
	dir := tu.SafeTempDir(t)
	srcParent := filepath.Join(dir, "srcparent")
	require.NoError(t, os.Mkdir(srcParent, 0o755))
	srcPath := filepath.Join(srcParent, "src.txt")
	dstPath := filepath.Join(dir, "dst.txt")
	content := []byte("moved content")
	require.NoError(t, os.WriteFile(srcPath, content, 0o600))
	require.NoError(t, os.WriteFile(dstPath, []byte("previous content"), 0o600))

	fs := NewFileSystem(FileSystemConfig{})
	srcDirFd, srcFile := openSourceForMove(t, fs, srcParent, "src.txt")
	dstDirFd := dirFdForTest(t, fs, dir)

	// Take away write permission on the source's parent so the trailing unlink
	// fails with EACCES, after the rename has already put the inode in place.
	require.NoError(t, os.Chmod(srcParent, 0o555))
	t.Cleanup(func() { _ = os.Chmod(srcParent, 0o755) })

	recorder := captureWarnings(t)
	err := moveOpenFileCore(srcFile, srcDirFd, "src.txt", dstDirFd, "dst.txt", dstPath, 0o600, fs.GetGroupMembership())
	require.ErrorIs(t, err, ErrDestinationCommitted)

	got, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.Equal(t, content, got, "the rename went through, so the destination holds the moved content")

	record := recorder.RequireRecord(t, slog.LevelWarn, destinationCommittedMsg)
	record.AssertAttrs(t, map[string]any{"destination": dstPath})
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
	// The prefix is passed in rather than baked into randomTempName, so the
	// stub also pins the call site: an entry left behind is only recognizable
	// as this package's if the move path asks for the prefix it documents.
	var gotPrefixes []string
	origFunc := generateTempLinkName
	generateTempLinkName = func(prefix string) (string, error) {
		gotPrefixes = append(gotPrefixes, prefix)
		name := names[call]
		call++
		return name, nil
	}
	t.Cleanup(func() { generateTempLinkName = origFunc })

	tmpName, err := linkFileToTempName(osFile, dirFdForTest(t, fs, dir))
	require.NoError(t, err)
	assert.Equal(t, names[1], tmpName)
	assert.Equal(t, 2, call, "expected exactly one retry after the collision")
	assert.Equal(t, []string{tempLinkNamePrefix, tempLinkNamePrefix}, gotPrefixes)

	// Removing it also asserts where it was created: the link must be an entry
	// of the directory the fd names, which is the whole point of passing a
	// directory fd rather than a path.
	require.NoError(t, os.Remove(filepath.Join(dir, tmpName)))
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
	generateTempLinkName = func(_ string) (string, error) { return collidingName, nil }
	t.Cleanup(func() { generateTempLinkName = origFunc })

	_, err = linkFileToTempName(osFile, dirFdForTest(t, fs, dir))
	require.ErrorIs(t, err, ErrTempNameExhausted)
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

	_, err = linkFileToTempName(osFile, dirFdForTest(t, fs, dir))
	require.Error(t, err)
	assert.ErrorIs(t, err, unix.EPERM)
	assert.Equal(t, 1, calls, "a non-EEXIST linkat error must not be retried")
}

// openFDTargetsUnder returns the set of paths under dir that this process
// currently holds open, read from /proc/self/fd.
//
// It is a set of link targets rather than a count of entries: the Go runtime
// opens and closes descriptors of its own, and os.ReadDir occupies one while it
// runs, so a difference in the number of entries says nothing about the file
// under test.
func openFDTargetsUnder(t *testing.T, dir string) map[string]struct{} {
	t.Helper()
	entries, err := os.ReadDir("/proc/self/fd")
	require.NoError(t, err)

	prefix := dir + string(os.PathSeparator)
	targets := map[string]struct{}{}
	for _, entry := range entries {
		target, err := os.Readlink(filepath.Join("/proc/self/fd", entry.Name()))
		if err != nil {
			// The descriptor was closed between the ReadDir and this readlink
			// (the ReadDir's own is the usual one).
			continue
		}
		if strings.HasPrefix(target, prefix) {
			targets[target] = struct{}{}
		}
	}
	return targets
}

// TestSafeOpenFileFallback_ClosesFDWhenPostCheckFails lives in the Linux-only
// file because it observes descriptors through /proc/self/fd; the function it
// covers is portable and is reached here by calling it directly.
//
// The negative control -- deleting the Close and watching this fail -- must be
// run with GOGC=off. Otherwise a garbage collection can run *os.File's
// finalizer, closing the leaked descriptor before the test looks, and the test
// passes over a real leak.
//
// Both subtests are needed because the two branches release the descriptor
// through different code: the not-created branch closes it inline, the created
// branch hands it to removeVerifiedFileByPath.
func TestSafeOpenFileFallback_ClosesFDWhenPostCheckFails(t *testing.T) {
	cases := []struct {
		name  string
		flag  int
		setup func(t *testing.T, path string)
	}{
		{
			name: "not_created",
			flag: os.O_RDONLY,
			setup: func(t *testing.T, path string) {
				t.Helper()
				require.NoError(t, os.WriteFile(path, []byte("content"), 0o600))
			},
		},
		{
			name:  "created",
			flag:  os.O_CREATE | os.O_WRONLY,
			setup: func(*testing.T, string) {},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := tu.SafeTempDir(t)
			filePath := filepath.Join(dir, "target.txt")
			tc.setup(t, filePath)

			before := openFDTargetsUnder(t, dir)

			stubEnsureParentDirsAfterOpen(t, func(string) error { return errPostCheckFailed })

			file, err := safeOpenFileFallback(filePath, tc.flag, 0o600)
			require.ErrorIs(t, err, errPostCheckFailed)
			assert.Nil(t, file, "no handle may be returned alongside an error")

			assert.Equal(t, before, openFDTargetsUnder(t, dir),
				"the descriptor opened before the failed check must not be left open")
		})
	}
}

func assertCloseOnExec(t *testing.T, fd int, what string) {
	t.Helper()
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	require.NoError(t, err)
	assert.NotZero(t, flags&unix.FD_CLOEXEC, "%s must be opened close-on-exec", what)
}

// TestOpenPrimitives_SetCloseOnExec covers every descriptor this package hands
// out, on both routes, because the flag is set in a different place on each:
// openat2's how.flags, the fallback's unix.Open/unix.Openat, and os.OpenFile,
// which supplies it on its own.
//
// The runner forks and execs commands, so a descriptor of a file the runner
// validated -- or of a directory it holds only to anchor a move -- that survives
// into a child hands that child access nothing gave it.
func TestOpenPrimitives_SetCloseOnExec(t *testing.T) {
	for _, route := range openRoutes {
		t.Run(route.name, func(t *testing.T) {
			fs := newFileSystemForRoute(t, route)
			dir := tu.SafeTempDir(t)
			filePath := filepath.Join(dir, "target.txt")
			require.NoError(t, os.WriteFile(filePath, []byte("content"), 0o600))

			file, err := fs.SafeOpenFile(filePath, os.O_RDONLY, 0)
			require.NoError(t, err)
			t.Cleanup(func() { _ = file.Close() })
			osFile, ok := file.(*os.File)
			require.True(t, ok, "SafeOpenFile must return an *os.File")
			assertCloseOnExec(t, int(osFile.Fd()), "SafeOpenFile")

			dirFd := dirFdForTest(t, fs, dir)
			assertCloseOnExec(t, dirFd, "openDirNoSymlinks")

			osfs, ok := fs.(*osFS)
			require.True(t, ok, "NewFileSystem must return *osFS")
			atFile, err := osfs.openFileAt(dirFd, "target.txt", os.O_RDONLY, 0)
			require.NoError(t, err)
			t.Cleanup(func() { _ = atFile.Close() })
			assertCloseOnExec(t, int(atFile.Fd()), "openFileAt")
		})
	}
}

// TestOpenDirNoSymlinksFallback_ClosesFDWhenPostCheckFails is the directory
// counterpart of the test above, and lives here for the same reason: it observes
// descriptors through /proc/self/fd, while the function it covers is portable
// and is reached here by calling it directly.
//
// A directory fd that outlived its failed check would be worse than a leak. The
// move path anchors renameat and openat to it, so a descriptor handed on after
// the check said the directory had changed would defeat the check entirely.
//
// It also covers the case where the second walk refuses the path outright,
// which is why it asserts the warning as well; the cases where the walk accepts
// the path are in TestOpenDirNoSymlinksFallback_AbandonsOpenWhenDirChanges.
func TestOpenDirNoSymlinksFallback_ClosesFDWhenPostCheckFails(t *testing.T) {
	parent := tu.SafeTempDir(t)
	dir := filepath.Join(parent, "target")
	require.NoError(t, os.Mkdir(dir, 0o750))

	before := openFDTargetsUnder(t, parent)
	recorder := captureWarnings(t)

	stubEnsureDirAfterOpen(t, func(string) (string, error) { return "", errPostCheckFailed })

	fd, err := openDirNoSymlinksFallback(dir)
	require.ErrorIs(t, err, errPostCheckFailed)
	assert.Equal(t, -1, fd, "no descriptor may be returned alongside an error")

	assert.Equal(t, before, openFDTargetsUnder(t, parent),
		"the directory descriptor opened before the failed check must not be left open")

	record := recorder.RequireRecord(t, slog.LevelWarn, dirPostOpenCheckFailedMsg)
	record.AssertAttrs(t, map[string]any{"dir": dir})
}

type openat2SyscallFunc func(dirfd int, pathname string, how *openHow) (int, error)

// stubOpenat2 installs an openat2 stub for the duration of the test. The stub
// is built from the value it displaces, so it can delegate the opens it does
// not care about instead of reimplementing them.
func stubOpenat2(t *testing.T, stub func(realCall openat2SyscallFunc) openat2SyscallFunc) {
	t.Helper()
	realCall := openat2SyscallFunc(openat2SyscallOverride)
	t.Cleanup(func() { openat2SyscallOverride = realCall })
	openat2SyscallOverride = stub(realCall)
}

// TestOpenat2_RetriesOnEINTR stubs the raw system call because EINTR arrives
// from the Go runtime's preemption signal, which a test cannot provoke on
// demand.
//
// The stub interrupts more than once on purpose: a "retry exactly once"
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

// TestOpenat2_NonEINTRErrnoMapping guards the errno mapping that predates the
// EINTR retry, including that these errnos are not swept into the retry loop.
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

// TestOpenat2Mode pins the rule that decides openHow.mode. The directory row
// is here because a caller-level test cannot reach it cheaply; see
// mayCreateFile for why a directory open is at risk of being misread.
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

// TestOpenat2_PassesModeOnlyForCreatingOpens reads openHow.mode as it is
// handed to the system call, so the rule is checked against the kernel
// interface rather than only against openat2Mode's return value.
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

	// Assert the recorded call sequence, not just the mode: an absent key
	// would read back as the zero value and let "mode is 0" pass without the
	// stub having observed anything.
	require.Equal(t, []uint64{0}, modes[existingPath], "an open without O_CREATE must pass mode 0")
	require.Equal(t, []uint64{uint64(perm)}, modes[createdPath], "a creating open must pass the requested permission bits")
	require.Equal(t, []uint64{uint64(perm)}, modes[dir], "an O_TMPFILE open creates a file and must pass the requested permission bits")
}
