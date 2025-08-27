package groupmembership

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestGroupMembership tests the new GroupMembership struct
func TestGroupMembership(t *testing.T) {
	t.Run("New creates instance with default timeout", func(t *testing.T) {
		gm := New()
		assert.NotNil(t, gm)
		assert.Equal(t, 30*time.Second, gm.cacheTimeout)
	})

	t.Run("NewWithTimeout creates instance with custom timeout", func(t *testing.T) {
		timeout := 5 * time.Second
		gm := NewWithTimeout(timeout)
		assert.NotNil(t, gm)
		assert.Equal(t, timeout, gm.cacheTimeout)
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
		assert.Equal(t, 1, stats["total_entries"])
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

	t.Run("cache expiration", func(t *testing.T) {
		gm := NewWithTimeout(100 * time.Millisecond)

		// Add entry to cache
		_, err := gm.GetGroupMembers(0)
		assert.NoError(t, err)

		// Verify cache has entry
		stats := gm.GetCacheStats()
		assert.Equal(t, 1, stats["total_entries"])

		// Wait for expiration
		time.Sleep(150 * time.Millisecond)

		// Access again (should trigger cleanup)
		_, err = gm.GetGroupMembers(1) // Different GID to trigger cleanup
		assert.NoError(t, err)

		// Verify expired entries were cleaned up
		stats = gm.GetCacheStats()
		assert.Equal(t, 1, stats["total_entries"]) // Only the new entry
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
		assert.Equal(t, 2, stats["total_entries"])

		// Clear cache
		gm.ClearCache()

		// Verify cache is empty
		stats = gm.GetCacheStats()
		assert.Equal(t, 0, stats["total_entries"])
	})

	t.Run("SetCacheTimeout", func(t *testing.T) {
		gm := New()

		newTimeout := 5 * time.Second
		gm.SetCacheTimeout(newTimeout)
		assert.Equal(t, newTimeout, gm.cacheTimeout)
	})

	t.Run("GetCacheStats format", func(t *testing.T) {
		gm := New()

		stats := gm.GetCacheStats()
		assert.Contains(t, stats, "total_entries")
		assert.Contains(t, stats, "expired_entries")
		assert.Contains(t, stats, "cache_timeout")

		assert.IsType(t, 0, stats["total_entries"])
		assert.IsType(t, 0, stats["expired_entries"])
		assert.IsType(t, "", stats["cache_timeout"])
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

// TestPackageLevelFunctions tests backward compatibility functions
func TestPackageLevelFunctions(t *testing.T) {
	t.Run("IsUserInGroup package function", func(t *testing.T) {
		// Test with root group (should exist on most systems)
		isMember, err := IsUserInGroup("root", "root")
		if err != nil {
			t.Skipf("Skipping test: %v", err)
		}
		assert.NoError(t, err)
		assert.IsType(t, false, isMember)
	})

	t.Run("ClearCache package function", func(t *testing.T) {
		// Test that the package-level function works
		ClearCache()

		stats := GetCacheStats()
		assert.Equal(t, 0, stats["total_entries"])
	})

	t.Run("SetCacheTimeout package function", func(t *testing.T) {
		originalTimeout := 30 * time.Second
		defer SetCacheTimeout(originalTimeout)

		newTimeout := 5 * time.Second
		SetCacheTimeout(newTimeout)

		// We can't directly access the timeout, but we can verify it through stats
		stats := GetCacheStats()
		assert.Contains(t, stats["cache_timeout"].(string), "5s")
	})
}
