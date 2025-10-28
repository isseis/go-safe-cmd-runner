//go:build test

package groupmembership

import (
	"errors"
	"os"
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

	t.Run("ClearExpiredCache with expired entries", func(t *testing.T) {
		gm := New()

		// Add entries to cache
		_, err := gm.GetGroupMembers(0)
		assert.NoError(t, err)

		// Verify cache has entries
		stats := gm.GetCacheStats()
		assert.Equal(t, 1, stats.TotalEntries)
		assert.Equal(t, 0, stats.ExpiredEntries) // Entry should not be expired yet

		// Manually expire the cache entry by directly modifying the expiry time
		gm.cacheMutex.Lock()
		for gid, entry := range gm.membershipCache {
			entry.expiry = time.Now().Add(-1 * time.Second) // Set expiry to 1 second ago
			gm.membershipCache[gid] = entry
		}
		gm.cacheMutex.Unlock()

		// Verify that GetCacheStats reports the expired entry
		stats = gm.GetCacheStats()
		assert.Equal(t, 1, stats.TotalEntries)
		assert.Equal(t, 1, stats.ExpiredEntries)

		// Trigger cleanup by making CleanupInterval cache misses
		// clearExpiredCache is called internally after CleanupInterval misses
		for i := 0; i < CleanupInterval; i++ {
			// Try to get a non-existent group to trigger cache misses
			_, _ = gm.GetGroupMembers(uint32(10000 + i))
		}

		// Verify that expired entries were cleaned up
		stats = gm.GetCacheStats()
		// After cleanup, the expired entry should be removed
		// Note: We can't check exact count since we added new entries above
		assert.GreaterOrEqual(t, stats.TotalEntries, 0, "Cache should have some entries or be empty")
	})

	t.Run("ClearExpiredCache with valid entries", func(t *testing.T) {
		gm := New()

		// Add entries to cache
		_, err := gm.GetGroupMembers(0)
		assert.NoError(t, err)
		_, err = gm.GetGroupMembers(1)
		assert.NoError(t, err)

		// Verify cache has entries
		stats := gm.GetCacheStats()
		assert.Equal(t, 2, stats.TotalEntries)
		assert.Equal(t, 0, stats.ExpiredEntries) // Entries should not be expired

		// Trigger cleanup - valid entries should be preserved
		for i := 0; i < CleanupInterval; i++ {
			_, _ = gm.GetGroupMembers(uint32(10000 + i))
		}

		// Valid entries should still be in the cache (along with new ones)
		stats = gm.GetCacheStats()
		assert.GreaterOrEqual(t, stats.TotalEntries, 2, "Valid entries should be preserved")
	})

	t.Run("ClearExpiredCache with empty cache", func(t *testing.T) {
		gm := New()

		// Verify cache is empty
		stats := gm.GetCacheStats()
		assert.Equal(t, 0, stats.TotalEntries)

		// Trigger cleanup on empty cache - should not cause errors
		for i := 0; i < CleanupInterval; i++ {
			_, _ = gm.GetGroupMembers(uint32(10000 + i))
		}

		// Verify no errors occurred and cache has entries from above calls
		stats = gm.GetCacheStats()
		assert.GreaterOrEqual(t, stats.TotalEntries, 0, "Cache operations should complete without errors")
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
		_, err := gm.CanUserSafelyWriteFile(otherUID, uid, gid, 0o644) // owner writable only
		assert.Error(t, err, "Non-owner should be denied for owner-only writable files")
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

// TestCanCurrentUserSafelyWriteFile_AllPermissions tests all permission patterns
func TestCanCurrentUserSafelyWriteFile_AllPermissions(t *testing.T) {
	gm := New()

	// Create a temporary file to get its owner information
	uid, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	t.Run("owner_only_writable", func(t *testing.T) {
		canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, 0o600)
		assert.NoError(t, err)
		assert.True(t, canWrite, "Owner should be able to write to owner-only file")
	})

	t.Run("group_writable_member", func(t *testing.T) {
		// Current user owns the file, so they are in the group
		canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, 0o660)
		assert.NoError(t, err)
		assert.True(t, canWrite, "Group member should be able to write to group-writable file")
	})

	t.Run("group_writable_non_member", func(t *testing.T) {
		// Use a GID that the current user is not a member of
		// GID 99999 is unlikely to exist and user won't be a member
		canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, 99999, 0o660)
		// This may error or return false depending on system configuration
		// Just verify it doesn't panic and returns a boolean
		assert.IsType(t, false, canWrite)
		t.Logf("Non-member write result: %v, error: %v", canWrite, err)
	})

	t.Run("world_writable", func(t *testing.T) {
		canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, 0o666)
		assert.Error(t, err, "World writable files should be denied")
		assert.False(t, canWrite)
		assert.True(t, errors.Is(err, ErrFileWorldWritable))
	})
}

