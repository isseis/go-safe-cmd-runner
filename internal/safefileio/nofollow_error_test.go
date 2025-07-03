//go:build !netbsd

package safefileio

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
		// POSIX system returns ELOOP when opening a symlink with O_NOFOLLOW
		{
			name: "ELOOP error",
			err:  &os.PathError{Err: syscall.ELOOP},
			want: true,
		},
		// FreeBSD returns EMLINK when opening a symlink with O_NOFOLLOW
		{
			name: "EMLINK error",
			err:  &os.PathError{Err: syscall.EMLINK},
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
