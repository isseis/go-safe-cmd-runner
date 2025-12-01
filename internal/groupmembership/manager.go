package groupmembership

import (
	"errors"
	"fmt"
	"math"
	"os"
	"os/user"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

const (
	// DefaultCacheTimeout is the default timeout duration for cache entries
	DefaultCacheTimeout = 30 * time.Second
	// CleanupInterval defines how often to perform full cache cleanup (every N cache misses)
	CleanupInterval = 10
	// AllPermissionBits represents all possible permission and special bits
	AllPermissionBits = 0o7777
	// MaxAllowedReadPerms defines the maximum allowed file permissions for read operations
	MaxAllowedReadPerms = 0o6775 // rwsrwsr-x with setuid and setgid
	// MaxAllowedWritePerms defines the maximum allowed file permissions for write operations
	MaxAllowedWritePerms = 0o664 // rw-rw-r-- with group write allowed for write operations
)

// ErrUIDOutOfBounds is returned when a UID value is out of bounds for uint32
var ErrUIDOutOfBounds = errors.New("UID is out of bounds for uint32")

// ErrFileWorldWritable is returned when a file has world-writable permissions
var ErrFileWorldWritable = errors.New("file is world-writable")

// ErrFileNotWritable is returned when a file has no writable permissions for the user
var ErrFileNotWritable = errors.New("file has no writable permissions for user")

// ErrFileNotOwner is returned when a user does not own the file
var ErrFileNotOwner = errors.New("user does not own the file")

// ErrGroupWritableNonMember is returned when accessing group writable file with non-member user
var ErrGroupWritableNonMember = errors.New("group writable file with non-member user access")

// ErrPermissionsExceedMaximum is returned when file permissions exceed the maximum allowed for the operation
var ErrPermissionsExceedMaximum = errors.New("file permissions exceed maximum allowed for operation")

// ErrSudoUIDOutOfRange is returned when SUDO_UID value is out of range for uint32
var ErrSudoUIDOutOfRange = errors.New("SUDO_UID value out of range")

// FileOperation represents the type of file operation being performed
type FileOperation int

const (
	// FileOpRead indicates a read operation
	FileOpRead FileOperation = iota
	// FileOpWrite indicates a write operation
	FileOpWrite
)

// GroupMembership provides group membership checking functionality with explicit cache management
type GroupMembership struct {
	// cache for group membership data with thread safety
	membershipCache map[uint32]groupMemberCache
	cacheMutex      sync.RWMutex
	// cleanupCounter tracks cache misses to trigger periodic cleanup
	cleanupCounter int
}

// groupMemberCache holds cached group membership data with expiration
type groupMemberCache struct {
	members []string
	expiry  time.Time
}

// New creates a new GroupMembership instance
func New() *GroupMembership {
	return &GroupMembership{
		membershipCache: make(map[uint32]groupMemberCache),
	}
}

// GetGroupMembers returns all members of a group given its GID
// Results are cached for performance with the configured timeout
func (gm *GroupMembership) GetGroupMembers(gid uint32) ([]string, error) {
	// Check cache first
	gm.cacheMutex.RLock()
	if cached, exists := gm.membershipCache[gid]; exists && time.Now().Before(cached.expiry) {
		gm.cacheMutex.RUnlock()
		return cached.members, nil
	}
	gm.cacheMutex.RUnlock()

	// Cache miss or expired - acquire write lock and compute
	gm.cacheMutex.Lock()
	defer gm.cacheMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine might have populated it)
	if cached, exists := gm.membershipCache[gid]; exists && time.Now().Before(cached.expiry) {
		return cached.members, nil
	}

	// Increment cleanup counter and perform periodic cleanup
	gm.cleanupCounter++
	if gm.cleanupCounter >= CleanupInterval {
		gm.clearExpiredCache()
		gm.cleanupCounter = 0
	}

	// Get group members using the appropriate implementation (CGO or non-CGO)
	members, err := getGroupMembers(gid)
	if err != nil {
		return nil, err
	}

	// Cache the result
	gm.membershipCache[gid] = groupMemberCache{
		members: members,
		expiry:  time.Now().Add(DefaultCacheTimeout),
	}

	return members, nil
}

