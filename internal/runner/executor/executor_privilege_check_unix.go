//go:build !windows

package executor

import (
	"fmt"
	"syscall"
)

// defaultIdentityChecker verifies that the effective UID and GID match the real UID and GID.
// This is the security invariant that must hold outside of any privileged operation:
// the process should never carry elevated identity between commands.
func defaultIdentityChecker() error {
	euid := syscall.Geteuid()
	uid := syscall.Getuid()
	if euid != uid {
		return fmt.Errorf("%w: effective UID %d does not match real UID %d", ErrPrivilegeLeak, euid, uid)
	}

	egid := syscall.Getegid()
	gid := syscall.Getgid()
	if egid != gid {
		return fmt.Errorf("%w: effective GID %d does not match real GID %d", ErrPrivilegeLeak, egid, gid)
	}

	return nil
}
