package errmsg

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The constructor signatures are fixed at compile time. In particular
// IndentedCause takes the cause alone, so a caller has no argument through
// which to pass its own indentation bytes.
var (
	_ func(string) Part    = Const
	_ func(string) Part    = Ident
	_ func(string) Part    = Path
	_ func(string) Part    = Text
	_ func(error) Part     = Cause
	_ func(error) Part     = PathErrorCause
	_ func(error) Part     = IndentedCause
	_ func(string) Summary = ConstSummary
	_ func(string) Summary = TextSummary
)

// textError is an error with an arbitrary text, including the trailing
// newlines an error string should not normally have.
type textError string

func (e textError) Error() string { return string(e) }

// wrapInError returns an *Error whose message is prefix followed by cause.
func wrapInError(prefix string, cause error) *Error {
	return NewError(Text(prefix), Cause(cause))
}

// groupErrorFormat is GroupError.Error()'s formatting of its cause, the
// reference IndentedCause must reproduce.
func groupErrorFormat(text string) string {
	return strings.ReplaceAll(strings.TrimRight(text, "\r\n"), "\n", "\n  ")
}

func TestMessage_SegmentsFollowFlatteningContracts(t *testing.T) {
	structured := NewError(Const("failed to run "), Ident("deploy"), Const(": "), Cause(errors.New("exit status 1")))

	tests := []struct {
		name string
		msg  Message
		want Segments
	}{
		{
			name: "role parts become one segment each",
			msg:  NewMessage(Const("group "), Ident("deploy"), Const(" at "), Path("/tmp/x"), Text(" note")),
			want: Segments{
				{RoleConstant, "group "},
				{RoleIdentifier, "deploy"},
				{RoleConstant, " at "},
				{RolePath, "/tmp/x"},
				{RoleText, " note"},
			},
		},
		{
			name: "an unstructured cause is one text segment",
			msg:  NewMessage(Const("prefix: "), Cause(errors.New("token leaked"))),
			want: Segments{{RoleConstant, "prefix: "}, {RoleText, "token leaked"}},
		},
		{
			name: "a structured cause is expanded with its declared roles",
			msg:  NewMessage(Const("outer: "), Cause(structured)),
			want: Segments{
				{RoleConstant, "outer: "},
				{RoleConstant, "failed to run "},
				{RoleIdentifier, "deploy"},
				{RoleConstant, ": "},
				{RoleText, "exit status 1"},
			},
		},
		{
			// Only the cause's own dynamic type is checked. A structured error
			// behind an unstructured wrapper stays inside one text segment, so
			// the wrapper's own words are not lost.
			name: "a structured error behind an unstructured wrapper is not searched for",
			msg:  NewMessage(Const("outer: "), Cause(fmt.Errorf("middle: %w", structured))),
			want: Segments{{RoleConstant, "outer: "}, {RoleText, "middle: failed to run deploy: exit status 1"}},
		},
		{
			name: "a nil cause renders as <nil>",
			msg:  NewMessage(Const("x: "), Cause(nil)),
			want: Segments{{RoleConstant, "x: "}, {RoleText, "<nil>"}},
		},
		{
			name: "a cause keeps a *fs.PathError as one text segment",
			msg:  NewMessage(Cause(&fs.PathError{Op: "mkdir", Path: "/tmp/token", Err: syscall.EACCES})),
			want: Segments{{RoleText, "mkdir /tmp/token: permission denied"}},
		},
		{
			name: "a path error cause splits a *fs.PathError",
			msg:  NewMessage(PathErrorCause(&fs.PathError{Op: "mkdir", Path: "/tmp/token", Err: syscall.EACCES})),
			want: Segments{
				{RoleText, "mkdir"},
				{RoleConstant, " "},
				{RolePath, "/tmp/token"},
				{RoleConstant, ": "},
				{RoleText, "permission denied"},
			},
		},
		{
			name: "a path error cause with a structured inner error expands it",
			msg:  NewMessage(PathErrorCause(&fs.PathError{Op: "open", Path: "/p", Err: structured})),
			want: Segments{
				{RoleText, "open"},
				{RoleConstant, " "},
				{RolePath, "/p"},
				{RoleConstant, ": "},
				{RoleConstant, "failed to run "},
				{RoleIdentifier, "deploy"},
				{RoleConstant, ": "},
				{RoleText, "exit status 1"},
			},
		},
		{
			name: "a path error cause with a nil inner error renders <nil>",
			msg:  NewMessage(PathErrorCause(&fs.PathError{Op: "chmod", Path: "/p"})),
			want: Segments{{RoleText, "chmod"}, {RoleConstant, " "}, {RolePath, "/p"}, {RoleConstant, ": "}, {RoleText, "<nil>"}},
		},
		{
			name: "a path error cause that is not a *fs.PathError is a plain cause",
			msg:  NewMessage(PathErrorCause(fmt.Errorf("wrapped: %w", &fs.PathError{Op: "mkdir", Path: "/p", Err: syscall.EACCES}))),
			want: Segments{{RoleText, "wrapped: mkdir /p: permission denied"}},
		},
		{
			name: "a nil path error cause renders as <nil>",
			msg:  NewMessage(PathErrorCause(nil)),
			want: Segments{{RoleText, "<nil>"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.msg.Segments())
		})
	}
}

