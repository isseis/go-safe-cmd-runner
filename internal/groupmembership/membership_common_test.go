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
	enumeration, err := getGroupMembers(currentGID)
	assert.NoError(t, err, "getGroupMembers should not return an error")
	assert.NotNil(t, enumeration.members, "getGroupMembers should return a slice")

	// The result might be empty if the group has no explicit members
	// (only primary group assignment), which is valid
	t.Logf("Group %d has %d explicit members: %v", currentGID, len(enumeration.members), enumeration.members)
}

func TestGetGroupMembers_InvalidGID_Common(t *testing.T) {
	// Use a GID that's very unlikely to exist
	const invalidGID = 99999

	enumeration, err := getGroupMembers(invalidGID)
	assert.NoError(t, err, "getGroupMembers should not return an error for non-existent group")
	assert.Empty(t, enumeration.members, "getGroupMembers should return empty slice for non-existent group")
	// A group that is absent from the user database is an enumeration of
	// zero members, not a reason to doubt the enumeration. What the verdict
	// says beyond that depends on the host this runs on -- a host whose own
	// /etc/group holds an unparsable line is incomplete for every GID -- so
	// only the environment-independent part is asserted here. That an absent
	// group adds no cause of its own is pinned deterministically by
	// TestEnumerateMissingGroupStatesTheEnvironmentVerdict.
	assert.NotEqual(t, completenessUnstated, enumeration.verdict.completeness,
		"an enumeration must always state its completeness")
}
