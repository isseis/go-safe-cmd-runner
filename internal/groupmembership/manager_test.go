package groupmembership

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestGroupMembership tests the new GroupMembership struct
func TestGroupMembership(t *testing.T) {
	t.Run("New creates instance", func(t *testing.T) {
		gm := New()
		assert.NotNil(t, gm)
	})

	t.Run("GetGroupMembers with valid GID", func(t *testing.T) {
		gm := New()

		// Test with a valid GID (0 = root group exists on most systems)
		members, err := gm.GetGroupMembers(0)
		assert.NoError(t, err)
		assert.NotNil(t, members)

		// Test caching - second call should be from cache
		members2, err := gm.GetGroupMembers(0)
		assert.NoError(t, err)
		assert.Equal(t, members, members2)

		// Verify cache stats
		stats := gm.GetCacheStats()
		assert.Equal(t, 1, stats.TotalEntries)
	})

	t.Run("GetGroupMembers with invalid GID", func(t *testing.T) {
		gm := New()

		// Test with an invalid GID
		members, err := gm.GetGroupMembers(99999)
		assert.NoError(t, err)
		assert.Empty(t, members) // Should return empty slice for non-existent group
	})

	t.Run("IsUserInGroup with valid group", func(t *testing.T) {
		gm := New()

		// Test with root group (should exist on most systems)
		isMember, err := gm.IsUserInGroup("root", "root")
		if err != nil {
			t.Skipf("Skipping test: %v", err)
		}
		assert.NoError(t, err)
		// We can't assert the specific result since it depends on system configuration
		assert.IsType(t, false, isMember)
	})

	t.Run("IsUserInGroup with invalid group", func(t *testing.T) {
		gm := New()

		// Test with non-existent group
		_, err := gm.IsUserInGroup("testuser", "nonexistent_group_12345")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to lookup group")
	})

	t.Run("cache behavior", func(t *testing.T) {
		gm := New()

		// Add entry to cache
		_, err := gm.GetGroupMembers(0)
		assert.NoError(t, err)

		// Verify cache has entry
		stats := gm.GetCacheStats()
		assert.Equal(t, 1, stats.TotalEntries)
		assert.Equal(t, DefaultCacheTimeout, stats.CacheTimeout)

		// Add another entry
		_, err = gm.GetGroupMembers(1)
		assert.NoError(t, err)

		// Verify cache has both entries
		stats = gm.GetCacheStats()
		assert.Equal(t, 2, stats.TotalEntries)
	})

	t.Run("ClearCache", func(t *testing.T) {
		gm := New()

		// Add entries to cache
		_, err := gm.GetGroupMembers(0)
		assert.NoError(t, err)
		_, err = gm.GetGroupMembers(1)
		assert.NoError(t, err)

		// Verify cache has entries
		stats := gm.GetCacheStats()
		assert.Equal(t, 2, stats.TotalEntries)

		// Clear cache
		gm.ClearCache()

		// Verify cache is empty
		stats = gm.GetCacheStats()
		assert.Equal(t, 0, stats.TotalEntries)
	})

	t.Run("GetCacheStats format", func(t *testing.T) {
		gm := New()

		stats := gm.GetCacheStats()

		// Type-safe access to cache statistics
		assert.IsType(t, 0, stats.TotalEntries)
		assert.IsType(t, 0, stats.ExpiredEntries)
		assert.IsType(t, time.Duration(0), stats.CacheTimeout)

		// Verify initial values
		assert.Equal(t, 0, stats.TotalEntries)
		assert.Equal(t, 0, stats.ExpiredEntries)
		assert.Equal(t, DefaultCacheTimeout, stats.CacheTimeout)
	})
}

// TestGroupMembershipIsCurrentUserOnlyGroupMember tests the IsCurrentUserOnlyGroupMember method
func TestGroupMembershipIsCurrentUserOnlyGroupMember(t *testing.T) {
	gm := New()

	// Create a temporary file to get its owner information
	uid, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	// Test with the file we just created (should be owned by current user)
	isOnlyMember, err := gm.IsCurrentUserOnlyGroupMember(uid, gid)
	assert.NoError(t, err, "IsCurrentUserOnlyGroupMember should not return an error")

	// The result depends on the system configuration
	// We can't assert the specific value, but we can check it's a valid boolean
	t.Logf("Current user is only group member: %v", isOnlyMember)
}

