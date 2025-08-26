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
