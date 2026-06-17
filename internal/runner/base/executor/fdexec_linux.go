//go:build linux

package executor

import (
	"fmt"
	"os"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/risktypes"
)

// fdExecSupported reports whether fd-bound execution via /proc/self/fd is
// available. On Linux it is, provided /proc is mounted (verified at exec time by
// the kernel resolving the /proc/self/fd path).
func fdExecSupported() bool { return true }

// fdExecExtraFile prepares fd-bound execution for the verified identity.
//
// It duplicates the verified descriptor so the returned *os.File owns an
// independent descriptor: the original stays owned by id.FD (a VerifiedFD) and
// the duplicate is owned by the *os.File. Both are closed exactly once, by
// different owners, so there is no double-close. The caller passes the *os.File
// as the child's only ExtraFiles entry (child fd 3) and execs childPath
// (/proc/self/fd/3), which the kernel resolves to the verified inode regardless
// of any later rename or symlink swap of the original path (closing the TOCTOU
// window). The caller must close the returned *os.File after the child
// has started (or failed to start).
func fdExecExtraFile(id *risktypes.VerifiedIdentity) (childPath string, f *os.File, err error) {
	if id == nil || id.FD == nil {
		return "", nil, ErrNoVerifiedFD
	}
	// dup(2) does not set FD_CLOEXEC; os/exec clears CLOEXEC for inherited
	// ExtraFiles on the child side regardless, so the child receives the
	// descriptor at fd 3.
	dup, err := syscall.Dup(id.FD.Fd())
	if err != nil {
		return "", nil, fmt.Errorf("duplicate verified fd: %w", err)
	}
	f = os.NewFile(uintptr(dup), id.ResolvedPath) // #nosec G115 -- dup is a valid non-negative fd from syscall.Dup; int->uintptr cannot overflow
	if f == nil {
		_ = syscall.Close(dup)
		return "", nil, ErrNoVerifiedFD
	}
	// ExtraFiles[0] becomes child fd 3 (3 + index 0).
	return "/proc/self/fd/3", f, nil
}
