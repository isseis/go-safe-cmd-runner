//go:build linux

package privilege

import (
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// ErrInvalidSavedSetIDs is returned by readSavedIDs when the kernel returns
// a negative saved-set user or group ID, which indicates an unrecoverable
// kernel state error.
var ErrInvalidSavedSetIDs = errors.New("invalid saved-set IDs from kernel: negative uid or gid")

// readSavedIDs reads the process's saved-set-user-id and saved-set-group-id via
// Getresuid/Getresgid. These are the uid/gid that a setuid binary's saved-set
// retains after privileges are dropped, and the invariant we verify after
// restoration is that they have not changed since the start of the operation.
//
// Getresuid returns (ruid, euid, suid) — the third return value is the
// saved-set-uid. Getresgid returns the same layout for GIDs.
// On Linux this is the only reliable way to obtain the saved IDs; the standard
// syscall package does not expose Getresuid/Getresgid.
//
// The syscall always succeeds on Linux, but we validate the returned values
// are non-negative. A negative value would indicate an unrecoverable kernel
// state error; returning an error lets the caller fail-closed rather than
// silently accepting a zero-value (root) result.
func readSavedIDs() (suid, sgid int, err error) {
	_, _, suid = unix.Getresuid()
	_, _, sgid = unix.Getresgid()
	if suid < 0 || sgid < 0 {
		return 0, 0, fmt.Errorf("readSavedIDs: %w (suid=%d, sgid=%d)", ErrInvalidSavedSetIDs, suid, sgid)
	}
	return suid, sgid, nil
}