// TestCanUserSafelyWriteFile tests the CanUserSafelyWriteFile method
func TestCanUserSafelyWriteFile(t *testing.T) {
	gm := New()

	// Create a temporary file to get its owner information
	uid, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	t.Run("owner can safely write", func(t *testing.T) {
		// Test with the file owner (current user) and owner-writable permissions
		canWrite, err := gm.CanUserSafelyWriteFile(int(uid), uid, gid, 0o644)
		assert.NoError(t, err, "CanUserSafelyWriteFile should not return an error for file owner")
		assert.True(t, canWrite, "File owner should be able to safely write")
	})

	t.Run("nonexistent user with group writable permissions", func(t *testing.T) {
		// Test with a user ID that doesn't exist trying to access group-writable file
		nonexistentUID := int(uid) + 1000                                           // Use a UID that's unlikely to exist
		canWrite, err := gm.CanUserSafelyWriteFile(nonexistentUID, uid, gid, 0o664) // group writable

		// Should return an error for nonexistent user when trying to check group membership
		assert.Error(t, err, "CanUserSafelyWriteFile should return an error for nonexistent user")
		assert.False(t, canWrite, "Should return false for nonexistent user")
		assert.Contains(t, err.Error(), "failed to lookup user", "Error should mention user lookup failure")
	})

	t.Run("root user test", func(t *testing.T) {
		// Test with root user (UID 0) - this should work if running with appropriate permissions
		canWrite, err := gm.CanUserSafelyWriteFile(0, uid, gid, 0o644)

		if err != nil {
			// If we can't test with root, skip this test
			t.Skipf("Cannot test with root user: %v", err)
		} else {
			// Root typically can write to any file they own or if they're the only group member
			t.Logf("Root user (UID 0) can safely write: %v", canWrite)
		}
	})

	// Add comprehensive permission tests
	t.Run("world writable file denied", func(t *testing.T) {
		canWrite, err := gm.CanUserSafelyWriteFile(int(uid), uid, gid, 0o666) // world writable
		assert.Error(t, err, "World writable files should be denied")
		assert.False(t, canWrite, "Should return false for world writable files")
		assert.True(t, errors.Is(err, ErrFileWorldWritable), "Error should be ErrFileWorldWritable")
	})

	t.Run("group writable file - owner allowed", func(t *testing.T) {
		canWrite, err := gm.CanUserSafelyWriteFile(int(uid), uid, gid, 0o664) // group writable
		assert.NoError(t, err, "Group writable file should not error for owner")
		assert.True(t, canWrite, "File owner should be allowed for group writable files")
	})

	t.Run("non-writable file denied", func(t *testing.T) {
		canWrite, err := gm.CanUserSafelyWriteFile(int(uid), uid, gid, 0o444) // read-only
		assert.Error(t, err, "Non-writable files should be denied")
		assert.False(t, canWrite, "Should return false for non-writable files")
		assert.True(t, errors.Is(err, ErrFileNotWritable), "Error should be ErrFileNotWritable")
	})

	t.Run("owner writable only - non-owner denied", func(t *testing.T) {
		otherUID := int(uid) + 1
		canWrite, err := gm.CanUserSafelyWriteFile(otherUID, uid, gid, 0o644) // owner writable only
		assert.NoError(t, err, "Should not error for valid UID check")
		assert.False(t, canWrite, "Non-owner should be denied for owner-only writable files")
	})
}

// TestCanCurrentUserSafelyWriteFile tests the CanCurrentUserSafelyWriteFile method
func TestCanCurrentUserSafelyWriteFile(t *testing.T) {
	gm := New()

	// Create a temporary file to get its owner information
	uid, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	t.Run("current user can safely write to own file", func(t *testing.T) {
		// Test with the file we just created (should be owned by current user)
		canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, 0o644)
		assert.NoError(t, err, "CanCurrentUserSafelyWriteFile should not return an error")
		assert.True(t, canWrite, "Current user should be able to safely write to own file")
	})

	t.Run("consistency with old function", func(t *testing.T) {
		// Test that the new function gives the same result as the old one
		oldResult, err1 := gm.IsCurrentUserOnlyGroupMember(uid, gid)
		newResult, err2 := gm.CanCurrentUserSafelyWriteFile(uid, gid, 0o644)

		assert.NoError(t, err1, "IsCurrentUserOnlyGroupMember should not return an error")
		assert.NoError(t, err2, "CanCurrentUserSafelyWriteFile should not return an error")

		// For files owned by current user, both functions should return true
		// (The new function is more permissive as it allows owners regardless of group membership)
		if oldResult {
			assert.True(t, newResult, "If old function returns true, new function should also return true")
		}
		t.Logf("Old function result: %v, New function result: %v", oldResult, newResult)
	})
}
