package groupmembership

import (
	"fmt"
	"os/user"
	"slices"
	"strconv"
	"sync"
	"time"
)

const (
	// DefaultCacheTimeout is the default timeout duration for cache entries
	DefaultCacheTimeout = 30 * time.Second
)

// GroupMembership provides group membership checking functionality with explicit cache management
type GroupMembership struct {
	// cache for group membership data with thread safety
	membershipCache map[uint32]groupMemberCache
	cacheMutex      sync.RWMutex
	cacheTimeout    time.Duration
}

// groupMemberCache holds cached group membership data with expiration
type groupMemberCache struct {
	members []string
	expiry  time.Time
}

// New creates a new GroupMembership instance with default cache timeout
func New() *GroupMembership {
	return NewWithTimeout(DefaultCacheTimeout)
}

// NewWithTimeout creates a new GroupMembership instance with specified cache timeout
func NewWithTimeout(timeout time.Duration) *GroupMembership {
	return &GroupMembership{
		membershipCache: make(map[uint32]groupMemberCache),
		cacheTimeout:    timeout,
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

	// Clear expired entries periodically (simple cleanup strategy)
	gm.clearExpiredCache()

	// Get group members using the appropriate implementation (CGO or non-CGO)
	members, err := getGroupMembers(gid)
	if err != nil {
		return nil, err
	}

	// Cache the result
	gm.membershipCache[gid] = groupMemberCache{
		members: members,
		expiry:  time.Now().Add(gm.cacheTimeout),
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

// ClearCache manually clears all cached group membership data
func (gm *GroupMembership) ClearCache() {
	gm.cacheMutex.Lock()
	defer gm.cacheMutex.Unlock()
	gm.membershipCache = make(map[uint32]groupMemberCache)
}

// SetCacheTimeout updates the cache timeout duration
func (gm *GroupMembership) SetCacheTimeout(timeout time.Duration) {
	gm.cacheMutex.Lock()
	defer gm.cacheMutex.Unlock()
	gm.cacheTimeout = timeout
}

// GetCacheStats returns cache statistics for monitoring and debugging
func (gm *GroupMembership) GetCacheStats() map[string]interface{} {
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

	return map[string]interface{}{
		"total_entries":   totalEntries,
		"expired_entries": expiredEntries,
		"cache_timeout":   gm.cacheTimeout.String(),
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

// Package-level functions for backward compatibility
// These use a default global instance

var defaultManager = New()

// IsUserInGroup checks if a user is a member of a group using the default instance
func IsUserInGroup(username, groupName string) (bool, error) {
	return defaultManager.IsUserInGroup(username, groupName)
}

// ClearCache clears the global cache (for backward compatibility)
func ClearCache() {
	defaultManager.ClearCache()
}

// SetCacheTimeout sets the global cache timeout (for backward compatibility)
func SetCacheTimeout(timeout time.Duration) {
	defaultManager.SetCacheTimeout(timeout)
}

// GetCacheStats returns global cache statistics (for backward compatibility)
func GetCacheStats() map[string]interface{} {
	return defaultManager.GetCacheStats()
}
