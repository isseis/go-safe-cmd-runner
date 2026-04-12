//go:build cgo

package groupmembership

import (
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
