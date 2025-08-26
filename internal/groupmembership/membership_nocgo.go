//go:build !cgo

// Package groupmembership provides utilities for checking group membership
// and related user/group operations.
// This file provides fallback implementations when CGO is disabled by parsing /etc/group.
package groupmembership

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// groupMemberCache holds cached group membership data with expiration
type groupMemberCache struct {
	members []string
	expiry  time.Time
}

// cache for group membership data with thread safety
var (
	membershipCache = make(map[uint32]groupMemberCache)
	cacheMutex      sync.RWMutex
	cacheTimeout    = 30 * time.Second // Cache timeout duration
)

// clearExpiredCache removes expired cache entries
func clearExpiredCache() {
	now := time.Now()
	for gid, entry := range membershipCache {
		if now.After(entry.expiry) {
			delete(membershipCache, gid)
		}
	}
}

// getGroupMembers returns all members of a group given its GID by parsing /etc/group
// and /etc/passwd to find users with this GID as their primary group
// Results are cached for performance with a configurable timeout
func getGroupMembers(gid uint32) ([]string, error) {
	// Check cache first
	cacheMutex.RLock()
	if cached, exists := membershipCache[gid]; exists && time.Now().Before(cached.expiry) {
		cacheMutex.RUnlock()
		return cached.members, nil
	}
	cacheMutex.RUnlock()

	// Cache miss or expired - acquire write lock and compute
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// Double-check after acquiring write lock (another goroutine might have populated it)
	if cached, exists := membershipCache[gid]; exists && time.Now().Before(cached.expiry) {
		return cached.members, nil
	}

	// Clear expired entries periodically (simple cleanup strategy)
	clearExpiredCache()

	groupEntry, err := findGroupByGID(gid)
	if err != nil {
		return nil, err
	}
	if groupEntry == nil {
		// Cache empty result too
		membershipCache[gid] = groupMemberCache{
			members: []string{},
			expiry:  time.Now().Add(cacheTimeout),
		}
		return []string{}, nil // Group not found
	}

	// Start with explicit members from /etc/group
	memberSet := make(map[string]bool)
	if groupEntry.members != "" {
		members := strings.Split(groupEntry.members, ",")
		for _, member := range members {
			member = strings.TrimSpace(member)
			if member != "" {
				memberSet[member] = true
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
		memberSet[user] = true
	}

	// Convert map to slice
	result := make([]string, 0, len(memberSet))
	for member := range memberSet {
		result = append(result, member)
	}

	// Cache the result
	membershipCache[gid] = groupMemberCache{
		members: result,
		expiry:  time.Now().Add(cacheTimeout),
	}

	return result, nil
}

// SetCacheTimeout allows configuring the cache timeout duration
// This is useful for testing and performance tuning
func SetCacheTimeout(timeout time.Duration) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	cacheTimeout = timeout
}

// ClearCache manually clears all cached group membership data
// This is useful when system group/user configuration changes are detected
func ClearCache() {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()
	membershipCache = make(map[uint32]groupMemberCache)
}

// GetCacheStats returns cache statistics for monitoring and debugging
func GetCacheStats() map[string]interface{} {
	cacheMutex.RLock()
	defer cacheMutex.RUnlock()

	totalEntries := len(membershipCache)
	expiredEntries := 0
	now := time.Now()

	for _, entry := range membershipCache {
		if now.After(entry.expiry) {
			expiredEntries++
		}
	}

	return map[string]interface{}{
		"total_entries":   totalEntries,
		"expired_entries": expiredEntries,
		"cache_timeout":   cacheTimeout.String(),
	}
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

// findUsersWithPrimaryGID finds all users that have the specified GID as their primary group
// by parsing /etc/passwd
func findUsersWithPrimaryGID(gid uint32) ([]string, error) {
	file, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, fmt.Errorf("failed to open /etc/passwd: %w", err)
	}
	defer file.Close()

	var users []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue // Skip empty lines and comments
		}

		user, userGID, err := parsePasswdLine(line)
		if err != nil {
			continue // Skip malformed lines
		}

		if userGID == gid {
			users = append(users, user)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading /etc/passwd: %w", err)
	}

	return users, nil
}

// parsePasswdLine parses a single line from /etc/passwd and returns username and primary GID
// Format: username:password:uid:gid:gecos:home:shell
func parsePasswdLine(line string) (string, uint32, error) {
	fields := strings.Split(line, ":")
	if len(fields) < 4 {
		return "", 0, fmt.Errorf("invalid passwd line format")
	}

	gid, err := strconv.ParseUint(fields[3], 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("invalid GID: %w", err)
	}

	return fields[0], uint32(gid), nil
}
