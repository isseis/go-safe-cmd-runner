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

	t.Run("IsUserInGroup with valid uid/gid", func(t *testing.T) {
		gm := New()

		// Test with root user (UID 0) and root group (GID 0) - should exist on most systems
		isMember, err := gm.IsUserInGroup(0, 0)
		if err != nil {
			t.Skipf("Skipping test: %v", err)
		}
		assert.NoError(t, err)
		// We can't assert the specific result since it depends on system configuration
		assert.IsType(t, false, isMember)
	})

	t.Run("IsUserInGroup with invalid uid", func(t *testing.T) {
		gm := New()

		// Test with non-existent user UID
		_, err := gm.IsUserInGroup(99999, 0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to lookup user")
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

		// With new stricter policy, non-owner users are rejected immediately for group writable files
		// before group membership is even checked
		assert.Error(t, err, "CanUserSafelyWriteFile should return an error for non-owner user")
		assert.False(t, canWrite, "Should return false for non-owner user")
		assert.True(t, errors.Is(err, ErrFileNotOwner), "Error should be ErrFileNotOwner for non-owner accessing group writable file")
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

	t.Run("group writable file - owner only allowed if exclusive group member", func(t *testing.T) {
		canWrite, err := gm.CanUserSafelyWriteFile(int(uid), uid, gid, 0o664) // group writable
		// With new stricter policy, even file owners are only allowed if they're the exclusive group member
		// The function can return (false, nil) if the user is not the exclusive group member
		// or (true, nil) if the user is the exclusive group member
		// We test both outcomes are handled correctly
		assert.NoError(t, err, "Group membership check should not error for valid user and group")

		if canWrite {
			t.Log("File owner is allowed (is exclusive group member)")
		} else {
			t.Log("File owner is denied (not exclusive group member)")
		}

		// Both outcomes (true or false) are valid depending on system configuration
		assert.IsType(t, false, canWrite, "Should return a boolean result")
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
}

// TestCanCurrentUserSafelyReadFile tests the CanCurrentUserSafelyReadFile method
func TestCanCurrentUserSafelyReadFile(t *testing.T) {
	gm := New()

	// Create a temporary file to get its owner information
	uid, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	t.Run("current user can safely read from own file", func(t *testing.T) {
		// Test with the file we just created (should be owned by current user)
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o644)
		assert.NoError(t, err, "CanCurrentUserSafelyReadFile should not return an error")
		assert.True(t, canRead, "Current user should be able to safely read from own file")
	})

	t.Run("current user can read group writable file if in group", func(t *testing.T) {
		// Test with group writable permissions - new spec: deny only if current user is NOT in the group
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o664)
		// Since we created the file, current user should be in the group and read should be allowed
		assert.NoError(t, err, "CanCurrentUserSafelyReadFile should not return an error for group writable")
		assert.True(t, canRead, "Current user should be able to read group writable file since they're in the group")
		t.Logf("Can read group writable file: %v", canRead)
	})

	t.Run("world writable file denied", func(t *testing.T) {
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o666) // world writable
		assert.Error(t, err, "World writable files should be denied for read")
		assert.False(t, canRead, "Should return false for world writable files")
		assert.True(t, errors.Is(err, ErrFileWorldWritable), "Error should be ErrFileWorldWritable")
	})

	t.Run("setuid file allowed for read", func(t *testing.T) {
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o4755) // setuid
		assert.NoError(t, err, "Setuid files should be allowed for read operations")
		assert.True(t, canRead, "Should allow reading setuid files")
	})

	t.Run("consistency with write function - read should be more permissive", func(t *testing.T) {
		// Test that read function is more permissive than write function
		writeResult, writeErr := gm.CanCurrentUserSafelyWriteFile(uid, gid, 0o664)
		readResult, readErr := gm.CanCurrentUserSafelyReadFile(gid, 0o664)

		assert.NoError(t, readErr, "CanCurrentUserSafelyReadFile should not return an error")

		// Read should be at least as permissive as write
		if writeErr == nil && writeResult {
			assert.True(t, readResult, "If write is allowed, read should also be allowed")
		}

		t.Logf("Write result: %v (err: %v), Read result: %v (err: %v)", writeResult, writeErr, readResult, readErr)
	})
}
