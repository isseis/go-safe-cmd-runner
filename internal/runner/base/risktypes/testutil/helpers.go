//go:build test

package risktypestestutil

import (
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
)

// RunAsResolutionCase is a single entry in the shared resolution-case table
// that both the executor and dry-run tests read, so the two paths are always
// exercising the same set of resolver inputs and the fail-closed judgments
// cannot diverge by one test adding a case the other forgets.
type RunAsResolutionCase struct {
	Name        string
	UserName    string
	GroupName   string
	Resolver    risktypes.RunAsResolver
	WantFailure bool
}

// RunAsResolutionCases returns the four standard cases that the tests use to
// verify that dry-run validation and execution-time resolution agree:
// success, unknown user, unknown group, and supplementary group
// enumeration failure.
func RunAsResolutionCases() []RunAsResolutionCase {
	return []RunAsResolutionCase{
		{
			Name:        "success",
			UserName:    "testuser",
			GroupName:   "testgroup",
			Resolver:    StubResolveRunAsIdentSuccess,
			WantFailure: false,
		},
		{
			Name:        "user_unknown",
			UserName:    "nonexistent_user",
			GroupName:   "testgroup",
			Resolver:    StubResolveRunAsIdentUnknownUser,
			WantFailure: true,
		},
		{
			Name:        "group_unknown",
			UserName:    "testuser",
			GroupName:   "nonexistent_group",
			Resolver:    StubResolveRunAsIdentUnknownGroup,
			WantFailure: true,
		},
		{
			Name:        "supplementary_groups_unavailable",
			UserName:    "testuser",
			GroupName:   "testgroup",
			Resolver:    StubResolveRunAsIdentNilGroups,
			WantFailure: true,
		},
	}
}
