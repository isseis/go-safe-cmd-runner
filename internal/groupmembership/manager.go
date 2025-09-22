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
)

const (
	// DefaultCacheTimeout is the default timeout duration for cache entries
	DefaultCacheTimeout = 30 * time.Second
	// CleanupInterval defines how often to perform full cache cleanup (every N cache misses)
	CleanupInterval = 10
)

// ErrUIDOutOfBounds is returned when a UID value is out of bounds for uint32
var ErrUIDOutOfBounds = errors.New("UID is out of bounds for uint32")

// ErrFileWorldWritable is returned when a file has world-writable permissions
var ErrFileWorldWritable = errors.New("file is world-writable")

// ErrFileNotWritable is returned when a file has no writable permissions for the user
var ErrFileNotWritable = errors.New("file has no writable permissions for user")

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
func (gm *GroupMembership) IsUserInGroup(username, groupName string) (bool, error) {
	// Look up the group by name to get its GID
	group, err := user.LookupGroup(groupName)
	if err != nil {
		return false, fmt.Errorf("failed to lookup group %s: %w", groupName, err)
	}

	gid, err := strconv.ParseUint(group.Gid, 10, 32)
	if err != nil {
		return false, fmt.Errorf("invalid GID %s for group %s: %w", group.Gid, groupName, err)
	}

	// Get all members of the group
	members, err := gm.GetGroupMembers(uint32(gid))
	if err != nil {
		return false, fmt.Errorf("failed to get members of group %s: %w", groupName, err)
	}

	// Check if the user is in the members list
	return slices.Contains(members, username), nil
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

	// Get all members of the group
	members, err := gm.GetGroupMembers(groupGID)
	if err != nil {
		return false, fmt.Errorf("failed to get group members for GID %d: %w", groupGID, err)
	}

	// Check if there's exactly one member and it's the specified user
	return len(members) == 1 && members[0] == user.Username, nil
}

// IsCurrentUserOnlyGroupMember checks if:
// 1. Current user is the file owner
// 2. Current user is a member of the file's group
// 3. Current user is the ONLY member of the file's group
func (gm *GroupMembership) IsCurrentUserOnlyGroupMember(fileUID, fileGID uint32) (bool, error) {
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
	groupMembers, err := gm.GetGroupMembers(fileGID)
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

// CanUserSafelyWriteFile checks if a user can safely write to a file based on file permissions, ownership and group membership.
//
// This function implements the comprehensive security policy:
// 1. Deny if file has other writable permissions (world writable)
// 2. If file has group writable permissions: allow only if user owns file OR user is the only member of file's group
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
	// Convert userUID to uint32 for comparison
	// #nosec G115 -- safe: `userUID` represents a system user ID (UID), which is
	// non-negative and constrained by the operating system to fit within a 32-bit
	// unsigned value on supported platforms. We only use this value for equality
	// comparison against `fileUID`, so this conversion does not lead to unsafe
	// arithmetic or influence memory sizes.
	userUID32 := uint32(userUID) // #nosec G115

	perm := filePerm.Perm()

	// 1. Always forbid world writable (other writable)
	if perm&0o002 != 0 {
		return false, fmt.Errorf("%w with permissions %o", ErrFileWorldWritable, perm)
	}

	// 2. Check group writable permissions
	if perm&0o020 != 0 {
		// Group writable: allow only if user owns file OR user is the only member of the group
		if userUID32 == fileUID {
			return true, nil // User owns the file, safe to write
		}
		// Check if user is the only member of the file's group
		return gm.isUserOnlyGroupMember(userUID, fileGID)
	}

	// 3. Check owner writable permissions
	if perm&0o200 != 0 {
		// Owner writable: allow only if user owns the file
		return userUID32 == fileUID, nil
	}

	// File is not writable by user, group, or others
	return false, fmt.Errorf("%w UID %d", ErrFileNotWritable, userUID)
}

// CanCurrentUserSafelyWriteFile is a convenience wrapper for the current user.
//
// This function checks if the current user can safely write to a file, using the same
// security policy as CanUserSafelyWriteFile. It's a simpler alternative to IsCurrentUserOnlyGroupMember
// with more intuitive semantics.
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
	currentUser, err := user.Current()
	if err != nil {
		return false, fmt.Errorf("failed to get current user: %w", err)
	}

	currentUID, err := strconv.Atoi(currentUser.Uid)
	if err != nil {
		return false, fmt.Errorf("failed to parse current user UID: %w", err)
	}

	if currentUID < 0 || currentUID > math.MaxUint32 {
		return false, fmt.Errorf("%w: %d", ErrUIDOutOfBounds, currentUID)
	}

	return gm.CanUserSafelyWriteFile(currentUID, fileUID, fileGID, filePerm)
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
