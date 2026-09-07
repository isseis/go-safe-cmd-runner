//go:build integration

// Package executor_test integration coverage for run-as supplementary-group
// handling. This file is named executor_usergroup_integration_test.go (a
// _test.go suffix, as Go requires for anything go test must discover) rather
// than the "integration_skip.go" name floated in the implementation plan --
// that name would not compile as a test at all, since only files ending in
// _test.go are recognized as tests.
//
// Build/run requires BOTH tags, not just "integration": this file imports
// the executor/testutil helper package, which itself is gated by
// //go:build test || performance. Compiling with only -tags integration
// fails with an unrelated-looking "build constraints exclude all Go files"
// error from the testutil package. Use:
//
//	go test -tags "test integration" ./internal/runner/base/executor/...
package executor_test

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"

	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot exercises the same
// setuid entry model as the privilege-gap tests. The shared fixture rejects
// native-root execution and de-escalates to the non-root invoker before the
// production privilege manager starts the child.
func TestRunAsSupplementaryGroups_MatchTargetUser_NotRoot(t *testing.T) {
	requireSetuidModel(t)
	fixture := newSetuidFixture(t)
	idPath := executortestutil.ResolveCommand("id")
	cmd := executortestutil.CreateRuntimeCommand(idPath, []string{"-G"},
		executortestutil.WithWorkDir(""),
		executortestutil.WithRunAsUser(fixture.target.Username))

	result, err := fixture.executor.Execute(context.Background(), nil, cmd, map[string]string{}, nil)
	require.NoError(t, err)
	require.Equal(t, 0, result.ExitCode)

	gotGroups := parseNumericIDs(t, strings.Fields(result.Stdout))
	assert.ElementsMatch(t, fixture.targetGroups, gotGroups,
		"run-as child's supplementary groups must match the target user's own group list exactly")

	invokerGroups, err := os.Getgroups()
	require.NoError(t, err)
	for _, group := range invokerGroups {
		if !slices.Contains(fixture.targetGroups, group) {
			assert.NotContains(t, gotGroups, group,
				"run-as child must not carry over the invoker's supplementary group %d", group)
		}
	}
}
