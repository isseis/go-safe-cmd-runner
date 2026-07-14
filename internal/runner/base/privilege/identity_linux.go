//go:build linux

package privilege

import (
	"golang.org/x/sys/unix"
)

// readSavedIDs reads the process's saved-set-user-id and saved-set-group-id via
// Getresuid/Getresgid. These are the uid/gid that a setuid binary's saved-set
// retains after privileges are dropped, and the invariant we verify after
// restoration is that they have not changed since the start of the operation.
//
// On Linux this is the only reliable way to obtain the saved IDs; the standard
// syscall package does not expose Getresuid/Getresgid.
func readSavedIDs() (suid, sgid int) {
	_, suid, _ = unix.Getresuid()
	_, sgid, _ = unix.Getresgid()
	return suid, sgid
}
