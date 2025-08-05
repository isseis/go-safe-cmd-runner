// Package privilege provides secure privilege escalation functionality for command execution.
package privilege

import (
	"fmt"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
)

// Standard errors
var (
	ErrPrivilegeElevationFailed        = fmt.Errorf("failed to elevate privileges")
	ErrPrivilegeRestorationFailed      = fmt.Errorf("failed to restore privileges")
	ErrInvalidUID                      = fmt.Errorf("invalid user ID")
	ErrPrivilegedExecutionNotSupported = fmt.Errorf("privileged execution not supported")
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
