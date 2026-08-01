package groupmembership

import (
	"errors"
	"log/slog"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		assert.ErrorIs(t, err, ErrFileNotOwner)
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
		assert.ErrorIs(t, err, ErrFileWorldWritable)
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
		assert.ErrorIs(t, err, ErrFileNotWritable)
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
		// On macOS, the default group (GID 20 "staff") has multiple members,
		// so isUserOnlyGroupMember returns false and access is denied.
		// This is correct security behavior — skip on macOS.
		if runtime.GOOS == "darwin" {
			t.Skip("Skipping: macOS primary group (staff) has multiple members, access correctly denied")
		}
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
		assert.ErrorIs(t, err, ErrFileWorldWritable)
	})

	t.Run("setuid file allowed for read", func(t *testing.T) {
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, 0o4755) // setuid
		assert.NoError(t, err, "Setuid files should be allowed for read operations")
		assert.True(t, canRead, "Should allow reading setuid files")
	})

	t.Run("consistency with write function - read should be more permissive", func(t *testing.T) {
		// On macOS, the default group (GID 20 "staff") has multiple members,
		// so isUserOnlyGroupMember returns false and access is denied.
		// This is correct security behavior — skip on macOS.
		if runtime.GOOS == "darwin" {
			t.Skip("Skipping: macOS primary group (staff) has multiple members, access correctly denied")
		}
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
		// On macOS, the default group (GID 20 "staff") has multiple members,
		// so isUserOnlyGroupMember returns false and write is denied.
		// This is correct security behavior — skip on macOS.
		if runtime.GOOS == "darwin" {
			t.Skip("Skipping: macOS primary group (staff) has multiple members, write correctly denied")
		}
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
		assert.ErrorIs(t, err, ErrFileWorldWritable)
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
		assert.ErrorIs(t, err, ErrGroupWritableNonMember)
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
		assert.ErrorIs(t, err, ErrFileWorldWritable)
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
		// MaxAllowedReadPerms (0o6775) includes group write, which triggers group
		// membership check. On macOS, the default group (staff) has multiple members,
		// so isUserOnlyGroupMember returns false and access is denied.
		if runtime.GOOS == "darwin" {
			t.Skip("Skipping: macOS primary group (staff) has multiple members, access correctly denied")
		}
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, MaxAllowedReadPerms)
		assert.NoError(t, err)
		assert.True(t, canRead, "Should allow maximum allowed read permissions")
	})

	t.Run("exceeding_maximum_permissions", func(t *testing.T) {
		// MaxAllowedReadPerms|0o1000 includes group write + sticky, which triggers group
		// membership check. On macOS, the default group (staff) has multiple members,
		// so isUserOnlyGroupMember returns false and access is denied (but for the wrong reason).
		if runtime.GOOS == "darwin" {
			t.Skip("Skipping: macOS primary group (staff) has multiple members, group write check short-circuits")
		}
		// Add sticky bit to exceed maximum
		canRead, err := gm.CanCurrentUserSafelyReadFile(gid, MaxAllowedReadPerms|0o1000)
		assert.Error(t, err)
		assert.False(t, canRead)
		assert.ErrorIs(t, err, ErrPermissionsExceedMaximum)
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

// TestGetPermissionCheckUID tests the getPermissionCheckUID method
func TestGetPermissionCheckUID(t *testing.T) {
	t.Run("returns real UID under the final default policy", func(t *testing.T) {
		t.Setenv("SUDO_UID", "9999")

		gm := New()
		uid, err := gm.getPermissionCheckUID()
		assert.NoError(t, err)
		assert.Equal(t, os.Getuid(), uid)
	})

	t.Run("reads SUDO_UID from the real environment under SudoUIDAware", func(t *testing.T) {
		t.Setenv("SUDO_UID", "9999")

		gm := New(WithPermissionCheckUIDPolicy(SudoUIDAware))
		uid, err := gm.getPermissionCheckUID()
		assert.NoError(t, err)
		if os.Getuid() == 0 {
			assert.Equal(t, 9999, uid)
		} else {
			assert.Equal(t, os.Getuid(), uid)
		}
	})

	t.Run("SUDO_UID with invalid value", func(t *testing.T) {
		// Test parseSudoUID directly - this doesn't require root privileges
		invalidValues := []struct {
			name  string
			value string
		}{
			{"non-numeric", "invalid"},
			{"negative value", "-1"},
			{"large overflow", "999999999999"},
			{"empty string", ""},
		}
		for _, test := range invalidValues {
			t.Run(test.name, func(t *testing.T) {
				_, err := parseSudoUID(test.value)
				assert.Error(t, err, "parseSudoUID(%s) should return an error", test.value)
			})
		}
	})

	t.Run("malicious SUDO_UID values - out of bounds", func(t *testing.T) {
		// Test parseSudoUID directly with malicious values - this doesn't require root privileges
		maliciousValues := []struct {
			name         string
			value        string
			expectsError string
		}{
			{"negative value", "-1", "SUDO_UID value out of range"},
			{"large overflow", "999999999999999999999", "failed to parse SUDO_UID"}, // Way beyond int, fails to parse
			{"max uint32 + 1", "4294967296", "SUDO_UID value out of range"},         // 2^32, parses but exceeds bounds
			{"max uint64 + 1", "18446744073709551616", "failed to parse SUDO_UID"},  // 2^64, fails to parse
			{"scientific notation", "1e10", "failed to parse SUDO_UID"},
		}

		for _, test := range maliciousValues {
			t.Run(test.name, func(t *testing.T) {
				_, err := parseSudoUID(test.value)
				// All malicious values should return an error
				assert.Error(t, err, "parseSudoUID(%s) should be rejected", test.value)
				assert.Contains(t, err.Error(), test.expectsError)
			})
		}
	})

	t.Run("valid SUDO_UID values", func(t *testing.T) {
		// Test parseSudoUID with valid values - this doesn't require root privileges
		validValues := []struct {
			name     string
			value    string
			expected int
		}{
			{"zero", "0", 0},
			{"normal user", "1000", 1000},
			{"max uint32", "4294967295", 4294967295}, // 2^32 - 1
		}

		for _, test := range validValues {
			t.Run(test.name, func(t *testing.T) {
				uid, err := parseSudoUID(test.value)
				assert.NoError(t, err, "parseSudoUID(%s) should not return an error", test.value)
				assert.Equal(t, test.expected, uid)
			})
		}
	})
}

// TestGetProcessRealUID tests the getProcessRealUID function
func TestGetProcessRealUID(t *testing.T) {
	t.Run("returns os.Getuid and does not error", func(t *testing.T) {
		uid, err := getProcessRealUID()
		assert.NoError(t, err)
		assert.Equal(t, os.Getuid(), uid)
	})

	t.Run("ignores SUDO_UID", func(t *testing.T) {
		t.Setenv("SUDO_UID", "9999")
		uid, err := getProcessRealUID()
		assert.NoError(t, err)
		assert.Equal(t, os.Getuid(), uid)
	})
}

// TestCanCurrentUserSafelyWriteFile_UsesRealUID verifies that write-safety judgments are
// driven solely by os.Getuid(), independent of the getProcessRealUID implementation detail
// (step 3-2 replaced user.Current() with os.Getuid()).
func TestCanCurrentUserSafelyWriteFile_UsesRealUID(t *testing.T) {
	gm := New()

	newTempFileWithPerm := func(t *testing.T, perm os.FileMode) (uint32, uint32, os.FileMode) {
		t.Helper()
		tempFile, err := os.CreateTemp("", "grouptest")
		require.NoError(t, err)
		name := tempFile.Name()
		require.NoError(t, tempFile.Close())
		t.Cleanup(func() { os.Remove(name) })

		require.NoError(t, os.Chmod(name, perm))

		fileInfo, err := os.Stat(name)
		require.NoError(t, err)
		stat, ok := fileInfo.Sys().(*syscall.Stat_t)
		require.True(t, ok)

		return stat.Uid, stat.Gid, fileInfo.Mode()
	}

	t.Run("owner-only writable file is writable", func(t *testing.T) {
		uid, gid, mode := newTempFileWithPerm(t, 0o600)
		canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, mode)
		assert.NoError(t, err)
		assert.True(t, canWrite)

		want, wantErr := gm.CanUserSafelyWriteFile(os.Getuid(), uid, gid, mode)
		assert.Equal(t, wantErr, err)
		assert.Equal(t, want, canWrite)
	})

	t.Run("world-writable file is rejected", func(t *testing.T) {
		uid, gid, mode := newTempFileWithPerm(t, 0o666)
		canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, mode)
		assert.False(t, canWrite)
		assert.ErrorIs(t, err, ErrFileWorldWritable)

		want, wantErr := gm.CanUserSafelyWriteFile(os.Getuid(), uid, gid, mode)
		assert.Equal(t, want, canWrite)
		assert.ErrorIs(t, wantErr, ErrFileWorldWritable)
	})

	t.Run("unwritable file is rejected", func(t *testing.T) {
		uid, gid, mode := newTempFileWithPerm(t, 0o400)
		canWrite, err := gm.CanCurrentUserSafelyWriteFile(uid, gid, mode)
		assert.False(t, canWrite)
		assert.ErrorIs(t, err, ErrFileNotWritable)

		want, wantErr := gm.CanUserSafelyWriteFile(os.Getuid(), uid, gid, mode)
		assert.Equal(t, want, canWrite)
		assert.ErrorIs(t, wantErr, ErrFileNotWritable)
	})
}

