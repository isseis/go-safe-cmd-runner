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

// IndentedCause takes the cause alone, so a caller has no argument through
// which to pass its own indentation bytes.
var _ func(error) Part = IndentedCause

// textError is an error with an arbitrary text, including the trailing
// newlines an error string should not normally have.
type textError string

func (e textError) Error() string { return string(e) }

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
			name: "a path error cause that is not a *fs.PathError is a plain cause",
			msg:  NewMessage(PathErrorCause(fmt.Errorf("wrapped: %w", &fs.PathError{Op: "mkdir", Path: "/p", Err: syscall.EACCES}))),
			want: Segments{{RoleText, "wrapped: mkdir /p: permission denied"}},
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

	assert.Equal(t, fmt.Errorf("failed to set permissions on temporary directory: %w", pe).Error(), err.Error())
}

// A JoinedError nested as the cause of an Error renders like errors.Join
// nested in fmt.Errorf; the single-layer forms are covered by the Join and
// deep-chain tests.
func TestMessage_StringEqualsErrorForStructuredTypes(t *testing.T) {
	boom := errors.New("boom")
	second := errors.New("second")
	inner := NewError(Const("inner "), Ident("name"), Const(": "), Cause(boom))

	got := NewError(Text("outer: "), Cause(Join(inner, second)))

	assert.Equal(t, fmt.Errorf("outer: %w", errors.Join(fmt.Errorf("inner name: %w", boom), second)).Error(), got.Error())
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := NewMessage(IndentedCause(tt.cause))
			assert.Equal(t, tt.want, msg.Segments())
			assert.Equal(t, groupErrorFormat(tt.cause.Error()), msg.String())
		})
	}
}

func TestMessage_DeepChainMatchesFmtErrorfChain(t *testing.T) {
	const depth = 64
	root := errors.New("root cause")
	structured, formatted := root, root
	for i := range depth {
		name := "group" + strconv.Itoa(i)
		structured = NewError(Const("layer "), Ident(name), Const(": "), Cause(structured))
		formatted = fmt.Errorf("layer %s: %w", name, formatted)
	}

	assert.Equal(t, formatted.Error(), structured.Error())
	assert.ErrorIs(t, structured, root)
	// Every layer keeps its declared role, down to the innermost one.
	segments := structured.(*Error).StructuredMessage().Segments()
	require.Len(t, segments, 3*depth+1)
	for i := range depth {
		assert.Equal(t, Segment{RoleIdentifier, "group" + strconv.Itoa(depth-1-i)}, segments[3*i+1])
	}
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
		{name: "two cause parts", parts: []Part{Cause(errors.New("a")), Cause(errors.New("b"))}},
		{name: "nil cause", parts: []Part{Const("x: "), Cause(nil)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Panics(t, func() { NewError(tt.parts...) })
		})
	}
}

// TestQuoted_MatchesStrconvQuote pins Quoted's rendering to fmt's %q of the
// unquoted text, byte for byte, for inputs whose escaping differs from the
// raw text, while each inner segment keeps its declared role.
func TestQuoted_MatchesStrconvQuote(t *testing.T) {
	tests := []struct {
		name  string
		parts []Part
	}{
		{name: "empty", parts: nil},
		{name: "plain identifier", parts: []Part{Ident("deploy")}},
		{name: "quote and backslash", parts: []Part{Ident(`a"b\c`)}},
		{name: "non-ASCII", parts: []Part{Ident("d\u00e9pl\u00f6y\u65e5")}},
		{name: "control and non-printable", parts: []Part{Text("tab\there\n\x00\u200b")}},
		{name: "invalid UTF-8", parts: []Part{Text("bad\xff\xfe")}},
		{name: "incomplete rune at a boundary stays invalid", parts: []Part{Text("x\xc3"), Const("A")}},
		{name: "several roles", parts: []Part{Const("group["), Ident(`g"1`), Const("]."), Path(`/p\q`), Text("\u00e9")}},
		{name: "nested quoted", parts: []Part{Const("a "), Quoted(Ident(`b"c`))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := NewMessage(tt.parts...)
			quoted := NewMessage(Quoted(tt.parts...))
			assert.Equal(t, strconv.Quote(raw.String()), quoted.String())
			assert.Equal(t, fmt.Sprintf("%q", raw.String()), quoted.String())

			// The marks are constants and the inner segments keep their roles.
			rawSegments := raw.Segments()
			got := quoted.Segments()
			require.Len(t, got, len(rawSegments)+2)
			assert.Equal(t, Segment{RoleConstant, `"`}, got[0])
			assert.Equal(t, Segment{RoleConstant, `"`}, got[len(got)-1])
			for i, s := range rawSegments {
				q := strconv.Quote(s.Text)
				assert.Equal(t, Segment{s.Role, q[1 : len(q)-1]}, got[i+1])
			}
		})
	}
}

// TestQuoted_SplitRuneFallsBackToText pins the fail-closed rendering: a UTF-8
// character split across two segments cannot be escaped segment by segment,
// so the whole quoted text becomes one RoleText segment.
func TestQuoted_SplitRuneFallsBackToText(t *testing.T) {
	const word = "caf\u00e9"
	cut := len(word) - 1 // inside the two-byte encoding of U+00E9
	msg := NewMessage(Const("x "), Quoted(Ident(word[:cut]), Ident(word[cut:])), Const(" y"))

	assert.Equal(t, Segments{
		{RoleConstant, "x "},
		{RoleText, strconv.Quote(word)},
		{RoleConstant, " y"},
	}, msg.Segments())
	assert.Equal(t, "x "+strconv.Quote(word)+" y", msg.String())
}

// TestQuoted_RejectsCauses pins that a cause, directly or as the only part,
// cannot be quoted.
func TestQuoted_RejectsCauses(t *testing.T) {
	assert.Panics(t, func() { Quoted(Cause(errors.New("boom"))) })
	assert.Panics(t, func() { Quoted(Const("a"), PathErrorCause(errors.New("boom"))) })
	assert.Panics(t, func() { Quoted(IndentedCause(errors.New("boom"))) })
}

// TestQuoted_IsNotACauseOfNewError pins that a quoted part never counts as the
// cause NewError requires, and that Freeze keeps the quoted segments.
func TestQuoted_IsNotACauseOfNewError(t *testing.T) {
	assert.Panics(t, func() { NewError(Quoted(Ident("x"))) })

	cause := errors.New("boom")
	err := NewError(Const("command "), Quoted(Ident(`b"x`)), Const(": "), Cause(cause))
	assert.Equal(t, fmt.Sprintf("command %q: %v", `b"x`, cause), err.Error())
	assert.Equal(t, err.StructuredMessage().Segments(), err.StructuredMessage().Freeze().Segments())
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
			assert.Empty(t, Message{}.Freeze().String())
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

	var buf bytes.Buffer
	slog.New(slog.NewTextHandler(&buf, nil)).Error("report", slog.Any("error_message", msg))
	assert.Contains(t, buf.String(), "error_message="+strconv.Quote(msg.String()))
}
