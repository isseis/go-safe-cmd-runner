//go:build !cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations.
// This file provides fallback implementations when CGO is disabled by parsing /etc/group.
package groupmembership

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"slices"
	"strconv"
	"strings"
)

// GetGroupMembers returns all members of a group given its GID by parsing /etc/group
func GetGroupMembers(gid uint32) ([]string, error) {
	groupEntry, err := findGroupByGID(gid)
	if err != nil {
		return nil, err
	}
	if groupEntry == nil {
		return []string{}, nil // Group not found
	}

	// Parse members from the group entry
	if groupEntry.members == "" {
		return []string{}, nil
	}

	members := strings.Split(groupEntry.members, ",")
	// Filter out empty strings
	result := make([]string, 0, len(members))
	for _, member := range members {
		member = strings.TrimSpace(member)
		if member != "" {
			result = append(result, member)
		}
	}
	return result, nil
}

// IsCurrentUserOnlyGroupMember checks if the current user is the only member of the file's group
// by parsing /etc/group when CGO is disabled
func IsCurrentUserOnlyGroupMember(fileUID, fileGID uint32) (bool, error) {
	// Get current user
	currentUser, err := user.Current()
	if err != nil {
		return false, fmt.Errorf("failed to get current user: %w", err)
	}

	// Check if current user is the file owner
	currentUID, err := strconv.ParseUint(currentUser.Uid, 10, 32)
	if err != nil {
		return false, fmt.Errorf("failed to parse current user UID: %w", err)
	}

	if uint32(currentUID) != fileUID {
		return false, nil // Not the file owner
	}

	// Get user's group memberships
	groupIDs, err := currentUser.GroupIds()
	if err != nil {
		return false, fmt.Errorf("failed to get user group memberships: %w", err)
	}

	// Check if user is member of the file's group
	fileGidStr := strconv.FormatUint(uint64(fileGID), 10)
	isUserInGroup := slices.Contains(groupIDs, fileGidStr) || currentUser.Gid == fileGidStr

	if !isUserInGroup {
		return false, nil // User is not in the file's group
	}

	// Get all members of the file's group
	groupMembers, err := GetGroupMembers(fileGID)
	if err != nil {
		return false, fmt.Errorf("failed to get group members: %w", err)
	}

	// Check if current user is the only member
	if len(groupMembers) == 0 {
		// Group has no explicit members, only primary group users
		// For simplicity, we'll allow this case if it's the user's primary group
		return currentUser.Gid == fileGidStr, nil
	}

	if len(groupMembers) == 1 && groupMembers[0] == currentUser.Username {
		return true, nil
	}

	// More than one member or the only member is not the current user
	return false, nil
}

// groupEntry represents a parsed line from /etc/group
type groupEntry struct {
	name    string
	gid     uint32
	members string
}

// findGroupByGID searches for a group entry in /etc/group by GID
func findGroupByGID(gid uint32) (*groupEntry, error) {
	file, err := os.Open("/etc/group")
	if err != nil {
		return nil, fmt.Errorf("failed to open /etc/group: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		entry, err := parseGroupLine(line)
		if err != nil {
			continue // Skip malformed lines
		}

		if entry.gid == gid {
			return entry, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading /etc/group: %w", err)
	}

	return nil, nil // Group not found
}

// parseGroupLine parses a single line from /etc/group
// Format: groupname:password:gid:member1,member2,member3
func parseGroupLine(line string) (*groupEntry, error) {
	fields := strings.Split(line, ":")
	if len(fields) < 4 {
		return nil, fmt.Errorf("invalid group line format")
	}

	gid, err := strconv.ParseUint(fields[2], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("invalid GID: %w", err)
	}

	return &groupEntry{
		name:    fields[0],
		gid:     uint32(gid),
		members: fields[3],
	}, nil
}