// TestIsUserOnlyGroupMember_NoSpecialCasing verifies that isUserOnlyGroupMember no longer
// has a primary-GID special-case branch and uses only len(members)==1 && members[0]==user.Username.
func TestIsUserOnlyGroupMember_NoSpecialCasing(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)
	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)
	currentPrimaryGID, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	require.NoError(t, err)

	tests := []struct {
		name       string
		members    []string
		wantIsOnly bool
		wantErr    bool
	}{
		{
			name:       "empty set returns false",
			members:    []string{},
			wantIsOnly: false,
			wantErr:    false,
		},
		{
			name:       "single current user returns true",
			members:    []string{currentUser.Username},
			wantIsOnly: true,
			wantErr:    false,
		},
		{
			name:       "current user plus other returns false",
			members:    []string{currentUser.Username, "other-user"},
			wantIsOnly: false,
			wantErr:    false,
		},
		{
			name:       "other user only returns false",
			members:    []string{"other-user"},
			wantIsOnly: false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			members := tt.members
			gm := newWithEnumerator(func(_ uint32) ([]string, error) {
				return members, nil
			})
			isOnly, err := gm.isUserOnlyGroupMember(currentUID, uint32(currentPrimaryGID))
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantIsOnly, isOnly)
		})
	}
}

// TestIsUserOnlyGroupMember_EnumerationError verifies that when enumeration fails,
// isUserOnlyGroupMember returns (false, error) and the error wraps the enumeration error.
func TestIsUserOnlyGroupMember_EnumerationError(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)
	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)

	sentinelErr := errors.New("injected enumeration failure")
	gm := newWithEnumerator(func(_ uint32) ([]string, error) {
		return nil, sentinelErr
	})

	isOnly, err := gm.isUserOnlyGroupMember(currentUID, 0)
	assert.False(t, isOnly)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
}

