package executor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor"
	executortestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/executor/testutil"
	privilegetestutil "github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolverCall records a single invocation of a run-as resolver: the base
// identity it started from and the user/group names it was asked to resolve.
type resolverCall struct {
	base      risktypes.RunAsIdent
	userName  string
	groupName string
}

// capturingResolver returns a resolver that records every call it receives and
// always resolves to ident, so a test can assert on the base identity and
// user/group names the executor passed in. It does not prove what the executor
// does with ident afterward (building SysProcAttr.Credential and reaching
// execve is exercised end-to-end only in the privileged integration tests);
// the mock privilege manager's elevation log records only the RunAsUser/
// RunAsGroup strings, not the resolved uid/gid/groups.
func capturingResolver(calls *[]resolverCall, ident risktypes.RunAsIdent) func(risktypes.RunAsIdent, string, string) (risktypes.RunAsIdent, error) {
	return func(base risktypes.RunAsIdent, userName, groupName string) (risktypes.RunAsIdent, error) {
		*calls = append(*calls, resolverCall{base: base, userName: userName, groupName: groupName})
		return ident, nil
	}
}

// TestExecuteWithUserGroup_ResolverArgs_ThreeForms verifies that, for each of the
// three run_as forms (user only, group only, both), the executor calls the run-as
// resolver with the shared original-execution-identity base and the exact
// user/group names from the command -- the wiring that feeds the kernel-level
// Credential. This proves only the inputs to the resolver; the resolver's own
// output semantics (target uid/gid/supplementary-groups per form, including
// group-only inheriting supplementary groups from the base rather than the
// named group) are verified against the real OS user database in
// risktypes/runas_ident_test.go, and are not re-verified here.
func TestExecuteWithUserGroup_ResolverArgs_ThreeForms(t *testing.T) {
	tests := []struct {
		name          string
		runAsUser     string
		runAsGroup    string
		wantUserName  string
		wantGroupName string
	}{
		{name: "user_only", runAsUser: "testuser", wantUserName: "testuser"},
		{name: "group_only", runAsGroup: "testgroup", wantGroupName: "testgroup"},
		{name: "user_and_group", runAsUser: "testuser", runAsGroup: "testgroup", wantUserName: "testuser", wantGroupName: "testgroup"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []resolverCall
			resolvedIdent := risktypes.RunAsIdent{UID: 5001, GID: 6001, Groups: []uint32{7001, 7002}}

			mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
			exec := executor.NewDefaultExecutor(
				executor.WithPrivilegeManager(mockPriv),
				executor.WithFileSystem(&executortestutil.MockFileSystem{}),
				executor.WithRunAsResolver(capturingResolver(&calls, resolvedIdent)),
			)

			cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"},
				executortestutil.WithWorkDir(""),
				executortestutil.WithRunAsUser(tt.runAsUser),
				executortestutil.WithRunAsGroup(tt.runAsGroup))

			// The command may succeed (running as root in CI) or fail with EPERM
			// (unprivileged CAP_SETUID/CAP_SETGID); either way the resolver must
			// have been consulted exactly once with the right arguments before the
			// kernel-level credential was attempted.
			_, _ = exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

			require.Len(t, calls, 1, "resolver should be called exactly once")
			assert.Equal(t, risktypes.OriginalExecutionIdentity(), calls[0].base,
				"resolver base must be the shared original-execution-identity cache, not a freshly re-read identity")
			assert.Equal(t, tt.wantUserName, calls[0].userName)
			assert.Equal(t, tt.wantGroupName, calls[0].groupName)

			assert.Contains(t, mockPriv.ElevationCalls, "user_group_change:"+tt.runAsUser+":"+tt.runAsGroup,
				"privilege escalation must be requested for the resolved command")
		})
	}
}

// TestExecuteWithUserGroup_ResolverError_FailsClosed verifies that when the
// run-as resolver fails (unknown user/group), the command is never executed and
// no privilege escalation is even attempted: resolution happens before
// WithPrivileges is called, so a fail-closed error here must leave the mock
// privilege manager's elevation log empty.
func TestExecuteWithUserGroup_ResolverError_FailsClosed(t *testing.T) {
	resolverErr := errors.New("unknown run-as user")
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	exec := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(mockPriv),
		executor.WithFileSystem(&executortestutil.MockFileSystem{}),
		executor.WithRunAsResolver(func(risktypes.RunAsIdent, string, string) (risktypes.RunAsIdent, error) {
			return risktypes.RunAsIdent{}, resolverErr
		}),
	)

	cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"},
		executortestutil.WithWorkDir(""),
		executortestutil.WithRunAsUser("nosuchuser"),
		executortestutil.WithRunAsGroup("nosuchgroup"))

	result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, executor.ErrRunAsIdentityResolution)
	assert.Nil(t, result, "the command must not run, so there is no result to report")
	assert.Empty(t, mockPriv.ElevationCalls, "privilege escalation must not be attempted when identity resolution fails")
}

// TestExecuteWithUserGroup_ResolverNilGroups_FailsClosed verifies that a
// resolver reporting success but with a nil Groups slice (the signal that
// supplementary-group enumeration failed) is treated as a fail-closed error
// rather than silently passing a nil group list into the process credential.
func TestExecuteWithUserGroup_ResolverNilGroups_FailsClosed(t *testing.T) {
	mockPriv := privilegetestutil.NewMockPrivilegeManager(true)
	exec := executor.NewDefaultExecutor(
		executor.WithPrivilegeManager(mockPriv),
		executor.WithFileSystem(&executortestutil.MockFileSystem{}),
		executor.WithRunAsResolver(func(risktypes.RunAsIdent, string, string) (risktypes.RunAsIdent, error) {
			return risktypes.RunAsIdent{UID: 1000, GID: 1000, Groups: nil}, nil
		}),
	)

	cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"},
		executortestutil.WithWorkDir(""),
		executortestutil.WithRunAsUser("testuser"),
		executortestutil.WithRunAsGroup("testgroup"))

	result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, executor.ErrRunAsIdentityResolution)
	assert.Nil(t, result)
	assert.Empty(t, mockPriv.ElevationCalls, "privilege escalation must not be attempted when supplementary groups could not be enumerated")
}

// TestExecuteWithUserGroup_NoRunAs_ResolverNotInvoked verifies that a command
// with no run_as user/group takes the normal execution path and never
// consults the run-as resolver, so no supplementary-group resolution or
// kernel credential is attempted for commands that never asked for one.
func TestExecuteWithUserGroup_NoRunAs_ResolverNotInvoked(t *testing.T) {
	resolverCalled := false
	exec := executor.NewDefaultExecutor(
		executor.WithFileSystem(&executortestutil.MockFileSystem{}),
		executor.WithRunAsResolver(func(risktypes.RunAsIdent, string, string) (risktypes.RunAsIdent, error) {
			resolverCalled = true
			return risktypes.RunAsIdent{}, nil
		}),
	)

	cmd := executortestutil.CreateRuntimeCommand(echoCmd, []string{"test"}, executortestutil.WithWorkDir(""))

	result, err := exec.Execute(context.Background(), nil, cmd, map[string]string{}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 0, result.ExitCode)
	assert.False(t, resolverCalled, "the run-as resolver must not be consulted for commands without run_as")
}
