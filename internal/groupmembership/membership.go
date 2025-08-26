package groupmembership

import (
	"fmt"
	"os/user"
	"slices"
	"strconv"
)

// IsCurrentUserOnlyGroupMember implements the common logic for checking if:
// 1. Current user is the file owner
// 2. Current user is a member of the file's group
// 3. Current user is the ONLY member of the file's group
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
	groupMembers, err := getGroupMembers(fileGID)
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
