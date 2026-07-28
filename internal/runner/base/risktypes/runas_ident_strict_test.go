//go:build test

package risktypes_test

import (
	"errors"
	"os/user"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errResolverForced = errors.New("resolver failure injected by test")

// nilGroupsResolver returns a RunAsResolver that succeeds but with Groups == nil,
// simulating a supplementary-group enumeration failure.
func nilGroupsResolver(t *testing.T) risktypes.RunAsResolver {
	t.Helper()
	return func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) {
		u, err := user.Current()
		require.NoError(t, err)
		return risktypes.RunAsIdent{UID: parseID(t, u.Uid), GID: parseID(t, u.Gid), Groups: nil}, nil
	}
}

// TestResolveRunAsIdentStrict_ResolverError: when the resolver returns an error,
// ErrRunAsIdentityResolution wraps it and the underlying error is still reachable
// via errors.Is.
func TestResolveRunAsIdentStrict_ResolverError(t *testing.T) {
	resolver := func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) {
		return risktypes.RunAsIdent{}, errResolverForced
	}
	_, err := risktypes.ResolveRunAsIdentStrict(resolver, risktypes.OriginalExecutionIdentity(), "testuser", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, risktypes.ErrRunAsIdentityResolution)
	assert.ErrorIs(t, err, errResolverForced, "the injected error is still reachable")
	assert.NotErrorIs(t, err, risktypes.ErrRunAsSupplementaryGroupsUnavailable, "supplementary-groups sentinel should not appear in resolver-error path")
}

// TestResolveRunAsIdentStrict_NilGroups: when the resolver succeeds but returns
// Groups == nil, both ErrRunAsIdentityResolution and
// ErrRunAsSupplementaryGroupsUnavailable are wrapped.
func TestResolveRunAsIdentStrict_NilGroups(t *testing.T) {
	u, err := user.Current()
	require.NoError(t, err)

	_, err = risktypes.ResolveRunAsIdentStrict(nilGroupsResolver(t), risktypes.RunAsIdent{}, u.Username, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, risktypes.ErrRunAsIdentityResolution)
	assert.ErrorIs(t, err, risktypes.ErrRunAsSupplementaryGroupsUnavailable)
	assert.NotErrorIs(t, err, errResolverForced, "resolver error path should not contaminate nil-groups path")
}

// TestResolveRunAsIdentStrict_Success: a successful resolver returns the ident
// unchanged through ResolveRunAsIdentStrict.
func TestResolveRunAsIdentStrict_Success(t *testing.T) {
	u, err := user.Current()
	require.NoError(t, err)

	expected := risktypes.RunAsIdent{UID: parseID(t, u.Uid), GID: parseID(t, u.Gid), Groups: []uint32{4242}}
	resolver := func(_ risktypes.RunAsIdent, _, _ string) (risktypes.RunAsIdent, error) {
		return expected, nil
	}
	base := risktypes.OriginalExecutionIdentity()
	ident, err := risktypes.ResolveRunAsIdentStrict(resolver, base, u.Username, "")
	require.NoError(t, err)
	assert.Equal(t, expected, ident)
}

// TestResolveRunAsIdentStrict_NilResolverUsesDefault: passing nil for the resolver
// falls back to the default production resolver (ResolveRunAsIdent).
func TestResolveRunAsIdentStrict_NilResolverUsesDefault(t *testing.T) {
	u, err := user.Current()
	require.NoError(t, err)

	base := risktypes.OriginalExecutionIdentity()
	ident, err := risktypes.ResolveRunAsIdentStrict(nil, base, u.Username, "")
	require.NoError(t, err)
	assert.Equal(t, parseID(t, u.Uid), ident.UID)
	assert.NotEmpty(t, ident.Groups)
}

// TestResolveRunAsIdentStrict_ArgumentForms: the three argument forms defined in
// the architecture design produce the expected resolved RunAsIdent values via
// ResolveRunAsIdentStrict with the default resolver.
func TestResolveRunAsIdentStrict_ArgumentForms(t *testing.T) {
	u, err := user.Current()
	require.NoError(t, err)

	g, distinct := overrideGroup(t, u)

	base := risktypes.OriginalExecutionIdentity()

	t.Run("user_only", func(t *testing.T) {
		ident, err := risktypes.ResolveRunAsIdentStrict(nil, base, u.Username, "")
		require.NoError(t, err)
		assert.Equal(t, parseID(t, u.Uid), ident.UID)
		assert.Equal(t, parseID(t, u.Gid), ident.GID)
		assert.NotEmpty(t, ident.Groups)
	})

	t.Run("user_and_group", func(t *testing.T) {
		ident, err := risktypes.ResolveRunAsIdentStrict(nil, base, u.Username, g.Name)
		require.NoError(t, err)
		assert.Equal(t, parseID(t, u.Uid), ident.UID)
		assert.Equal(t, parseID(t, g.Gid), ident.GID)
		if distinct {
			assert.NotEqual(t, parseID(t, u.Gid), ident.GID)
		}
	})

	t.Run("group_only", func(t *testing.T) {
		ident, err := risktypes.ResolveRunAsIdentStrict(nil, base, "", g.Name)
		require.NoError(t, err)
		assert.Equal(t, base.UID, ident.UID, "uid stays the base identity (original execution identity)")
		assert.Equal(t, base.Groups, ident.Groups, "supplementary groups stay the base identity")
		assert.Equal(t, parseID(t, g.Gid), ident.GID)
	})
}
