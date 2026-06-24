package risk

import (
	"fmt"
	"os"
	"os/user"
	"strconv"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
)

// originalExecutionIdentity returns the identity of the current process. It is the
// default run-as identity for commands that set no run_as_user/run_as_group, and
// is resolved once at evaluator construction (the embedding layer). The zoning
// judgment itself never reads live identity; it consumes only this precomputed
// value. The zero value (uid 0) is never used as an implicit "unset" default,
// which is why this is resolved explicitly.
func originalExecutionIdentity() risktypes.RunAsIdent {
	// #nosec G115 -- safe: os.Getuid/Getgid/Getgroups return system IDs, which are
	// non-negative and fit in uint32 on all supported platforms.
	ident := risktypes.RunAsIdent{
		UID: uint32(os.Getuid()),
		GID: uint32(os.Getgid()),
	}
	if gids, err := syscall.Getgroups(); err == nil {
		ident.Groups = make([]uint32, 0, len(gids))
		for _, g := range gids {
			ident.Groups = append(ident.Groups, uint32(g)) // #nosec G115 -- system GID, non-negative and fits uint32
		}
	}
	return ident
}

// resolveRunAsIdent resolves a run-as user/group name pair to a RunAsIdent via the
// OS user database (the same lookups the privilege manager uses). It is the
// production runAsResolver. A failure (unknown user or group) is returned so the
// caller fails closed rather than trusting an unresolved identity.
//
// Forms: user only -> the user's uid/primary gid/supplementary groups; group only
// -> the current identity with the gid overridden; both -> the user's identity
// with the gid overridden by the named group.
func resolveRunAsIdent(userName, groupName string) (risktypes.RunAsIdent, error) {
	ident := originalExecutionIdentity()

	if userName != "" {
		u, err := user.Lookup(userName)
		if err != nil {
			return risktypes.RunAsIdent{}, fmt.Errorf("resolve run-as user %q: %w", userName, err)
		}
		uid, err := strconv.ParseUint(u.Uid, 10, 32)
		if err != nil {
			return risktypes.RunAsIdent{}, fmt.Errorf("parse uid for run-as user %q: %w", userName, err)
		}
		gid, err := strconv.ParseUint(u.Gid, 10, 32)
		if err != nil {
			return risktypes.RunAsIdent{}, fmt.Errorf("parse primary gid for run-as user %q: %w", userName, err)
		}
		ident.UID = uint32(uid)
		ident.GID = uint32(gid)
		ident.Groups = supplementaryGroups(u)
	}

	if groupName != "" {
		g, err := user.LookupGroup(groupName)
		if err != nil {
			return risktypes.RunAsIdent{}, fmt.Errorf("resolve run-as group %q: %w", groupName, err)
		}
		gid, err := strconv.ParseUint(g.Gid, 10, 32)
		if err != nil {
			return risktypes.RunAsIdent{}, fmt.Errorf("parse gid for run-as group %q: %w", groupName, err)
		}
		ident.GID = uint32(gid)
	}

	return ident, nil
}

// supplementaryGroups returns the user's supplementary group GIDs, or nil when
// they cannot be enumerated (the Trusted predicate then sees only the primary GID,
// which is the conservative direction).
func supplementaryGroups(u *user.User) []uint32 {
	ids, err := u.GroupIds()
	if err != nil {
		return nil
	}
	groups := make([]uint32, 0, len(ids))
	for _, s := range ids {
		if n, err := strconv.ParseUint(s, 10, 32); err == nil {
			groups = append(groups, uint32(n))
		}
	}
	return groups
}
