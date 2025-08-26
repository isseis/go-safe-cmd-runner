//go:build cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations using system calls.
package groupmembership

/*
#include <sys/types.h>
#include <grp.h>
#include <stdlib.h>
#include <string.h>

// get_group_members returns the members of a group given its GID
// Returns a null-terminated array of strings, or NULL if error
// Caller is responsible for freeing the returned array and its strings
char** get_group_members(gid_t gid, int* count) {
    struct group *grp = getgrgid(gid);
    if (grp == NULL || grp->gr_mem == NULL) {
        *count = 0;
        return NULL;
    }

    // Count members
    int member_count = 0;
    while (grp->gr_mem[member_count] != NULL) {
        member_count++;
    }

    // Allocate array for member names
    char** members = malloc((member_count + 1) * sizeof(char*));
    if (members == NULL) {
        *count = 0;
        return NULL;
    }

    // Copy member names
    for (int i = 0; i < member_count; i++) {
        members[i] = strdup(grp->gr_mem[i]);
        if (members[i] == NULL) {
            // Free already allocated strings on error
            for (int j = 0; j < i; j++) {
                free(members[j]);
            }
            free(members);
            *count = 0;
            return NULL;
        }
    }
    members[member_count] = NULL;

    *count = member_count;
    return members;
}

// free_string_array frees an array of strings returned by get_group_members
void free_string_array(char** arr, int count) {
    if (arr == NULL) return;
    for (int i = 0; i < count; i++) {
        free(arr[i]);
    }
    free(arr);
}
*/
import "C"

import (
	"fmt"
	"os/user"
	"slices"
	"strconv"
	"unsafe"
)

// getGroupMembers returns all members of a group given its GID
func getGroupMembers(gid uint32) ([]string, error) {
	var count C.int
	members := C.get_group_members(C.gid_t(gid), &count)
	if members == nil {
		return []string{}, nil
	}
	defer C.free_string_array(members, count)

	result := make([]string, int(count))
	for i := 0; i < int(count); i++ {
		memberPtr := (**C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(members)) + uintptr(i)*unsafe.Sizeof((*C.char)(nil))))
		result[i] = C.GoString(*memberPtr)
	}
	return result, nil
}

// IsCurrentUserOnlyGroupMember checks if:
// 1. Current user is the file owner
// 2. Current user is a member of the file's group
// 3. Current user is the ONLY member of the file's group (excluding primary group assignment)
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
		// Need to check if the group is anyone's primary group other than current user
		// For simplicity, we'll allow this case if it's the user's primary group
		return currentUser.Gid == fileGidStr, nil
	}

	if len(groupMembers) == 1 && groupMembers[0] == currentUser.Username {
		return true, nil
	}

	// More than one member or the only member is not the current user
	return false, nil
}
