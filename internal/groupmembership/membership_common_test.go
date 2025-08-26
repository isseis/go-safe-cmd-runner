package groupmembership

import (
	"os"
	"os/user"
	"strconv"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Common test helper functions

// getCurrentUserGID returns the current user's primary group ID
func getCurrentUserGID(t *testing.T) uint32 {
	t.Helper()
	currentUser, err := user.Current()
	require.NoError(t, err, "Failed to get current user")

	currentGID, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	require.NoError(t, err, "Failed to parse current user GID")

	return uint32(currentGID)
}

// createTempFileWithStat creates a temporary file and returns its UID/GID
func createTempFileWithStat(t *testing.T) (uint32, uint32, func()) {
	t.Helper()
	tempFile, err := os.CreateTemp("", "grouptest")
	require.NoError(t, err, "Failed to create temp file")

	cleanup := func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}

	// Get file stat info
	fileInfo, err := tempFile.Stat()
	require.NoError(t, err, "Failed to stat temp file")

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	require.True(t, ok, "Failed to get syscall.Stat_t")

	return stat.Uid, stat.Gid, cleanup
}

// Common test implementations

func TestGetGroupMembers_Common(t *testing.T) {
	currentGID := getCurrentUserGID(t)

	// Test getting members of current user's primary group
	members, err := getGroupMembers(currentGID)
	assert.NoError(t, err, "getGroupMembers should not return an error")
	assert.NotNil(t, members, "getGroupMembers should return a slice")

	// The result might be empty if the group has no explicit members
	// (only primary group assignment), which is valid
	t.Logf("Group %d has %d explicit members: %v", currentGID, len(members), members)
}

func TestGetGroupMembers_InvalidGID_Common(t *testing.T) {
	// Use a GID that's very unlikely to exist
	const invalidGID = 99999

	members, err := getGroupMembers(invalidGID)
	assert.NoError(t, err, "getGroupMembers should not return an error for non-existent group")
	assert.Empty(t, members, "getGroupMembers should return empty slice for non-existent group")
}

func TestIsCurrentUserOnlyGroupMember_Common(t *testing.T) {
	uid, gid, cleanup := createTempFileWithStat(t)
	defer cleanup()

	// Test with the file we just created (should be owned by current user)
	isOnlyMember, err := IsCurrentUserOnlyGroupMember(uid, gid)
	assert.NoError(t, err, "IsCurrentUserOnlyGroupMember should not return an error")

	// The result depends on the system configuration
	// We can't assert the specific value, but we can check it's a valid boolean
	t.Logf("Current user is only group member: %v", isOnlyMember)
}

func TestIsCurrentUserOnlyGroupMember_NotFileOwner_Common(t *testing.T) {
	currentGID := getCurrentUserGID(t)

	// Use a different UID (root = 0, assuming current user is not root)
	const rootUID = 0

	isOnlyMember, err := IsCurrentUserOnlyGroupMember(rootUID, currentGID)
	assert.NoError(t, err, "IsCurrentUserOnlyGroupMember should not return an error")
	assert.False(t, isOnlyMember, "Should return false when current user is not the file owner")
}