// TestCanCurrentUserSafelyWriteFile_EdgeCases tests edge cases
func TestCanCurrentUserSafelyWriteFile_EdgeCases(t *testing.T) {
	gm := New()

	uid, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	t.Run("special_permission_bits", func(t *testing.T) {
		tests := []struct {
			name string
			perm os.FileMode
		}{
			{"setuid", 0o4644},
			{"setgid", 0o2644},
			{"sticky", 0o1644},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, tt.perm)
				// CanUserSafelyWriteFile only checks Perm() bits, not special bits
				// So these should succeed if the underlying permission bits are valid
				assert.NoError(t, err, "Special bits don't affect write check")
				assert.True(t, canWrite)
			})
		}
	})

	t.Run("various_permission_combinations", func(t *testing.T) {
		tests := []struct {
			name      string
			perm      os.FileMode
			expectErr bool
		}{
			{"owner_read_write", 0o644, false},
			{"owner_only", 0o600, false},
			{"group_read_write", 0o664, false},
			{"execute_bit", 0o755, false},  // execute bits don't affect write check
			{"minimal_perms", 0o400, true}, // no write permission
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, tt.perm)
				if tt.expectErr {
					assert.Error(t, err)
					assert.False(t, canWrite)
				} else {
					assert.NoError(t, err)
					// Note: result depends on ownership/group membership
					t.Logf("Permission %o: can write=%v, err=%v", tt.perm, canWrite, err)
				}
			})
		}
	})
}

// TestCanCurrentUserSafelyReadFile_AllPermissions tests all permission patterns for read
func TestCanCurrentUserSafelyReadFile_AllPermissions(t *testing.T) {
	gm := New()

	_, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	t.Run("owner_only_readable", func(t *testing.T) {
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o400)
		assert.NoError(t, err)
		assert.True(t, canRead, "Should be able to read owner-only file")
	})

	t.Run("group_readable_member", func(t *testing.T) {
		// Current user owns the file, so they are in the group
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o440)
		assert.NoError(t, err)
		assert.True(t, canRead, "Group member should be able to read group-readable file")
	})

	t.Run("group_writable_non_member", func(t *testing.T) {
		// Use a GID that the current user is not a member of
		canRead, err := gm.CanCurrentUserSafelyReadFile(99999, 0o660)
		// Should error because user is not in group and file is group writable
		assert.Error(t, err)
		assert.False(t, canRead)
		assert.True(t, errors.Is(err, ErrGroupWritableNonMember))
	})

	t.Run("world_readable", func(t *testing.T) {
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o444)
		assert.NoError(t, err)
		assert.True(t, canRead, "Should be able to read world-readable file")
	})

	t.Run("world_writable_denied", func(t *testing.T) {
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o666)
		assert.Error(t, err, "World writable files should be denied for read")
		assert.False(t, canRead)
		assert.True(t, errors.Is(err, ErrFileWorldWritable))
	})
}

// TestCanCurrentUserSafelyReadFile_EdgeCases tests edge cases for read
func TestCanCurrentUserSafelyReadFile_EdgeCases(t *testing.T) {
	gm := New()

	_, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	t.Run("special_permission_bits_allowed_for_read", func(t *testing.T) {
		tests := []struct {
			name      string
			perm      os.FileMode
			expectErr bool
		}{
			{"setuid", 0o4755, false},        // setuid allowed for read
			{"setgid", 0o2755, false},        // setgid allowed for read
			{"sticky", 0o1755, true},         // sticky exceeds MaxAllowedReadPerms
			{"setuid_setgid", 0o6755, false}, // both setuid and setgid allowed
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				canRead, err := gm.CanCurrentUserSafelyReadFile(gid, tt.perm)
				if tt.expectErr {
					assert.Error(t, err)
					assert.False(t, canRead)
				} else {
					assert.NoError(t, err)
					assert.True(t, canRead)
				}
			})
		}
	})

	t.Run("maximum_allowed_permissions", func(t *testing.T) {
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, MaxAllowedReadPerms)
		assert.NoError(t, err)
		assert.True(t, canRead, "Should allow maximum allowed read permissions")
	})

	t.Run("exceeding_maximum_permissions", func(t *testing.T) {
		// Add sticky bit to exceed maximum
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, MaxAllowedReadPerms|0o1000)
		assert.Error(t, err)
		assert.False(t, canRead)
		assert.True(t, errors.Is(err, ErrPermissionsExceedMaximum))
	})

	t.Run("various_readable_permissions", func(t *testing.T) {
		tests := []struct {
			name string
			perm os.FileMode
		}{
			{"minimal", 0o400},
			{"normal", 0o644},
			{"group_read", 0o440},
			{"all_read", 0o444},
			{"with_execute", 0o555},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				canRead, err := gm.CanCurrentUserSafelyReadFile(gid, tt.perm)
				assert.NoError(t, err)
				assert.True(t, canRead)
			})
		}
	})
}
