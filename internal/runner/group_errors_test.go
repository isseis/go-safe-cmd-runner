package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGroupError_ErrorMatchesLegacyAssembly pins that one-line causes produce
// exactly the text the previous wrapping and errors.Join produced, for one and
// for several groups. The expected value is built the old way rather than
// spelled as a literal.
func TestGroupError_ErrorMatchesLegacyAssembly(t *testing.T) {
	causeA := errors.New("first cause")
	causeB := errors.New("second cause")

	single := newGroupError("group-a", causeA)
	assert.Equal(t,
		fmt.Errorf("failed to execute group %s: %w", "group-a", causeA).Error(),
		single.Error())

	groupErrs := newGroupErrors([]*GroupError{
		newGroupError("group-a", causeA),
		newGroupError("group-b", causeB),
	})
	assert.Equal(t,
		errors.Join(
			fmt.Errorf("failed to execute group %s: %w", "group-a", causeA),
			fmt.Errorf("failed to execute group %s: %w", "group-b", causeB),
		).Error(),
		groupErrs.Error())
}

// TestGroupError_IndentsContinuationLines pins that a multi-line cause keeps
// the group's line unindented and indents every continuation line, so a
// continuation stays visually under its own group when several are listed.
func TestGroupError_IndentsContinuationLines(t *testing.T) {
	cause := errors.Join(errors.New("first line"), errors.New("second line"))

	single := newGroupError("group-a", cause)
	assert.Equal(t, "failed to execute group group-a: first line\n  second line", single.Error())

	groupErrs := newGroupErrors([]*GroupError{
		newGroupError("group-a", cause),
		newGroupError("group-b", errors.New("one line")),
	})
	unindented := 0
	for line := range strings.SplitSeq(groupErrs.Error(), "\n") {
		if !strings.HasPrefix(line, "  ") {
			unindented++
		}
	}
	assert.Equal(t, 2, unindented,
		"exactly the two group lines must be unindented:\n%s", groupErrs.Error())
}

// TestGroupError_IndentsTimeoutCause pins the timeout shape: a
// *CommandExecutionError wrapping a joined deadline error still leads with the
// group line and indents the continuation.
func TestGroupError_IndentsTimeoutCause(t *testing.T) {
	timeoutCause := errors.Join(context.DeadlineExceeded, errors.New("signal: killed"))
	cmdErr := &CommandExecutionError{GroupName: "group-a", CommandName: "cmd-a", Err: timeoutCause}

	got := newGroupError("group-a", cmdErr).Error()

	assert.True(t, strings.HasPrefix(got, "failed to execute group group-a: "), "got: %q", got)
	assert.Contains(t, got, "\n  signal: killed", "the continuation must be indented: %q", got)
}