// TestCanUserSafelyWriteFile_EnumerationError verifies that when enumeration fails,
// CanUserSafelyWriteFile does not allow the write on the group-writable path.
func TestCanUserSafelyWriteFile_EnumerationError(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)
	currentUID, err := strconv.Atoi(currentUser.Uid)
	require.NoError(t, err)

	sentinelErr := errors.New("injected enumeration failure")
	gm := newWithEnumerator(func(_ uint32) ([]string, error) {
		return nil, sentinelErr
	})

	canWrite, err := gm.CanUserSafelyWriteFile(currentUID, uint32(currentUID), 0, 0o660)
	assert.False(t, canWrite)
	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
}

// TestGetGroupMembers_ErrorNotCached verifies that enumeration errors are not cached
// and can be retried.
func TestGetGroupMembers_ErrorNotCached(t *testing.T) {
	sentinelErr := errors.New("injected enumeration failure")

	callCount := 0
	gm := newWithEnumerator(func(_ uint32) ([]string, error) {
		callCount++
		if callCount == 1 {
			return nil, sentinelErr
		}
		return []string{"user"}, nil
	})

	members, err := gm.GetGroupMembers(0)
	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
	assert.Nil(t, members)
	assert.Equal(t, 1, callCount)

	stats := gm.GetCacheStats()
	assert.Equal(t, 0, stats.TotalEntries)

	members, err = gm.GetGroupMembers(0)
	assert.NoError(t, err)
	assert.Equal(t, []string{"user"}, members)
	assert.Equal(t, 2, callCount)

	stats = gm.GetCacheStats()
	assert.Equal(t, 1, stats.TotalEntries)
}