// IsUserInGroup checks if a user is a member of a group
func (gm *GroupMembership) IsUserInGroup(uid, gid uint32) (bool, error) {
	// Look up user by UID to get username and primary group
	userInfo, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return false, fmt.Errorf("failed to lookup user for UID %d: %w", uid, err)
	}

	// Check if this is the user's primary group
	userPrimaryGID, err := strconv.ParseUint(userInfo.Gid, 10, 32)
	if err != nil {
		return false, fmt.Errorf("failed to parse user's primary GID %s: %w", userInfo.Gid, err)
	}
	if uint32(userPrimaryGID) == gid {
		return true, nil
	}

	// Check secondary group memberships
	groupIDs, err := userInfo.GroupIds()
	if err != nil {
		return false, fmt.Errorf("failed to get user group memberships: %w", err)
	}

	targetGIDStr := strconv.FormatUint(uint64(gid), 10)
	if slices.Contains(groupIDs, targetGIDStr) {
		return true, nil
	}

	// Also check explicit group members (for completeness)
	members, err := gm.GetGroupMembers(gid)
	if err != nil {
		return false, fmt.Errorf("failed to get members of group GID %d: %w", gid, err)
	}

	// Check if the user is in the members list
	return slices.Contains(members, userInfo.Username), nil
}

// isUserOnlyGroupMember checks if the specified user is the only member of a group
// This is useful for security validation where group write permissions are acceptable
// only if the group has a single member who is the specified user
func (gm *GroupMembership) isUserOnlyGroupMember(userUID int, groupGID uint32) (bool, error) {
	// Get user information
	user, err := user.LookupId(strconv.Itoa(userUID))
	if err != nil {
		return false, fmt.Errorf("failed to lookup user for UID %d: %w", userUID, err)
	}

	// Check if this is the user's primary group
	userPrimaryGID, err := strconv.ParseUint(user.Gid, 10, 32)
	if err != nil {
		return false, fmt.Errorf("failed to parse user's primary GID %s: %w", user.Gid, err)
	}

	// Get all explicit members of the group
	members, err := gm.GetGroupMembers(groupGID)
	if err != nil {
		return false, fmt.Errorf("failed to get group members for GID %d: %w", groupGID, err)
	}

	if uint32(userPrimaryGID) == groupGID {
		// This is the user's primary group
		// User is the only member if there's exactly one member (the user themselves)
		// or if there are no explicit members (depends on implementation)
		if len(members) == 0 {
			return true, nil // No explicit members, user is the only primary group member
		}
		return len(members) == 1 && members[0] == user.Username, nil
	}
	// This is not the user's primary group
	// Check if there's exactly one explicit member and it's the specified user
	return len(members) == 1 && members[0] == user.Username, nil
}

// CanUserSafelyWriteFile checks if a user can safely write to a file based on file permissions, ownership and group membership.
//
// This function implements the comprehensive security policy:
// 1. Deny if file has other writable permissions (world writable)
// 2. If file has group writable permissions: allow only if user owns file AND user is the only member of file's group
// 3. If file has owner writable permissions: allow only if user owns the file
//
// This prevents potential security issues where files could be modified by unintended users.
//
// Parameters:
//   - userUID: The user ID to check (as int)
//   - fileUID: The file owner's user ID (as uint32)
//   - fileGID: The file's group ID (as uint32)
//   - filePerm: The file permissions (as os.FileMode)
//
// Returns:
//   - bool: true if the user can safely write to the file, false otherwise
//   - error: non-nil if there was an error checking user or group information, or if write is not safe
//
// This is the core security policy for determining write permissions in a multi-user environment.
func (gm *GroupMembership) CanUserSafelyWriteFile(userUID int, fileUID, fileGID uint32, filePerm os.FileMode) (bool, error) {
	// Validate userUID is within bounds for uint32 before conversion.
	// Reject negative UIDs to avoid underflow when converting to uint32.
	if userUID < 0 || userUID > math.MaxUint32 {
		return false, fmt.Errorf("%w: %d", ErrUIDOutOfBounds, userUID)
	}

	// Convert userUID to uint32 for comparison
	// #nosec G115 -- safe: `userUID` represents a system user ID (UID), which is
	// constrained by the operating system to fit within a 32-bit unsigned value on
	// supported platforms. We already validated bounds above.
	userUID32 := uint32(userUID) // #nosec G115

	perm := filePerm.Perm()

	// 1. Always forbid world writable (other writable)
	if perm&0o002 != 0 {
		return false, fmt.Errorf("%w with permissions %o", ErrFileWorldWritable, perm)
	}

	// 2. Check group writable permissions
	if perm&0o020 != 0 {
		// Group writable: allow only if user owns file AND user is the only member of the group
		if userUID32 != fileUID {
			return false, fmt.Errorf("%w with permissions %o", ErrFileNotOwner, perm) // User doesn't own the file, dangerous to write
		}
		// Check if user is the only member of the file's group
		return gm.isUserOnlyGroupMember(userUID, fileGID)
	}

	// 3. Check owner writable permissions
	if perm&0o200 != 0 {
		// Owner writable: allow only if user owns the file
		if userUID32 == fileUID {
			return true, nil
		}
	}

	// File is not writable by user, group, or others
	return false, fmt.Errorf("%w UID %d", ErrFileNotWritable, userUID)
}

