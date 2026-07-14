//go:build test

package executor

import (
	"log/slog"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/stretchr/testify/require"
)

// openVerifiedIdentityForTest opens path read-only and wraps it as a
// VerifiedIdentity, mirroring what the risk evaluator produces. The caller
// owns the returned identity's FD and must Close it.
func openVerifiedIdentityForTest(t *testing.T, path string) *risktypes.VerifiedIdentity {
	t.Helper()
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	require.NoError(t, err)
	return &risktypes.VerifiedIdentity{
		FD:           risktypes.NewVerifiedFD(fd),
		ResolvedPath: path,
		ContentHash:  "sha256:test",
	}
}

// scrStageDirs lists the "scr-stage-*" staging directories currently present
// under os.TempDir(), used to detect leaks left behind by a failed
// stageFromFD call.
func scrStageDirs(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	require.NoError(t, err)
	var dirs []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "scr-stage-") {
			dirs = append(dirs, e.Name())
		}
	}
	return dirs
}

// TestStageFromFD_ChownFailure_CleansUpStagingDir verifies that when chgrp'ing
// the staging directory fails (e.g. the caller lacks permission to assign the
// requested gid), stageFromFD removes the staging directory it created rather
// than leaking it.
func TestStageFromFD_ChownFailure_CleansUpStagingDir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Skipping EPERM assertion when running as root")
	}

	e := &DefaultExecutor{Logger: slog.Default()}

	identity := openVerifiedIdentityForTest(t, "/bin/echo")
	defer func() { _ = identity.FD.Close() }()

	before := scrStageDirs(t)

	// A gid the test process does not belong to: os.Chown to it fails with
	// EPERM for a non-root caller.
	cred := &syscall.Credential{Gid: 65534}

	_, _, err := e.stageFromFD(identity, cred)
	require.Error(t, err)

	after := scrStageDirs(t)
	require.ElementsMatch(t, before, after, "stageFromFD must not leak its staging directory on chown failure")
}

// TestStageFromFD_OpenFileFailure_CleansUpStagingDir verifies that when
// creating the staged file fails (here, because the basename derived from
// the verified path exceeds the filesystem's name length limit),
// stageFromFD removes the staging directory it created rather than leaking
// it.
func TestStageFromFD_OpenFileFailure_CleansUpStagingDir(t *testing.T) {
	e := &DefaultExecutor{Logger: slog.Default()}

	// filepath.Base(identity.ResolvedPath) becomes the staged file's name; a
	// component this long makes os.OpenFile fail with ENAMETOOLONG on Linux
	// (NAME_MAX is 255 bytes) regardless of privilege.
	longName := strings.Repeat("a", 300)
	identity := openVerifiedIdentityForTest(t, "/bin/echo")
	defer func() { _ = identity.FD.Close() }()
	identity.ResolvedPath = "/tmp/" + longName

	before := scrStageDirs(t)

	_, _, err := e.stageFromFD(identity, nil)
	require.Error(t, err)

	after := scrStageDirs(t)
	require.ElementsMatch(t, before, after, "stageFromFD must not leak its staging directory on staged-file creation failure")
}
