//go:build test

package risktypes_test

import (
	"os"
	"os/user"
	"strconv"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseID parses a decimal id string into uint32 for comparison against a
// resolved identity. It fails the test on a malformed id rather than returning an
// error, since the OS user database is the ground truth here.
func parseID(t *testing.T, s string) uint32 {
	t.Helper()
	n, err := strconv.ParseUint(s, 10, 32)
	require.NoError(t, err)
	return uint32(n)
}

// overrideGroup returns a group to use for gid-override assertions, preferring one
// whose gid differs from the current process primary gid so the override is proven
// (not a tautology). It falls back to the user's primary group when no distinct
// supplementary group exists, returning distinct=false so the caller can relax the
// "differs from primary" assertion on such hosts.
func overrideGroup(t *testing.T, u *user.User) (g *user.Group, distinct bool) {
	t.Helper()
	gid := u.Gid
	if ids, err := u.GroupIds(); err == nil {
		for _, id := range ids {
			if id != u.Gid {
				gid, distinct = id, true
				break
			}
		}
	}
	g, err := user.LookupGroupId(gid)
	require.NoError(t, err)
	return g, distinct
}

// TestResolveRunAsIdent_UserOnly: resolving a known user (the current process
// user) yields that user's uid, primary gid, and a non-empty supplementary group
// set -- exercising the production OS-user-database lookup, not the injected fake.
func TestResolveRunAsIdent_UserOnly(t *testing.T) {
	u, err := user.Current()
	require.NoError(t, err)

	ident, err := risktypes.ResolveRunAsIdent(risktypes.OriginalExecutionIdentity(), u.Username, "")
	require.NoError(t, err)
	assert.Equal(t, parseID(t, u.Uid), ident.UID, "uid follows the named user")
	assert.Equal(t, parseID(t, u.Gid), ident.GID, "gid is the user's primary group")
	assert.NotEmpty(t, ident.Groups, "supplementary groups are enumerated")
}

// TestResolveRunAsIdent_GroupOnly: resolving a group with no user keeps the base
// identity (uid and supplementary groups) and overrides only the gid. A sentinel
// base uid (distinct from the live process uid) proves the group-only form uses the
// passed-in base rather than re-reading the live process identity at resolve time.
func TestResolveRunAsIdent_GroupOnly(t *testing.T) {
	u, err := user.Current()
	require.NoError(t, err)
	g, distinct := overrideGroup(t, u)

	base := risktypes.RunAsIdent{UID: uint32(os.Getuid()) + 4242, GID: uint32(os.Getgid()), Groups: []uint32{4242}}
	ident, err := risktypes.ResolveRunAsIdent(base, "", g.Name)
	require.NoError(t, err)
	assert.Equal(t, base.UID, ident.UID, "uid stays the base identity, not the live process uid")
	assert.Equal(t, base.Groups, ident.Groups, "supplementary groups stay the base identity")
	assert.Equal(t, parseID(t, g.Gid), ident.GID, "gid is overridden by the named group")
	if distinct {
		assert.NotEqual(t, uint32(os.Getgid()), ident.GID, "the override actually changed the gid")
	}
}

// TestResolveRunAsIdent_UserAndGroup: when both are set, the gid from the named
// group overrides the user's primary gid, while the uid is the named user's.
func TestResolveRunAsIdent_UserAndGroup(t *testing.T) {
	u, err := user.Current()
	require.NoError(t, err)
	g, distinct := overrideGroup(t, u)

	ident, err := risktypes.ResolveRunAsIdent(risktypes.OriginalExecutionIdentity(), u.Username, g.Name)
	require.NoError(t, err)
	assert.Equal(t, parseID(t, u.Uid), ident.UID, "uid follows the named user")
	assert.Equal(t, parseID(t, g.Gid), ident.GID, "gid is overridden by the named group")
	if distinct {
		assert.NotEqual(t, parseID(t, u.Gid), ident.GID, "the named group overrode the user's primary gid")
	}
}

// TestResolveRunAsIdent_UnknownUser: an unresolvable user name returns an error
// so the caller fails closed rather than trusting an unknown identity.
func TestResolveRunAsIdent_UnknownUser(t *testing.T) {
	_, err := risktypes.ResolveRunAsIdent(risktypes.OriginalExecutionIdentity(), "no_such_user_0142_axis2", "")
	require.Error(t, err)
}

// TestResolveRunAsIdent_UnknownGroup: an unresolvable group name returns an error
// (fail-closed), even when the user resolves.
func TestResolveRunAsIdent_UnknownGroup(t *testing.T) {
	u, err := user.Current()
	require.NoError(t, err)
	_, err = risktypes.ResolveRunAsIdent(risktypes.OriginalExecutionIdentity(), u.Username, "no_such_group_0142_axis2")
	require.Error(t, err)
}

// TestOriginalExecutionIdentity: the startup default identity reflects the
// process's real uid/gid (captured at construction, never the zero value).
func TestOriginalExecutionIdentity(t *testing.T) {
	ident := risktypes.OriginalExecutionIdentity()
	assert.Equal(t, uint32(os.Getuid()), ident.UID)
	assert.Equal(t, uint32(os.Getgid()), ident.GID)
}