// CanCurrentUserSafelyWriteFile is a convenience wrapper for the current user.
//
// This function checks if the current user can safely write to a file, using the same
// security policy as CanUserSafelyWriteFile.
//
// Parameters:
//   - fileUID: The file owner's user ID (as uint32)
//   - fileGID: The file's group ID (as uint32)
//   - filePerm: The file permissions (as os.FileMode)
//
// Returns:
//   - bool: true if the current user can safely write to the file, false otherwise
//   - error: non-nil if there was an error getting current user info or checking permissions
//
// Example usage:
//
//	canWrite, err := gm.CanCurrentUserSafelyWriteFile(stat.Uid, stat.Gid, fileInfo.Mode())
//	if err != nil {
//	    return fmt.Errorf("failed to check write safety: %w", err)
//	}
//	if !canWrite {
//	    return fmt.Errorf("current user cannot safely write to file")
//	}
func (gm *GroupMembership) CanCurrentUserSafelyWriteFile(fileUID, fileGID uint32, filePerm os.FileMode) (bool, error) {
	// For write operations, use the actual EUID (not SUDO_UID) to verify
	// that the running process has permission to write to the file.
	// This is important for hash files that should only be writable by root.
	currentUID, err := getProcessEUID()
	if err != nil {
		return false, err
	}

	return gm.CanUserSafelyWriteFile(currentUID, fileUID, fileGID, filePerm)
}

// CanCurrentUserSafelyReadFile checks if the current user can safely read from a file
// with more relaxed permissions compared to write operations.
//
// This function implements the read-specific security policy:
//  1. Deny if file has world writable permissions (security risk)
//  2. If file has group writable permissions: deny only if current user is NOT in the file's group
//  3. Allow reading for files with standard read permissions (up to 0o6755)
//
// This is more permissive than write operations, as reading generally poses lower security risks.
//
// Parameters:
//   - fileGID: The file's group ID (as uint32)
//   - filePerm: The file permissions (as os.FileMode)
//
// Returns:
//   - bool: true if the current user can safely read from the file, false otherwise
//   - error: non-nil if there was an error checking user or group information
func (gm *GroupMembership) CanCurrentUserSafelyReadFile(fileGID uint32, filePerm os.FileMode) (bool, error) {
	effectiveUID, err := getPermissionCheckUID()
	if err != nil {
		return false, err
	}

	// For reads: deny only if effective user is NOT in the group
	// Convert userUID to uint32 for IsUserInGroup call
	// #nosec G115 -- safe: `effectiveUID` represents a system user ID (UID), which is
	// non-negative and constrained by the operating system to fit within a 32-bit
	// unsigned value on supported platforms. It was already validated in getPermissionCheckUID().
	effectiveUID32 := uint32(effectiveUID) // #nosec G115

	perm := filePerm.Perm()

	// 1. Always forbid world writable (other writable) - same as write policy
	if perm&0o002 != 0 {
		return false, fmt.Errorf("%w with permissions %o", ErrFileWorldWritable, perm)
	}

	// 2. Check group writable permissions - more relaxed than write policy
	if perm&0o020 != 0 {

		isUserInGroup, err := gm.IsUserInGroup(effectiveUID32, fileGID)
		if err != nil {
			return false, fmt.Errorf("failed to check group membership: %w", err)
		}

		if !isUserInGroup {
			return false, fmt.Errorf("%w: current user not in file's group", ErrGroupWritableNonMember)
		}
		// If user is in group, allow read access
	}

	// 3. Allow reading with broader permissions
	// This is more permissive than write operations

	permMask := filePerm & AllPermissionBits
	disallowedBits := permMask &^ MaxAllowedReadPerms // Find bits that are set but not allowed
	if disallowedBits != 0 {
		return false, fmt.Errorf("%w: file permissions %o have disallowed bits %o, maximum allowed %o",
			ErrPermissionsExceedMaximum, permMask, disallowedBits, MaxAllowedReadPerms)
	}

	return true, nil
}

// ValidateRequestedPermissions validates the requested permissions before file creation/modification
// This performs permission validation to ensure requested permissions don't exceed security limits
// for the specified operation type.
//
// Parameters:
//   - perm: The requested file permissions
//   - operation: The intended file operation (read/write)
//
// Returns:
//   - error: Validation error if permissions exceed maximum allowed for the operation
func (gm *GroupMembership) ValidateRequestedPermissions(perm os.FileMode, operation FileOperation) error {
	// Select maximum allowed permissions based on operation type
	var maxAllowedPerms os.FileMode
	switch operation {
	case FileOpRead:
		maxAllowedPerms = MaxAllowedReadPerms
	case FileOpWrite:
		maxAllowedPerms = MaxAllowedWritePerms
	default:
		return fmt.Errorf("%w: unknown file operation", common.ErrInvalidFileOperation)
	}

	// Check if requested permissions exceed the maximum allowed
	// Use full mode to include setuid/setgid/sticky bits, not just Perm()
	fullMode := perm & AllPermissionBits // Include all permission and special bits
	disallowedBits := fullMode &^ maxAllowedPerms
	if disallowedBits != 0 {
		return fmt.Errorf("%w: requested permissions %o exceed maximum allowed %o for %v operation",
			ErrPermissionsExceedMaximum, fullMode, maxAllowedPerms, operation)
	}

	return nil
}