// TestGroupErrors_UnwrapReachesEachCause pins that errors.Is and errors.As
// reach every group's cause, whether one group failed or several. errors.AsType
// returns the first match in chain order, so each type is also reached through
// a single-entry list where it is the only candidate.
func TestGroupErrors_UnwrapReachesEachCause(t *testing.T) {
	sentinelA := errors.New("cause a")
	sentinelB := errors.New("cause b")
	cmdA := &CommandExecutionError{GroupName: "group-a", CommandName: "cmd-a", Err: sentinelA}
	cmdB := &CommandExecutionError{GroupName: "group-b", CommandName: "cmd-b", Err: sentinelB}
	capErr := &output.CaptureError{
		Type:  output.ErrorTypeSizeLimit,
		Path:  "/tmp/out",
		Phase: output.PhaseExecution,
		Cause: output.ErrOutputSizeExceeded,
	}

	singleA := newGroupErrors([]*GroupError{newGroupError("group-a", cmdA)})
	assert.ErrorIs(t, singleA, sentinelA)
	gotA, ok := errors.AsType[*CommandExecutionError](singleA)
	require.True(t, ok, "the first failure must reach its CommandExecutionError")
	assert.Equal(t, "group-a", gotA.GroupName)

	singleB := newGroupErrors([]*GroupError{
		newGroupError("group-b", fmt.Errorf("wrap: %w", errors.Join(cmdB, capErr))),
	})
	assert.ErrorIs(t, singleB, sentinelB)
	assert.ErrorIs(t, singleB, output.ErrOutputSizeExceeded)
	gotCap, ok := errors.AsType[*output.CaptureError](singleB)
	require.True(t, ok, "the capture error must be reachable")
	assert.Equal(t, "/tmp/out", gotCap.Path)

	multi := newGroupErrors([]*GroupError{
		newGroupError("group-a", cmdA),
		newGroupError("group-b", fmt.Errorf("wrap: %w", errors.Join(cmdB, capErr))),
	})
	assert.ErrorIs(t, multi, sentinelA)
	assert.ErrorIs(t, multi, sentinelB)
	assert.ErrorIs(t, multi, output.ErrOutputSizeExceeded)
	gotFirst, ok := errors.AsType[*CommandExecutionError](multi)
	require.True(t, ok)
	assert.Equal(t, "group-a", gotFirst.GroupName,
		"AsType returns the first match in chain order")
}

// TestGroupError_ReadsCommandNameFromCause pins that the command name comes
// from the cause's type, and is empty when the failure names none.
func TestGroupError_ReadsCommandNameFromCause(t *testing.T) {
	stageCause := errors.New("stage cause")

	tests := []struct {
		name  string
		cause error
		want  string
	}{
		{
			name:  "command execution error",
			cause: &CommandExecutionError{GroupName: "g", CommandName: "cmd", Err: errors.New("x")},
			want:  "cmd",
		},
		{
			name:  "command-level stage error",
			cause: newCommandStageError(GroupStageCommandPreparation, "g", "cmd", stageCause),
			want:  "cmd",
		},
		{
			name:  "group-level stage error has no command",
			cause: newGroupStageError(GroupStageGroupPreparation, "g", stageCause),
			want:  "",
		},
		{
			name:  "plain error has no command",
			cause: errors.New("plain"),
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, newGroupError("g", tt.cause).CommandName())
		})
	}
}

// TestGroupErrors_ConstructorsRejectInvalidInput pins that an empty group name,
// a nil cause, an empty list and a nil element are rejected as caller
// mistakes.
func TestGroupErrors_ConstructorsRejectInvalidInput(t *testing.T) {
	cause := errors.New("cause")
	valid := newGroupError("group-a", cause)

	tests := []struct {
		name string
		call func()
	}{
		{name: "empty group name", call: func() { newGroupError("", cause) }},
		{name: "nil cause", call: func() { newGroupError("group-a", nil) }},
		{name: "empty list", call: func() { newGroupErrors(nil) }},
		{name: "nil element", call: func() { newGroupErrors([]*GroupError{valid, nil}) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, tt.call)
		})
	}
}

// TestGroupErrors_ErrorsReturnsCopy pins that neither the slice passed to the
// constructor nor the one returned by Errors can be used to mutate the list
// the value holds.
func TestGroupErrors_ErrorsReturnsCopy(t *testing.T) {
	original := newGroupError("group-a", errors.New("cause"))

	source := []*GroupError{original}
	groupErrs := newGroupErrors(source)
	source[0] = newGroupError("tampered", errors.New("tampered"))
	require.Len(t, groupErrs.Errors(), 1)
	assert.Equal(t, "group-a", groupErrs.Errors()[0].GroupName(),
		"mutating the constructor input must not change the held list")

	returned := groupErrs.Errors()
	returned[0] = newGroupError("tampered", errors.New("tampered"))
	require.Len(t, groupErrs.Errors(), 1)
	assert.Equal(t, "group-a", groupErrs.Errors()[0].GroupName(),
		"mutating the Errors result must not change the held list")
}
