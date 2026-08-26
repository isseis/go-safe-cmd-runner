//go:build !cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations.
// This file provides fallback implementations when CGO is disabled by parsing /etc/group.
package groupmembership

import (
	"fmt"
	"strings"
)

// userDatabaseSource names the user database this build consults for user
// lookups. Without cgo, os/user parses /etc/passwd only; directory-backed
// sources such as LDAP or SSSD are not consulted.
const userDatabaseSource = "passwd-file"

// getGroupMembers returns all members of a group given its GID by parsing /etc/group
// and /etc/passwd to find users with this GID as their primary group
// This is a stateless function - caching is handled by the GroupMembership struct
func getGroupMembers(gid uint32) (groupEnumeration, error) {
	groupEntry, err := findGroupByGID(gid)
	if err != nil {
		return groupEnumeration{}, err
	}
	if groupEntry == nil {
		return groupEnumeration{members: []string{}, verdict: completeVerdict()}, nil // Group not found
	}

	// Start with explicit members from /etc/group
	memberSet := make(map[string]struct{})
	if groupEntry.members != "" {
		for member := range strings.SplitSeq(groupEntry.members, ",") {
			member = strings.TrimSpace(member)
			if member != "" {
				memberSet[member] = struct{}{}
			}
		}
	}

	// Find users with this GID as their primary group by parsing /etc/passwd
	primaryUsers, err := findUsersWithPrimaryGID(gid)
	if err != nil {
		return groupEnumeration{}, fmt.Errorf("failed to find users with primary GID %d: %w", gid, err)
	}

	// Add primary group users to the member set
	for _, user := range primaryUsers {
		memberSet[user] = struct{}{}
	}

	// Convert map to slice
	result := make([]string, 0, len(memberSet))
	for member := range memberSet {
		result = append(result, member)
	}

	return groupEnumeration{members: result, verdict: completeVerdict()}, nil
}

// precomputeEnumerationEnvironment resolves whatever environment facts this
// build needs before the first enumeration, so that a build unable to
// enumerate every member says so at startup rather than at the first
// group-writable file.
func precomputeEnumerationEnvironment() {
}