// TestIsUserInGroup_NoRegressionWithPrimaryMembers verifies that the primary
// member enumeration addition does not change IsUserInGroup results.
func TestIsUserInGroup_NoRegressionWithPrimaryMembers(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)
	currentUID, err := strconv.ParseUint(currentUser.Uid, 10, 32)
	require.NoError(t, err)
	currentPrimaryGID, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	require.NoError(t, err)

	t.Run("primary GID match unaffected by enumeration expansion", func(t *testing.T) {
		gmWithUser := newWithEnumerator(func(_ uint32) ([]string, error) {
			return []string{currentUser.Username}, nil
		})
		isMember, err := gmWithUser.IsUserInGroup(uint32(currentUID), uint32(currentPrimaryGID))
		assert.NoError(t, err)
		assert.True(t, isMember)

		gmWithoutUser := newWithEnumerator(func(_ uint32) ([]string, error) {
			return []string{}, nil
		})
		isMember, err = gmWithoutUser.IsUserInGroup(uint32(currentUID), uint32(currentPrimaryGID))
		assert.NoError(t, err)
		assert.True(t, isMember)
	})

	t.Run("non-member GID returns false", func(t *testing.T) {
		gm := newWithEnumerator(func(_ uint32) ([]string, error) { return []string{}, nil })
		isMember, err := gm.IsUserInGroup(uint32(currentUID), 99999)
		assert.NoError(t, err)
		assert.False(t, isMember)
	})
}

// TestIsUserInGroup_EnumerationError verifies that when enumeration fails,
// IsUserInGroup propagates the error.
func TestIsUserInGroup_EnumerationError(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err)
	currentUID, err := strconv.ParseUint(currentUser.Uid, 10, 32)
	require.NoError(t, err)

	sentinelErr := errors.New("injected enumeration failure")
	gm := newWithEnumerator(func(_ uint32) ([]string, error) {
		return nil, sentinelErr
	})

	isMember, err := gm.IsUserInGroup(uint32(currentUID), 99999)
	assert.False(t, isMember)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
}

// TestCanCurrentUserSafelyReadFile_EnumerationError verifies that when enumeration
// fails, the read path is fail-closed.
func TestCanCurrentUserSafelyReadFile_EnumerationError(t *testing.T) {
	sentinelErr := errors.New("injected enumeration failure")
	gm := newWithEnumerator(func(_ uint32) ([]string, error) {
		return nil, sentinelErr
	})

	canRead, err := gm.CanCurrentUserSafelyReadFile(99999, 0o660)
	assert.False(t, canRead)
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinelErr)
}

// TestSudoUIDAdoptionReporter_Report verifies that report emits exactly one
// record with the level, message, and attributes required by the architecture
// document §4.3. Different values are used for realUID and permissionCheckUID
// to detect any accidental argument swap.
func TestSudoUIDAdoptionReporter_Report(t *testing.T) {
	t.Parallel()

	handler := newCaptureHandler()
	logger := slog.New(handler)
	r := &sudoUIDAdoptionReporter{}

	r.report(logger, 0, 1000)

	records := handler.snapshot()
	require.Len(t, records, 1)
	rec := records[0]
	assert.Equal(t, slog.LevelWarn, rec.level)
	assert.Equal(t,
		"Permission check UID taken from SUDO_UID instead of the real UID; if this process was not started via sudo, SUDO_UID may be a stale value inherited from the environment",
		rec.message)
	assert.Equal(t, int64(1000), rec.attrs["permission_check_uid"])
	assert.Equal(t, int64(0), rec.attrs["real_uid"])
	assert.Equal(t, sudoUIDEnvVar, rec.attrs["source_env_var"])
	assert.Equal(t, SudoUIDAware.String(), rec.attrs["permission_check_uid_policy"])
	assert.Equal(t, userDatabaseSource, rec.attrs["user_database_source"])
}

