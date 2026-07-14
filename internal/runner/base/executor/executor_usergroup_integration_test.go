//go:build integration

// Package executor_test integration coverage for run-as supplementary-group
// handling. This file is named executor_usergroup_integration_test.go (a
// _test.go suffix, as Go requires for anything go test must discover) rather
// than the "integration_skip.go" name floated in the implementation plan --
// that name would not compile as a test at all, since only files ending in
// _test.go are recognized as tests.
package executor_test

import (
	"context"
	"log/slog"
	"os"
	"os/user"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseGroupIDs parses the space-separated numeric GID list printed by
// `id -G` into a slice of ints.
func parseGroupIDs(t *testing.T, out string) []int {
	t.Helper()
	fields := strings.Fields(out)
	ids := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		require.NoErrorf(t, err, "unexpected non-numeric field %q in `id -G` output %q", f, out)
		ids = append(ids, n)
	}
	return ids
}

// userGroupIDs returns the target user's own group IDs (primary + supplementary,
// as reported by the OS user database), converted to ints for comparison
// against parsed `id -G` output.
func userGroupIDs(t *testing.T, u *user.User) []int {
	t.Helper()
	ids, err := u.GroupIds()
	require.NoError(t, err)
	out := make([]int, 0, len(ids))
	for _, s := range ids {
		n, err := strconv.Atoi(s)
		require.NoError(t, err)
		out = append(out, n)
	}
	return out
}

// TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot is a privileged
// integration test: it requires running as root against a real fixture user
// named by TEST_RUNAS_TARGET_USER, so it is gated both by the integration
// build tag (kept out of ordinary test runs) and, once compiled in, by
// canRunPrivilegedIntegrationTest (skipped at runtime when the process is not
// root or the fixture user is not configured/does not exist). It verifies the
// production (non-mock) privilege manager end to end: a run-as command's
// supplementary groups match the target user's own group list and do not
// carry over this (root) process's supplementary groups.
func TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot(t *testing.T) {
	targetUser := os.Getenv("TEST_RUNAS_TARGET_USER")
	if ok, reason := canRunPrivilegedIntegrationTest(os.Getuid(), targetUser); !ok {
		t.Skip(reason)
	}

	u, err := user.Lookup(targetUser)
	require.NoError(t, err)
	wantGroups := userGroupIDs(t, u)

	privMgr := privilege.NewManager(slog.Default())
	exec := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(privMgr),
		executor.WithLogger(slog.Default()),
	)

	idPath := executortestutil.ResolveCommand("id")
	cmd := executortestutil.CreateRuntimeCommand(idPath, []string{"-G"},
		executortestutil.WithWorkDir(""),
		executortestutil.WithRunAsUser(targetUser))

	result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)

	gotGroups := parseGroupIDs(t, result.Stdout)
	assert.ElementsMatch(t, wantGroups, gotGroups,
		"run-as child's supplementary groups must match the target user's own group list exactly")

	rootGroups, err := os.Getgroups()
	require.NoError(t, err)
	for _, rg := range rootGroups {
		if !slices.Contains(wantGroups, rg) {
			assert.NotContains(t, gotGroups, rg,
				"run-as child must not carry over this process's own supplementary group %d", rg)
		}
	}
}
