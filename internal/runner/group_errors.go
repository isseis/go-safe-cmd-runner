package runner

import (
	"errors"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
)

// GroupError is one group that failed during a run. Its fields are unexported
// so it is built only through newGroupError.
type GroupError struct {
	group   string // GroupSpec.Name of the failed group
	command string // command the failure belongs to, or "" when none
	err     error  // the error ExecuteGroup returned
}

// GroupName returns the GroupSpec.Name of the failed group.
func (e *GroupError) GroupName() string {
	return e.group
}

// CommandName returns the command the failure belongs to, or an empty string
// when the failure is not attributable to one command.
func (e *GroupError) CommandName() string {
	return e.command
}

// Error renders the structured message without redaction. The cause's
// continuation lines are indented so they stay visually under the group's line
// when several groups are reported one after another.
func (e *GroupError) Error() string {
	return e.StructuredMessage().String()
}

// StructuredMessage declares the group as an Identifier and indents the
// cause's continuation lines.
func (e *GroupError) StructuredMessage() errmsg.Message {
	return errmsg.NewMessage(
		errmsg.Const("failed to execute group "),
		errmsg.Ident(e.group),
		errmsg.Const(": "),
		errmsg.IndentedCause(e.err),
	)
}

// Unwrap returns the cause so errors.Is and errors.As reach the underlying
// error.
func (e *GroupError) Unwrap() error {
	return e.err
}

// GroupErrors reports every group that failed during a run. It always holds at
// least one GroupError, and a single failure is represented the same way as
// several so callers never branch on the shape.
type GroupErrors struct {
	errs []*GroupError
}

// Errors returns a copy of the failed groups, so a caller cannot mutate the
// list the value holds.
func (e *GroupErrors) Errors() []*GroupError {
	return append([]*GroupError(nil), e.errs...)
}

// Error renders the structured message without redaction.
func (e *GroupErrors) Error() string {
	return e.StructuredMessage().String()
}

// StructuredMessage joins each group's message with a newline. It merges the
// messages without flattening them, so every group keeps the roles its error
// declared.
func (e *GroupErrors) StructuredMessage() errmsg.Message {
	msgs := make([]errmsg.Message, 0, max(0, 2*len(e.errs)-1))
	for i, groupErr := range e.errs {
		if i > 0 {
			msgs = append(msgs, errmsg.NewMessage(errmsg.Const("\n")))
		}
		msgs = append(msgs, groupErr.StructuredMessage())
	}
	return errmsg.Merge(msgs...)
}

// Unwrap returns each group's error so errors.Is and errors.As reach every
// cause. The shape is not what declares several failures; the count returned
// by Errors is.
func (e *GroupErrors) Unwrap() []error {
	unwrapped := make([]error, len(e.errs))
	for i, err := range e.errs {
		unwrapped[i] = err
	}
	return unwrapped
}

// newGroupError builds the failure of one group. It panics on an empty group
// name or a nil cause: those are caller mistakes, not runtime inputs.
//
// The command name is read from the cause's chain by type: a
// *CommandExecutionError's CommandName when present, else a *GroupStageError's
// CommandName(), else an empty string.
func newGroupError(group string, err error) *GroupError {
	if group == "" {
		panic("newGroupError: group name must not be empty")
	}
	if err == nil {
		panic("newGroupError: cause must not be nil")
	}
	command := ""
	if cmdErr, ok := errors.AsType[*CommandExecutionError](err); ok {
		command = cmdErr.CommandName
	} else if stageErr, ok := errors.AsType[*GroupStageError](err); ok {
		command = stageErr.CommandName()
	}
	return &GroupError{group: group, command: command, err: err}
}

// newGroupErrors builds the failure list of a run. It panics on an empty list
// or a nil element, and keeps a copy of errs so the caller cannot mutate it.
func newGroupErrors(errs []*GroupError) *GroupErrors {
	if len(errs) == 0 {
		panic("newGroupErrors: at least one group error is required")
	}
	for _, err := range errs {
		if err == nil {
			panic("newGroupErrors: group error must not be nil")
		}
	}
	return &GroupErrors{errs: append([]*GroupError(nil), errs...)}
}