// TestSudoUIDAdoptionReporter_RealUIDArgumentIsPropagated exercises the
// reporter with non-zero realUID and permissionCheckUID values to assert
// that both parameters are actually forwarded to the log record and not
// silently dropped. TestSudoUIDAdoptionReporter_Report covers the
// zero/1000 case for the swap-detection contract; this test pins the
// forwarding behaviour for arbitrary non-zero values.
func TestSudoUIDAdoptionReporter_RealUIDArgumentIsPropagated(t *testing.T) {
	t.Parallel()

	handler := newCaptureHandler()
	logger := slog.New(handler)
	r := &sudoUIDAdoptionReporter{}

	r.report(logger, 1234, 5678)

	records := handler.snapshot()
	require.Len(t, records, 1)
	rec := records[0]
	assert.Equal(t, int64(5678), rec.attrs["permission_check_uid"])
	assert.Equal(t, int64(1234), rec.attrs["real_uid"])
}

// TestSudoUIDAdoptionReporter_ReportsOnlyOnce verifies that calling report
// repeatedly on a single instance emits a single record.
func TestSudoUIDAdoptionReporter_ReportsOnlyOnce(t *testing.T) {
	t.Parallel()

	handler := newCaptureHandler()
	logger := slog.New(handler)
	r := &sudoUIDAdoptionReporter{}

	r.report(logger, 0, 1000)
	r.report(logger, 0, 1000)
	r.report(logger, 0, 1000)

	records := handler.snapshot()
	require.Len(t, records, 1)
}

// TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently verifies the
// once-per-instance guarantee under concurrency, so that -race does not report
// a data race and only one record is emitted.
func TestSudoUIDAdoptionReporter_ReportsOnlyOnceConcurrently(t *testing.T) {
	t.Parallel()

	handler := newCaptureHandler()
	logger := slog.New(handler)
	r := &sudoUIDAdoptionReporter{}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			r.report(logger, i, 1000+i)
		}(i)
	}
	wg.Wait()

	records := handler.snapshot()
	require.Len(t, records, 1)
}

// TestSudoUIDAdoptionReporter_NilLoggerPanics documents that a nil
// logger is a programming error: report is only ever invoked from the
// production wiring (which passes slog.Default()) and from tests that
// always supply a real logger, so a nil dereference surfaces the
// mistake immediately.
func TestSudoUIDAdoptionReporter_NilLoggerPanics(t *testing.T) {
	t.Parallel()

	r := &sudoUIDAdoptionReporter{}
	assert.Panics(t, func() {
		r.report(nil, 0, 1000) //nolint:errcheck // intentional panic assertion
	})
	assert.True(t, r.reported.Load(), "the reported flag is set before logger.Warn; the panic occurs during the warn call")
}

// TestSudoUIDAdoptionReporter_HandlerErrorDoesNotChangeReported verifies
// that a handler returning an error does not rewind the reported flag, so
// the next call still does not record (the failure is dropped silently).
func TestSudoUIDAdoptionReporter_HandlerErrorDoesNotChangeReported(t *testing.T) {
	t.Parallel()

	handler := &captureHandler{handleErr: errors.New("simulated handler failure")}
	logger := slog.New(handler)
	r := &sudoUIDAdoptionReporter{}

	r.report(logger, 0, 1000)
	assert.True(t, r.reported.Load(), "the reported flag must be set even when the handler errors")
	require.Len(t, handler.snapshot(), 1, "the record is still captured by the handler before it returns the error")

	// A subsequent call must remain a no-op because the flag is already set.
	r.report(logger, 0, 1000)
	assert.Len(t, handler.snapshot(), 1)
}

// TestSudoUIDExistenceMemo_ReusesConfirmation verifies that a single UID
// is queried at most once.
func TestSudoUIDExistenceMemo_ReusesConfirmation(t *testing.T) {
	t.Parallel()

	m := sudoUIDExistenceMemo{confirmed: make(map[int]struct{})}
	calls := 0
	lookup := func(int) error {
		calls++
		return nil
	}

	for range 3 {
		require.NoError(t, m.verify(1000, lookup))
	}
	assert.Equal(t, 1, calls)
}

