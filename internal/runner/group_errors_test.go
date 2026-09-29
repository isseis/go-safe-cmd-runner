package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
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

// TestGroupErrorStructuredRoles pins the role sequence a group failure
// declares: fixed wording as Constant, the group name as Identifier, and the
// cause's own text as Text, so the name is exempt from redaction while the
// cause keeps the full protection.
func TestGroupErrorStructuredRoles(t *testing.T) {
	groupErr := newGroupError("token-rotate", errors.New("plain cause"))

	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "failed to execute group "},
		{Role: errmsg.RoleIdentifier, Text: "token-rotate"},
		{Role: errmsg.RoleConstant, Text: ": "},
		{Role: errmsg.RoleText, Text: "plain cause"},
	}, groupErr.StructuredMessage().Segments())
}

// TestCommandExecutionErrorStructuredRoles pins the role sequence a command
// failure declares: the command and group names are Identifiers and the cause
// is carried as Text unless the cause is itself structured.
func TestCommandExecutionErrorStructuredRoles(t *testing.T) {
	cmdErr := &CommandExecutionError{
		GroupName:   "token-rotate",
		CommandName: "renew",
		Err:         errors.New("plain cause"),
	}

	assert.Equal(t, errmsg.Segments{
		{Role: errmsg.RoleConstant, Text: "command "},
		{Role: errmsg.RoleIdentifier, Text: "renew"},
		{Role: errmsg.RoleConstant, Text: " in group "},
		{Role: errmsg.RoleIdentifier, Text: "token-rotate"},
		{Role: errmsg.RoleConstant, Text: " failed: "},
		{Role: errmsg.RoleText, Text: "plain cause"},
	}, cmdErr.StructuredMessage().Segments())
}

// TestGroupErrors_MergePreservesIdentifierSegments pins that a multi-group
// failure keeps every group's declared Identifier segments. Merging the
// messages is what preserves them; flattening the whole report to one Text
// would drop the roles.
func TestGroupErrors_MergePreservesIdentifierSegments(t *testing.T) {
	inner := errmsg.NewError(
		errmsg.Const("failed to expand group["),
		errmsg.Ident("api_key"),
		errmsg.Const("]"),
		errmsg.Cause(errors.New("plain cause")),
	)
	groupErrs := newGroupErrors([]*GroupError{
		newGroupError("token-rotate", inner),
		newGroupError("monkey-test", inner),
	})

	var identifiers []string
	for _, segment := range groupErrs.StructuredMessage().Segments() {
		if segment.Role == errmsg.RoleIdentifier {
			identifiers = append(identifiers, segment.Text)
		}
	}
	assert.Equal(t, []string{"token-rotate", "api_key", "monkey-test", "api_key"}, identifiers,
		"merging must keep every group's declared Identifier segments")
}

// TestStructuredRunnerErrorZeroValuesDoNotPanic pins that a zero value never
// panics while it is rendered.
func TestStructuredRunnerErrorZeroValuesDoNotPanic(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "GroupError", err: &GroupError{}, want: "failed to execute group : <nil>"},
		{name: "GroupErrors", err: &GroupErrors{}, want: ""},
		{name: "GroupStageError", err: &GroupStageError{}, want: "group pre-execution failed"},
		{name: "CommandExecutionError", err: &CommandExecutionError{}, want: "command  in group  failed: <nil>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotPanics(t, func() { _ = tt.err.Error() })
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
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
	// Build the capture error through the production path, so this test does
	// not depend on CaptureError's fields staying exported.
	capErr := (&output.Capture{OutputPath: "/tmp/out", MaxSize: 1}).WriteOutput([]byte("too much"))
	require.Error(t, capErr, "the bounded capture must reject an oversized write")
	require.Contains(t, capErr.Error(), "/tmp/out", "the capture error must name its path")

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
	_, ok = errors.AsType[*output.CaptureError](singleB)
	require.True(t, ok, "the capture error must be reachable")

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
