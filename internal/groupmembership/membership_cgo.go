//go:build cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations using system calls.
package groupmembership

/*
#include <sys/types.h>
#include <grp.h>
#include <pwd.h>
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

// get_users_with_primary_gid returns all users whose primary GID matches gid.
//
// Two-value contract:
//   non-NULL           : success. *count_out holds the number of users (>= 0).
//   NULL && *err_out!=0: enumeration error. *err_out holds an errno-like value.
//
// This function is the sole caller of setpwent/getpwent/endpwent within the process.
// Caller is responsible for freeing the returned array and its strings.
char** get_users_with_primary_gid(gid_t gid, int* count_out, int* err_out) {
    struct passwd *pw;
    char **users = NULL;
    int capacity = 0;
    int count = 0;

    *err_out = 0;
    *count_out = 0;

    setpwent();

    for (;;) {
        errno = 0;
        pw = getpwent();
        if (pw == NULL) {
            if (errno != 0) {
                *err_out = errno;
            }
            break;
        }

        if (pw->pw_gid != gid) {
            continue;
        }

        if (count >= capacity) {
            int new_capacity = capacity == 0 ? 16 : capacity * 2;
            char **new_users = realloc(users, (new_capacity + 1) * sizeof(char*));
            if (new_users == NULL) {
                for (int i = 0; i < count; i++) {
                    free(users[i]);
                }
                free(users);
                endpwent();
                *err_out = ENOMEM;
                return NULL;
            }
            users = new_users;
            capacity = new_capacity;
        }

        users[count] = strdup(pw->pw_name);
        if (users[count] == NULL) {
            for (int i = 0; i < count; i++) {
                free(users[i]);
            }
            free(users);
            endpwent();
            *err_out = ENOMEM;
            return NULL;
        }
        count++;
    }

    endpwent();

    if (*err_out != 0) {
        if (users != NULL) {
            for (int i = 0; i < count; i++) {
                free(users[i]);
            }
            free(users);
        }
        return NULL;
    }

    if (users == NULL) {
        users = malloc(sizeof(char*));
        if (users == NULL) {
            *err_out = ENOMEM;
            return NULL;
        }
    }
    users[count] = NULL;
    *count_out = count;
    return users;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
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

	// Clamp negative values before the signed->unsigned conversion; otherwise a
	// negative int would wrap to a huge size_t and defeat the buf_max cap.
	bufInitial := C.size_t(max(grBufferInitialSize, 0))
	bufMax := C.size_t(max(grBufferMaxSize, 0))

	cMembers := C.get_group_members(C.gid_t(gid), &count, &cerr, bufInitial, bufMax)
	if cerr != 0 {
		return nil, false, fmt.Errorf("%w: gid %d: C errno %d: %w", ErrGroupMemberEnumeration, gid, int(cerr), syscall.Errno(cerr))
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
//
// Result is the union of explicit members (gr_mem) and users whose primary
// GID matches the requested GID. This matches the non-CGO implementation
// semantics.
//
// pwentMutex serialises all setpwent/getpwent/endpwent calls within this
// package. Lock ordering: GroupMembership.cacheMutex -> pwentMutex.
// Reverse acquisition is forbidden.
var pwentMutex sync.Mutex

func getGroupMembers(gid uint32) ([]string, error) {
	members, found, err := getExplicitGroupMembers(gid)
	if err != nil {
		return nil, err
	}
	if !found {
		return []string{}, nil
	}

	pwentMutex.Lock()
	primary, err := getUsersWithPrimaryGID(gid)
	pwentMutex.Unlock()
	if err != nil {
		return nil, err
	}

	return mergeGroupMembers(members, primary)
}

// getUsersWithPrimaryGID returns users whose primary GID matches the given GID.
// The caller must hold pwentMutex.
func getUsersWithPrimaryGID(gid uint32) ([]string, error) {
	var count C.int
	var cerr C.int

	cUsers := C.get_users_with_primary_gid(C.gid_t(gid), &count, &cerr)
	if cerr != 0 {
		return nil, fmt.Errorf("%w: gid %d: %w", ErrGroupMemberEnumeration, gid, syscall.Errno(cerr))
	}

	if errv := validateGroupMemberCount(int(count)); errv != nil {
		C.free_string_array(cUsers, 0)
		return nil, fmt.Errorf("%w: gid %d: %w", ErrGroupMemberEnumeration, gid, errv)
	}
	defer C.free_string_array(cUsers, count)

	cArray := unsafe.Slice(cUsers, int(count))
	users := make([]string, int(count))
	for i, cStr := range cArray {
		users[i] = C.GoString(cStr)
	}
	return users, nil
}

// mergeGroupMembers returns the union of explicit and primary member slices
// with duplicate removal. Order is not guaranteed.
func mergeGroupMembers(explicit, primary []string) ([]string, error) {
	set := make(map[string]struct{})
	for _, m := range explicit {
		set[m] = struct{}{}
	}
	for _, m := range primary {
		set[m] = struct{}{}
	}
	if errv := validateGroupMemberCount(len(set)); errv != nil {
		return nil, fmt.Errorf("%w: merged member count: %w", ErrGroupMemberEnumeration, errv)
	}
	result := make([]string, 0, len(set))
	for m := range set {
		result = append(result, m)
	}
	return result, nil
}