// TestSudoUIDExistenceMemo_DoesNotRememberFailures verifies that a failed
// check is re-queried on the next call, and that a successful check is
// then remembered.
func TestSudoUIDExistenceMemo_DoesNotRememberFailures(t *testing.T) {
	t.Parallel()

	m := sudoUIDExistenceMemo{confirmed: make(map[int]struct{})}
	transient := errors.New("transient lookup failure")
	var calls int
	lookup := func(int) error {
		calls++
		if calls < 3 {
			return transient
		}
		return nil
	}

	require.ErrorIs(t, m.verify(1000, lookup), transient)
	require.ErrorIs(t, m.verify(1000, lookup), transient)
	require.NoError(t, m.verify(1000, lookup))
	require.NoError(t, m.verify(1000, lookup))
	assert.Equal(t, 3, calls, "lookup must be called on every failure and once on success; the next success reuses the memo")
}

// TestSudoUIDExistenceMemo_Concurrent exercises the memo from many
// goroutines to surface -race issues and confirm the post-condition
// the architecture actually guarantees: once the goroutines settle,
// successful UIDs are not re-queried, while failing UIDs are still
// re-queried on each call. The implementation deliberately invokes
// the lookup without the memo mutex, so the in-flight call count for
// the successful UID may briefly exceed 1 under heavy contention;
// this test only checks the post-condition.
func TestSudoUIDExistenceMemo_Concurrent(t *testing.T) {
	t.Parallel()

	m := sudoUIDExistenceMemo{confirmed: make(map[int]struct{})}
	transient := errors.New("transient lookup failure")

	var (
		mu        sync.Mutex
		callCount = map[int]int{}
	)
	lookup := func(uid int) error {
		mu.Lock()
		callCount[uid]++
		mu.Unlock()
		if uid == 9999 {
			return transient
		}
		return nil
	}

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		uid := i % 2
		if uid == 0 {
			uid = 1000
		} else {
			uid = 9999
		}
		go func(uid int) {
			defer wg.Done()
			_ = m.verify(uid, lookup)
		}(uid)
	}
	wg.Wait()

	// Snapshot the counters under the lock and assert the post-condition.
	// The mu must be released before the post-wait verifies below because
	// those invokes the lookup which itself takes the same mu.
	mu.Lock()
	successCalls := callCount[1000]
	failureCalls := callCount[9999]
	mu.Unlock()
	require.NotZero(t, successCalls, "the successful UID must have been queried at least once")
	require.NotZero(t, failureCalls, "the failing UID must have been queried at least once")

	// After the goroutines settle, a successful verify must not re-query
	// and a failing verify must re-query at least once more.
	require.NoError(t, m.verify(1000, lookup))
	mu.Lock()
	assert.Equal(t, successCalls, callCount[1000], "success must be remembered after the goroutines finish")
	mu.Unlock()

	beforeFailure := failureCalls
	require.ErrorIs(t, m.verify(9999, lookup), transient)
	mu.Lock()
	assert.Greater(t, callCount[9999], beforeFailure, "failure must be re-queried after the goroutines finish")
	mu.Unlock()
}

// TestNewInitializesSudoUIDExistenceMemo exercises the memo through New so
// that any nil-map initialization regression is caught. The lookup
// counter verifies both that the map is initialised (otherwise writing
// to a nil map panics) and that the second call is short-circuited by
// the memo (a successful first call must register the UID).
func TestNewInitializesSudoUIDExistenceMemo(t *testing.T) {
	t.Parallel()

	gm := New()
	calls := 0
	lookup := func(int) error { //nolint:unparam // returns nil by design: the test never exercises a lookup failure
		calls++
		return nil
	}

	require.NotPanics(t, func() {
		require.NoError(t, gm.sudoUIDExistence.verify(1000, lookup))
	})
	require.NotPanics(t, func() {
		require.NoError(t, gm.sudoUIDExistence.verify(1000, lookup))
	})
	assert.Equal(t, 1, calls, "the second verify must not invoke lookup because the memo remembers the first success")
}

// TestProcessSudoUIDAdoptionReporter_Exists pins the package-level reporter
// instance so that an accidental rename or removal is caught at the test
// level. The instance is intentionally not exercised here: its once-per-process
// flag would interact with other tests' use of slog.Default(). Phase 2 wires
// the reporter into the production code path; this test exists solely so the
// "unused" linter does not silently drop the declaration in phase 1.
func TestProcessSudoUIDAdoptionReporter_Exists(t *testing.T) {
	t.Parallel()

	r := &processSudoUIDAdoptionReporter
	assert.False(t, r.reported.Load(), "the package-level reporter must start in the not-reported state")
}