func TestMessage_PathErrorCauseMatchesPathErrorText(t *testing.T) {
	pe := &fs.PathError{Op: "chmod", Path: "/tmp/scr-run-123", Err: syscall.EPERM}
	err := NewError(Const("failed to set permissions on temporary directory: "), PathErrorCause(pe))

	assert.Equal(t, "failed to set permissions on temporary directory: "+pe.Error(), err.Error())
	assert.Equal(t, fmt.Errorf("failed to set permissions on temporary directory: %w", pe).Error(), err.Error())
	assert.ErrorIs(t, err, syscall.EPERM)
	got, ok := errors.AsType[*fs.PathError](err)
	require.True(t, ok)
	assert.Same(t, pe, got)
}

func TestMessage_RolePartsAndCausePartsAreDistinct(t *testing.T) {
	assert.Equal(t, "<nil>", NewMessage(Cause(nil)).String())
	assert.Equal(t, Segments{{RoleText, ""}}, NewMessage(Text("")).Segments())
	assert.Empty(t, NewMessage(Text("")).String())
	assert.Empty(t, Message{}.String())
	assert.Empty(t, Message{}.Segments())
}

func TestMessage_StringEqualsErrorForStructuredTypes(t *testing.T) {
	boom := errors.New("boom")
	second := errors.New("second")
	inner := NewError(Const("inner "), Ident("name"), Const(": "), Cause(boom))
	joined := Join(inner, second)
	tests := []struct {
		name string
		err  Structured
		want string
	}{
		{name: "Error", err: inner, want: fmt.Errorf("inner name: %w", boom).Error()},
		{
			name: "Error wrapping JoinedError",
			err:  wrapInError("outer: ", joined),
			want: fmt.Errorf("outer: %w", errors.Join(fmt.Errorf("inner name: %w", boom), second)).Error(),
		},
		{name: "JoinedError", err: joined.(*JoinedError), want: "inner name: boom\nsecond"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.StructuredMessage().String())
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestIndentedCause_MatchesGroupErrorFormatting(t *testing.T) {
	tests := []struct {
		name  string
		cause error
		want  Segments
	}{
		{
			name:  "unstructured cause with continuation lines and trailing newline",
			cause: textError("line one\nline two\n"),
			want:  Segments{{RoleText, "line one\n  line two"}},
		},
		{
			name:  "trailing newline-only segment is dropped",
			cause: NewError(Const("cause"), Cause(textError("\n"))),
			want:  Segments{{RoleConstant, "cause"}},
		},
		{
			name:  "trailing CRLF sequence spans segments",
			cause: NewError(Text("cause\r"), Const("\n"), Cause(textError("\r\n"))),
			want:  Segments{{RoleText, "cause"}},
		},
		{
			name:  "originally empty trailing segment is kept and the newline before it removed",
			cause: NewError(Text("cause\n"), Cause(errors.New(""))),
			want:  Segments{{RoleText, "cause"}, {RoleText, ""}},
		},
		{
			name:  "newlines in the middle and inside identifier and path segments are indented",
			cause: NewError(Const("a\n"), Ident("id\nx"), Path("/p\nq"), Cause(errors.New("tail\nend"))),
			want: Segments{
				{RoleConstant, "a\n  "}, {RoleIdentifier, "id\n  x"}, {RolePath, "/p\n  q"}, {RoleText, "tail\n  end"},
			},
		},
		{
			name:  "all newlines trims to nothing",
			cause: NewError(Const("\n"), Cause(textError("\r\n"))),
			want:  nil,
		},
		{
			name:  "nil cause",
			cause: nil,
			want:  Segments{{RoleText, "<nil>"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewMessage(IndentedCause(tt.cause))
			assert.Equal(t, tt.want, msg.Segments())
			if tt.cause != nil {
				assert.Equal(t, groupErrorFormat(tt.cause.Error()), msg.String())
			}
		})
	}
}

func TestMessage_DeepChainMatchesFmtErrorfChain(t *testing.T) {
	const depth = 64
	root := errors.New("root cause")
	structured, formatted := root, root
	for i := range depth {
		prefix := "layer " + strconv.Itoa(i) + ": "
		structured = NewError(Text(prefix), Cause(structured))
		formatted = fmt.Errorf("%s%w", prefix, formatted)
	}

	assert.Equal(t, formatted.Error(), structured.Error())
	assert.ErrorIs(t, structured, root)
}

func TestNewError_WrapsExactlyOneCause(t *testing.T) {
	cause := &fs.PathError{Op: "open", Path: "/p", Err: fs.ErrNotExist}
	err := NewError(Const("failed: "), Cause(cause))

	assert.Same(t, cause, err.Unwrap())
	assert.ErrorIs(t, err, fs.ErrNotExist)
	got, ok := errors.AsType[*fs.PathError](err)
	require.True(t, ok)
	assert.Same(t, cause, got)
	assert.Equal(t, "failed: open /p: file does not exist", err.Error())
}

func TestNewError_RejectsInvalidCauses(t *testing.T) {
	tests := []struct {
		name  string
		parts []Part
	}{
		{name: "no cause part", parts: []Part{Const("x")}},
		{name: "no parts", parts: nil},
		{name: "two cause parts", parts: []Part{Cause(errors.New("a")), Cause(errors.New("b"))}},
		{name: "nil cause", parts: []Part{Const("x: "), Cause(nil)}},
		{name: "nil path error cause", parts: []Part{PathErrorCause(nil)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() { NewError(tt.parts...) })
		})
	}
}

func TestJoin_MatchesErrorsJoin(t *testing.T) {
	first := errors.New("first")
	second := NewError(Const("second "), Ident("g"), Const(": "), Cause(errors.New("boom")))
	third := errors.New("third\nwith two lines")

	tests := []struct {
		name string
		errs []error
	}{
		{name: "several children", errs: []error{first, second, third}},
		{name: "nil children are dropped", errs: []error{nil, first, nil, second}},
		{name: "one child", errs: []error{third}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Join(tt.errs...)
			require.Error(t, got)
			assert.Equal(t, errors.Join(tt.errs...).Error(), got.Error())
			for _, child := range tt.errs {
				if child != nil {
					assert.ErrorIs(t, got, child)
				}
			}
		})
	}

	t.Run("all nil returns nil", func(t *testing.T) {
		assert.NoError(t, Join(nil, nil))
		assert.NoError(t, Join())
	})

	t.Run("structured children keep their roles", func(t *testing.T) {
		got := Join(first, second).(*JoinedError).StructuredMessage().Segments()
		assert.Equal(t, Segments{
			{RoleText, "first"},
			{RoleConstant, "\n"},
			{RoleConstant, "second "},
			{RoleIdentifier, "g"},
			{RoleConstant, ": "},
			{RoleText, "boom"},
		}, got)
	})
}

// countingError returns a different text on every Error() call.
type countingError struct {
	calls int
}

func (e *countingError) Error() string {
	e.calls++
	return "call " + strconv.Itoa(e.calls)
}

func TestMessage_FreezeEvaluatesCausesOnce(t *testing.T) {
	cause := &countingError{}
	msg := NewMessage(Const("prefix "), Ident("g"), Const(": "), Cause(cause))

	frozen := msg.Freeze()
	require.Equal(t, 1, cause.calls)

	assert.Equal(t, "prefix g: call 1", frozen.String())
	assert.Equal(t, Segments{{RoleConstant, "prefix "}, {RoleIdentifier, "g"}, {RoleConstant, ": "}, {RoleText, "call 1"}}, frozen.Segments())
	assert.Equal(t, frozen.String(), frozen.Freeze().String())
	assert.Equal(t, 1, cause.calls, "a frozen message must not call the cause's Error() again")

	// The unfrozen message evaluates the cause again each time.
	assert.Equal(t, "prefix g: call 2", msg.String())
}

func TestMerge_KeepsPartsOfEachMessage(t *testing.T) {
	a := NewError(Const("failed to execute group "), Ident("alpha"), Const(": "), Cause(errors.New("x")))
	b := NewError(Const("failed to execute group "), Ident("beta"), Const(": "), Cause(errors.New("y")))

	merged := Merge(a.StructuredMessage(), NewMessage(Const("\n")), b.StructuredMessage())

	assert.Equal(t, a.Error()+"\n"+b.Error(), merged.String())
	var idents []string
	for _, s := range merged.Segments() {
		if s.Role == RoleIdentifier {
			idents = append(idents, s.Text)
		}
	}
	assert.Equal(t, []string{"alpha", "beta"}, idents)
	assert.Empty(t, Merge().String())
}

func TestNewMessage_CopiesParts(t *testing.T) {
	parts := []Part{Const("a"), Const("b")}
	msg := NewMessage(parts...)
	parts[0] = Const("changed")

	assert.Equal(t, "ab", msg.String())
}

func TestSummary(t *testing.T) {
	tests := []struct {
		name     string
		summary  Summary
		wantText string
		wantRole Role
	}{
		{name: "constant", summary: ConstSummary("Failed to load"), wantText: "Failed to load", wantRole: RoleConstant},
		{name: "text", summary: TextSummary("token rotate"), wantText: "token rotate", wantRole: RoleText},
		{name: "zero value", summary: Summary{}, wantText: "", wantRole: RoleText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantText, tt.summary.String())
			assert.Equal(t, Segments{{tt.wantRole, tt.wantText}}, NewMessage(tt.summary.Part()).Segments())
		})
	}
}

