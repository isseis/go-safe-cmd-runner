//go:build cgo

package groupmembership

import (
	"errors"
	"math"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CGO-specific tests can be added here if needed in the future
// Currently, all tests are shared through membership_common_test.go

// TestValidateGroupMemberCount tests the boundary check helper for C group member counts.
func TestValidateGroupMemberCount(t *testing.T) {
	tests := []struct {
		name      string
		count     int
		wantErr   bool
		targetErr error
	}{
		{
			name:    "zero is valid",
			count:   0,
			wantErr: false,
		},
		{
			name:    "positive valid count",
			count:   1,
			wantErr: false,
		},
		{
			name:    "maximum allowed count",
			count:   maxGroupMembers,
			wantErr: false,
		},
		{
			name:      "negative count returns error",
			count:     -1,
			wantErr:   true,
			targetErr: ErrInvalidGroupMemberCount,
		},
		{
			name:      "large negative count returns error",
			count:     -9999,
			wantErr:   true,
			targetErr: ErrInvalidGroupMemberCount,
		},
		{
			name:      "count exceeds maximum returns error",
			count:     maxGroupMembers + 1,
			wantErr:   true,
			targetErr: ErrGroupMemberCountExceedsMax,
		},
		{
			name:      "count far exceeds maximum returns error",
			count:     100000,
			wantErr:   true,
			targetErr: ErrGroupMemberCountExceedsMax,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGroupMemberCount(tt.count)
			if tt.wantErr {
				require.Error(t, err)
				if tt.targetErr != nil {
					assert.ErrorIs(t, err, tt.targetErr)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetExplicitGroupMembers_ERANGERetrySucceeds(t *testing.T) {
	orig := grBufferInitialSize
	t.Cleanup(func() { grBufferInitialSize = orig })
	grBufferInitialSize = 16

	currentGID := getCurrentUserGID(t)

	members, found, err := getExplicitGroupMembers(currentGID)
	require.NoError(t, err)
	assert.True(t, found)

	grBufferInitialSize = 0
	baseline, foundBaseline, errBaseline := getExplicitGroupMembers(currentGID)
	require.NoError(t, errBaseline)
	assert.True(t, foundBaseline)
	assert.ElementsMatch(t, baseline, members)
}

func TestGetExplicitGroupMembers_ERANGERetryExceedsLimit(t *testing.T) {
	origInit := grBufferInitialSize
	origMax := grBufferMaxSize
	t.Cleanup(func() {
		grBufferInitialSize = origInit
		grBufferMaxSize = origMax
	})
	grBufferInitialSize = 16
	grBufferMaxSize = 16

	currentGID := getCurrentUserGID(t)
	_, found, err := getExplicitGroupMembers(currentGID)
	require.Error(t, err)
	assert.False(t, found)
	assert.True(t, errors.Is(err, ErrGroupMemberEnumeration))
}

func TestGetExplicitGroupMembers_AllocationFailure(t *testing.T) {
	origInit := grBufferInitialSize
	origMax := grBufferMaxSize
	t.Cleanup(func() {
		grBufferInitialSize = origInit
		grBufferMaxSize = origMax
	})
	// buf_max must be raised alongside buf_initial: get_group_members checks
	// bufsize > buf_max before calling malloc, so leaving buf_max at its
	// default would trip that ERANGE check first and never reach malloc,
	// making this indistinguishable from TestGetExplicitGroupMembers_ERANGERetryExceedsLimit.
	// math.MaxInt/2 stays a valid int constant on both 32-bit and 64-bit
	// platforms, unlike 1<<62 which overflows int on 32-bit builds.
	grBufferInitialSize = math.MaxInt / 2
	grBufferMaxSize = math.MaxInt / 2

	currentGID := getCurrentUserGID(t)
	_, found, err := getExplicitGroupMembers(currentGID)
	require.Error(t, err)
	assert.False(t, found)
	assert.True(t, errors.Is(err, ErrGroupMemberEnumeration))
	// The C errno is now wrapped via %w, so callers can branch on it directly.
	assert.True(t, errors.Is(err, syscall.ENOMEM))
}

func TestGetExplicitGroupMembers_InvalidGID(t *testing.T) {
	const invalidGID = 99999
	members, found, err := getExplicitGroupMembers(invalidGID)
	assert.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, members)
}
