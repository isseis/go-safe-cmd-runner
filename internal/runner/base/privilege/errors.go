// Package privilege provides secure privilege escalation functionality for command execution.
package privilege

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// Standard errors
var (
	ErrPrivilegeElevationFailed        = fmt.Errorf("failed to elevate privileges")
	ErrPrivilegeRestorationFailed      = fmt.Errorf("failed to restore privileges")
	ErrInvalidUID                      = fmt.Errorf("invalid user ID")
	ErrPrivilegedExecutionNotSupported = fmt.Errorf("privileged execution not supported")
)

// ErrReentrantPrivilegeCall is returned when WithPrivileges is called from within
// a privilege window on the same manager.
var ErrReentrantPrivilegeCall = errors.New("reentrant WithPrivileges call")

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
	return e.StructuredMessage().String()
}

// StructuredMessage declares the command name as an identifier; the operation,
// the uids and the system-call error are text. Timestamp is not rendered.
func (e *Error) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("privilege operation '"),
		errmsg.Text(string(e.Operation)),
		errmsg.Const("' failed for command '"),
		errmsg.Ident(e.CommandName),
		errmsg.Const("' (uid "),
		errmsg.Text(strconv.Itoa(e.OriginalUID)),
		errmsg.Const("->"),
		errmsg.Text(strconv.Itoa(e.TargetUID)),
		errmsg.Const("): "),
		errmsg.Cause(e.SyscallErr),
	)
}

func (e *Error) Unwrap() error {
	return e.SyscallErr
}
