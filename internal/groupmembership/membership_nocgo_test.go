//go:build !cgo

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

func TestGetGroupMembers_NoCGO(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err, "Failed to get current user")

	currentGID, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	require.NoError(t, err, "Failed to parse current user GID")

	// Test getting members of current user's primary group
	members, err := GetGroupMembers(uint32(currentGID))
	assert.NoError(t, err, "GetGroupMembers should not return an error")
	assert.NotNil(t, members, "GetGroupMembers should return a slice")

	// The result might be empty if the group has no explicit members
	// (only primary group assignment), which is valid
	t.Logf("Group %d has %d explicit members: %v", currentGID, len(members), members)
}

func TestGetGroupMembers_InvalidGID_NoCGO(t *testing.T) {
	// Use a GID that's very unlikely to exist
	const invalidGID = 99999

	members, err := GetGroupMembers(invalidGID)
	assert.NoError(t, err, "GetGroupMembers should not return an error for non-existent group")
	assert.Empty(t, members, "GetGroupMembers should return empty slice for non-existent group")
}

func TestIsCurrentUserOnlyGroupMember_NoCGO(t *testing.T) {
	// Create a temporary file to get realistic UID/GID
	tempFile, err := os.CreateTemp("", "grouptest")
	require.NoError(t, err, "Failed to create temp file")
	defer func() {
		tempFile.Close()
		os.Remove(tempFile.Name())
	}()

	// Get file stat info
	fileInfo, err := tempFile.Stat()
	require.NoError(t, err, "Failed to stat temp file")

	stat, ok := fileInfo.Sys().(*syscall.Stat_t)
	require.True(t, ok, "Failed to get syscall.Stat_t")

	// Test with the file we just created (should be owned by current user)
	isOnlyMember, err := IsCurrentUserOnlyGroupMember(stat.Uid, stat.Gid)
	assert.NoError(t, err, "IsCurrentUserOnlyGroupMember should not return an error")

	// The result depends on the system configuration
	// We can't assert the specific value, but we can check it's a valid boolean
	t.Logf("Current user is only group member: %v", isOnlyMember)
}

func TestIsCurrentUserOnlyGroupMember_NotFileOwner_NoCGO(t *testing.T) {
	currentUser, err := user.Current()
	require.NoError(t, err, "Failed to get current user")

	currentGID, err := strconv.ParseUint(currentUser.Gid, 10, 32)
	require.NoError(t, err, "Failed to parse current user GID")

	// Use a different UID (root = 0, assuming current user is not root)
	const rootUID = 0

	isOnlyMember, err := IsCurrentUserOnlyGroupMember(rootUID, uint32(currentGID))
	assert.NoError(t, err, "IsCurrentUserOnlyGroupMember should not return an error")
	assert.False(t, isOnlyMember, "Should return false when current user is not the file owner")
}

func TestParseGroupLine(t *testing.T) {
	tests := []struct {
		name        string
		line        string
		expected    *groupEntry
		shouldError bool
	}{
		{
			name: "normal group with members",
			line: "adm:x:4:syslog,issei",
			expected: &groupEntry{
				name:    "adm",
				gid:     4,
				members: "syslog,issei",
			},
			shouldError: false,
		},
		{
			name: "group with no members",
			line: "root:x:0:",
			expected: &groupEntry{
				name:    "root",
				gid:     0,
				members: "",
			},
			shouldError: false,
		},
		{
			name:        "invalid line format",
			line:        "invalid:line",
			expected:    nil,
			shouldError: true,
		},
		{
			name:        "invalid GID",
			line:        "group:x:notanumber:members",
			expected:    nil,
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseGroupLine(tt.line)
			if tt.shouldError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}