func TestMessage_ZeroValuesAndUndefinedRoles(t *testing.T) {
	t.Run("zero part is an empty text segment", func(t *testing.T) {
		assert.Equal(t, Segments{{RoleText, ""}}, NewMessage(Part{}).Segments())
	})

	t.Run("a role outside the declared set is carried and rendered", func(t *testing.T) {
		msg := NewMessage(Const("a "), Part{role: Role(99), text: "b"})
		assert.Equal(t, "a b", msg.String())
		assert.Equal(t, Segments{{RoleConstant, "a "}, {Role(99), "b"}}, msg.Segments())
	})

	t.Run("zero values do not panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			assert.Empty(t, Message{}.String())
			assert.Empty(t, Message{}.Freeze().String())
			assert.Empty(t, Summary{}.String())
			assert.Empty(t, (&Error{}).Error())
			require.NoError(t, (&Error{}).Unwrap())
			assert.Empty(t, (&JoinedError{}).Error())
			assert.Empty(t, (&JoinedError{}).Unwrap())
		})
	})
}

func TestMessage_LogValueReturnsUnredactedString(t *testing.T) {
	msg := NewMessage(Const("failed to expand group "), Ident("token-rotate"), Const(": "),
		Cause(errors.New("password=hunter2")))

	for _, tt := range []struct {
		name    string
		handler func(*bytes.Buffer) slog.Handler
		want    string
	}{
		{
			name:    "text handler",
			handler: func(b *bytes.Buffer) slog.Handler { return slog.NewTextHandler(b, nil) },
			want:    "error_message=" + strconv.Quote(msg.String()),
		},
		{
			name:    "json handler",
			handler: func(b *bytes.Buffer) slog.Handler { return slog.NewJSONHandler(b, nil) },
			want:    `"error_message":"failed to expand group token-rotate: password=hunter2"`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.New(tt.handler(&buf)).Error("report", slog.Any("error_message", msg))
			assert.Contains(t, buf.String(), tt.want)
		})
	}
}
