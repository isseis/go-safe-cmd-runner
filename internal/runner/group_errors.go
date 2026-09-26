package runner

import (
	"errors"
	"fmt"
	"strings"
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

// Error reports the group and its cause. The cause's continuation lines are
// indented so they stay visually under the group's line when several groups
// are reported one after another.
func (e *GroupError) Error() string {
	cause := strings.TrimRight(e.err.Error(), "\r\n")
	return fmt.Sprintf("failed to execute group %s: %s", e.group, strings.ReplaceAll(cause, "\n", "\n  "))
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

// Error joins each group's report with a newline. For one-line causes this is
// the same text the previous wrapping and errors.Join produced.
func (e *GroupErrors) Error() string {
	parts := make([]string, len(e.errs))
	for i, err := range e.errs {
		parts[i] = err.Error()
	}
	return strings.Join(parts, "\n")
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
