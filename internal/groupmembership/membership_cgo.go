//go:build cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations using system calls.
package groupmembership

/*
#include <sys/types.h>
#include <grp.h>
#include <stdlib.h>
#include <string.h>
#include <errno.h>
#include <unistd.h>

// get_group_members returns the members of a group given its GID.
//
// Three-value contract:
//   non-NULL           : success. *count_out holds the number of members (>= 0).
//   NULL && *err_out==0: group not found (not an error).
//   NULL && *err_out!=0: enumeration error. *err_out holds an errno-like value.
//
// buf_initial / buf_max control ERANGE retry buffer sizing.
// When buf_initial is 0 the function queries sysconf(_SC_GETGR_R_SIZE_MAX).
//
// Caller is responsible for freeing the returned array and its strings.
char** get_group_members(gid_t gid, int* count_out, int* err_out,
                         size_t buf_initial, size_t buf_max) {
    struct group grp;
    struct group *result;
    char *buf;
    size_t bufsize;
    int s;

    *err_out = 0;
    *count_out = 0;

    bufsize = buf_initial > 0 ? buf_initial : (size_t)sysconf(_SC_GETGR_R_SIZE_MAX);
    if (bufsize == (size_t)-1 || bufsize == 0) {
        bufsize = 16384;
    }

    for (;;) {
        if (bufsize > buf_max) {
            *err_out = ERANGE;
            return NULL;
        }

        buf = malloc(bufsize);
        if (buf == NULL) {
            *err_out = ENOMEM;
            return NULL;
        }

        s = getgrgid_r(gid, &grp, buf, bufsize, &result);
        if (s == ERANGE) {
            free(buf);
            size_t next = bufsize * 2;
            if (next < bufsize) {
                *err_out = ERANGE;
                return NULL;
            }
            bufsize = next;
            continue;
        }

        if (s != 0) {
            free(buf);
            *err_out = (s == -1) ? errno : s;
            return NULL;
        }

        if (result == NULL) {
            free(buf);
            return NULL;
        }

        break;
    }

    // Count members. gr_mem being NULL or empty is success (count=0), not "not found".
    int member_count = 0;
    if (grp.gr_mem != NULL) {
        while (grp.gr_mem[member_count] != NULL) {
            member_count++;
        }
    }

    // Allocate array for member names
    char** members = malloc((member_count + 1) * sizeof(char*));
    if (members == NULL) {
        free(buf);
        *err_out = ENOMEM;
        return NULL;
    }

    // Copy member names
    for (int i = 0; i < member_count; i++) {
        members[i] = strdup(grp.gr_mem[i]);
        if (members[i] == NULL) {
            for (int j = 0; j < i; j++) {
                free(members[j]);
            }
            free(members);
            free(buf);
            *err_out = ENOMEM;
            return NULL;
        }
    }
    members[member_count] = NULL;

    free(buf);
    *count_out = member_count;
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
	"errors"
	"fmt"
	"unsafe"
)

// maxGroupMembers is the maximum number of group members allowed.
// This limit prevents unsafe memory access from malformed C return values.
const maxGroupMembers = 65536

// ErrInvalidGroupMemberCount is returned when a negative group member count is
// received from C, which indicates a malformed or corrupted return value.
var ErrInvalidGroupMemberCount = errors.New("invalid group member count from C")

// ErrGroupMemberCountExceedsMax is returned when the group member count from C
// exceeds the allowed maximum, preventing unsafe memory access.
var ErrGroupMemberCountExceedsMax = errors.New("group member count exceeds maximum")

// grBufferInitialSize is the initial buffer size for getgrgid_r calls.
// When 0, sysconf(_SC_GETGR_R_SIZE_MAX) is used (falling back to 16384).
// Tests may set this to a small value to deterministically trigger ERANGE retry.
var grBufferInitialSize int

// grBufferMaxSize is the absolute upper limit for getgrgid_r buffer growth.
var grBufferMaxSize = 4 * 1024 * 1024

// validateGroupMemberCount validates the group member count received from C.
// It returns an error if count is negative or exceeds the maximum allowed.
func validateGroupMemberCount(count int) error {
	if count < 0 {
		return fmt.Errorf("%w: %d", ErrInvalidGroupMemberCount, count)
	}
	if count > maxGroupMembers {
		return fmt.Errorf("%w: %d exceeds %d", ErrGroupMemberCountExceedsMax, count, maxGroupMembers)
	}
	return nil
}

// getExplicitGroupMembers returns explicit (gr_mem) members of the group with the given GID.
//
// Three-value return:
//
//	found == true  : members contains the explicit group member names.
//	found == false : group does not exist (not an error).
//	err != nil     : NSS error, ERANGE limit, or allocation failure.
func getExplicitGroupMembers(gid uint32) (members []string, found bool, err error) {
	var count C.int
	var cerr C.int

	bufInitial := C.size_t(grBufferInitialSize)
	bufMax := C.size_t(grBufferMaxSize)

	cMembers := C.get_group_members(C.gid_t(gid), &count, &cerr, bufInitial, bufMax)
	if cerr != 0 {
		return nil, false, fmt.Errorf("%w: gid %d: C errno %d", ErrGroupMemberEnumeration, gid, int(cerr))
	}
	if cMembers == nil {
		return nil, false, nil
	}

	if errv := validateGroupMemberCount(int(count)); errv != nil {
		C.free_string_array(cMembers, 0)
		return nil, false, fmt.Errorf("%w: gid %d: %w", ErrGroupMemberEnumeration, gid, errv)
	}
	defer C.free_string_array(cMembers, count)

	cArray := unsafe.Slice(cMembers, int(count))
	members = make([]string, int(count))
	for i, cStr := range cArray {
		members[i] = C.GoString(cStr)
	}
	return members, true, nil
}

// getGroupMembers returns all members of a group given its GID.
// Phase 1 returns only explicit members; Phase 2 will add primary-GID members.
func getGroupMembers(gid uint32) ([]string, error) {
	members, found, err := getExplicitGroupMembers(gid)
	if err != nil {
		return nil, err
	}
	if !found {
		return []string{}, nil
	}
	return members, nil
}
