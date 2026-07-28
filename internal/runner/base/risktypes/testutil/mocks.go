//go:build test

// Package risktypestestutil provides shared test stubs for run-as identity
// resolution, so the executor (execution path) and DryRunResourceManager
// (dry-run/preview path) can exercise the same set of resolver behaviours and
// prove that their fail-closed judgments agree.
package risktypestestutil

import (
	"os/user"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
)

const (
	mockUID      = 1000
	mockGID      = 1000
	mockGroupID1 = 27
)

// StubResolveRunAsIdentSuccess returns a fixed, fully-resolved identity.
var StubResolveRunAsIdentSuccess risktypes.RunAsResolver = func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) {
	return risktypes.RunAsIdent{UID: mockUID, GID: mockGID, Groups: []uint32{mockUID, mockGroupID1}}, nil
}

// StubResolveRunAsIdentUnknownUser reports the user name as unresolvable.
// The userName parameter is embedded so each test case can use a distinct name
// without declaring a fresh resolver.
var StubResolveRunAsIdentUnknownUser risktypes.RunAsResolver = func(_ risktypes.RunAsIdent, userName, _ string) (risktypes.RunAsIdent, error) {
	return risktypes.RunAsIdent{}, user.UnknownUserError(userName)
}

// StubResolveRunAsIdentUnknownGroup reports the group name as unresolvable.
var StubResolveRunAsIdentUnknownGroup risktypes.RunAsResolver = func(_ risktypes.RunAsIdent, _, groupName string) (risktypes.RunAsIdent, error) {
	return risktypes.RunAsIdent{}, user.UnknownGroupError(groupName)
}

// StubResolveRunAsIdentNilGroups resolves the name(s) but reports that
// supplementary groups could not be enumerated (Groups == nil), reproducing a
// u.GroupIds() failure without depending on real OS state.
var StubResolveRunAsIdentNilGroups risktypes.RunAsResolver = func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) {
	return risktypes.RunAsIdent{UID: mockUID, GID: mockGID, Groups: nil}, nil
}
