//go:build netbsd

package filevalidator

import (
	"os"
	"syscall"
	"testing"
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
			if got := isNoFollowError(tt.err); got != tt.want {
				t.Errorf("isNoFollowError() = %v, want %v", got, tt.want)
			}
		})
	}
}