// ClearCache manually clears all cached group membership data
func (gm *GroupMembership) ClearCache() {
	gm.cacheMutex.Lock()
	defer gm.cacheMutex.Unlock()
	gm.membershipCache = make(map[uint32]groupMemberCache)
	gm.cleanupCounter = 0
}

// CacheStats represents cache statistics in a type-safe manner
type CacheStats struct {
	TotalEntries   int           `json:"total_entries"`
	ExpiredEntries int           `json:"expired_entries"`
	CacheTimeout   time.Duration `json:"cache_timeout"`
}

// GetCacheStats returns cache statistics for monitoring and debugging
func (gm *GroupMembership) GetCacheStats() CacheStats {
	gm.cacheMutex.RLock()
	defer gm.cacheMutex.RUnlock()

	totalEntries := len(gm.membershipCache)
	expiredEntries := 0
	now := time.Now()

	for _, entry := range gm.membershipCache {
		if now.After(entry.expiry) {
			expiredEntries++
		}
	}

	return CacheStats{
		TotalEntries:   totalEntries,
		ExpiredEntries: expiredEntries,
		CacheTimeout:   DefaultCacheTimeout,
	}
}

// clearExpiredCache removes expired cache entries (must be called with write lock held)
func (gm *GroupMembership) clearExpiredCache() {
	now := time.Now()
	for gid, entry := range gm.membershipCache {
		if now.After(entry.expiry) {
			delete(gm.membershipCache, gid)
		}
	}
}

// getPermissionCheckUID returns the effective user ID for permission checks.
// When running under sudo (EUID is 0 and SUDO_UID is set), it returns the original user's UID.
// Otherwise, it returns the current user's UID.
//
// This allows sudo to perform permission checks as if the original user were accessing the file,
// which is important for validating that the user has legitimate access to the files.
//
// This function is primarily used for read operations where we want to verify the original
// user has access to the file being read.
//
// Returns:
//   - int: The effective UID to use for permission checks
//   - error: Error if unable to determine the UID
func getPermissionCheckUID() (int, error) {
	currentUID, err := getProcessEUID()
	if err != nil {
		return 0, err
	}

	// Check if running under sudo: EUID must be 0 (root) and SUDO_UID must be set
	if currentUID == 0 {
		if sudoUID := os.Getenv("SUDO_UID"); sudoUID != "" {
			return parseSudoUID(sudoUID)
		}
	}

	return currentUID, nil
}

// parseSudoUID parses and validates a SUDO_UID string value.
// This is separated from getPermissionCheckUID to allow independent testing.
//
// Parameters:
//   - sudoUID: The string value of SUDO_UID environment variable
//
// Returns:
//   - int: The parsed UID value
//   - error: Error if the value is invalid (not a number, negative, or exceeds uint32)
func parseSudoUID(sudoUID string) (int, error) {
	parsedUID, err := strconv.Atoi(sudoUID)
	if err != nil {
		return 0, fmt.Errorf("failed to parse SUDO_UID %s: %w", sudoUID, err)
	}
	if parsedUID < 0 || parsedUID > math.MaxUint32 {
		return 0, fmt.Errorf("SUDO_UID value out of range %s: %w", sudoUID, ErrSudoUIDOutOfRange)
	}
	return parsedUID, nil
}

// getProcessEUID returns the current user's EUID without considering SUDO_UID.
// This returns the actual EUID of the running process.
//
// This function is primarily used for write operations where we want to verify
// the actual running process has the necessary permissions to write files.
//
// Returns:
//   - int: The current user's EUID
//   - error: Error if unable to determine the EUID
func getProcessEUID() (int, error) {
	currentUser, err := user.Current()
	if err != nil {
		return 0, fmt.Errorf("failed to get current user: %w", err)
	}

	currentUID, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		return 0, fmt.Errorf("failed to parse current user UID: %w", err)
	}

	if currentUID < 0 || currentUID > math.MaxUint32 {
		return 0, fmt.Errorf("%w: %d", ErrUIDOutOfBounds, currentUID)
	}

	return currentUID, nil
}
