// Package privilege provides secure privilege escalation functionality for command execution.
package privilege

import (
	"fmt"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Standard errors
var (
	ErrPrivilegedExecutionNotAvailable = fmt.Errorf("privileged execution not available (setuid not configured)")
	ErrPrivilegeElevationFailed        = fmt.Errorf("failed to elevate privileges")
	ErrPrivilegeRestorationFailed      = fmt.Errorf("failed to restore privileges")
	ErrPlatformNotSupported            = fmt.Errorf("privileged execution not supported on this platform")
	ErrInvalidUID                      = fmt.Errorf("invalid user ID")
)

// Error contains detailed information about privilege operation failures
type Error struct {
	Operation   runnertypes.Operation
	CommandName string
	OriginalUID int
	TargetUID   int
	SyscallErr  error
	Timestamp   time.Time
}

func (e *Error) Error() string {
	return fmt.Sprintf("privilege operation '%s' failed for command '%s' (uid %d->%d): %v",
		e.Operation, e.CommandName, e.OriginalUID, e.TargetUID, e.SyscallErr)
}

func (e *Error) Unwrap() error {
	return e.SyscallErr
}
