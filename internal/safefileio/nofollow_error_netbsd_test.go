//go:build netbsd

package safefileio

import (
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsNoFollowError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		// NetBSD returns EFTYPE when opening a symlink with O_NOFOLLOW
		{
			name: "EFTYPE error",
			err:  &os.PathError{Err: syscall.EFTYPE},
			want: true,
		},
		{
			name: "other error",
			err:  os.ErrNotExist,
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNoFollowError(tt.err)
			assert.Equal(t, tt.want, got, "isNoFollowError() result should match expected")
		})
	}
}
