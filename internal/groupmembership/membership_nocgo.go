//go:build !cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations.
// This file provides fallback implementations when CGO is disabled by parsing /etc/group.
package groupmembership

import (
	"fmt"
	"strings"
)

// getGroupMembers returns all members of a group given its GID by parsing /etc/group
// and /etc/passwd to find users with this GID as their primary group
// This is a stateless function - caching is handled by the GroupMembership struct
func getGroupMembers(gid uint32) ([]string, error) {
	groupEntry, err := findGroupByGID(gid)
	if err != nil {
		return nil, err
	}
	if groupEntry == nil {
		return []string{}, nil // Group not found
	}

	// Start with explicit members from /etc/group
	memberSet := make(map[string]struct{})
	if groupEntry.members != "" {
		members := strings.Split(groupEntry.members, ",")
		for _, member := range members {
			member = strings.TrimSpace(member)
			if member != "" {
				memberSet[member] = struct{}{}
			}
		}
	}

	// Find users with this GID as their primary group by parsing /etc/passwd
	primaryUsers, err := findUsersWithPrimaryGID(gid)
	if err != nil {
		return nil, fmt.Errorf("failed to find users with primary GID %d: %w", gid, err)
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

	return result, nil
}
