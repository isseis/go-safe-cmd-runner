// Package errmsg carries an error body as a sequence of parts whose redaction
// role is declared by type when the error is built. Redaction reads the role
// of each flattened segment; it never infers a role from the text. The package
// is a leaf: it imports only the standard library, and the unexported fields
// of Part keep the choice of role inside the constructors of this package.
package errmsg

import (
	"io/fs"
	"log/slog"
	"slices"
	"strconv"
	"strings"
)

// Role declares how redaction treats a segment. The zero value is RoleText,
// the role that assumes the least about its text.
type Role int

const (
	// RoleText is free text: it receives every redaction stage.
	RoleText Role = iota
	// RoleConstant is fixed wording taken from a constant expression.
	RoleConstant
	// RoleIdentifier is a name defined in the configuration, such as a group,
	// command or variable name.
	RoleIdentifier
	// RolePath is a file system path.
	RolePath
)

// causeKind declares how a cause part is flattened. The zero value is a
// plain cause.
type causeKind int

const (
	causePlain causeKind = iota
	causePathError
	causeIndented
)

// partKind declares whether a Part carries a role and text, a cause, or a
// quoted sequence of parts. The zero value is a role part.
type partKind int

const (
	partRole partKind = iota
	partCause
	partQuoted
)

// quoteMark is the quotation mark Quoted renders around its parts, the one
// strconv.Quote uses.
const quoteMark = `"`

// continuationIndent is inserted after each newline of an indented cause.
// It is a constant of this package, so a caller cannot put its own bytes
// into a segment through the indentation.
const continuationIndent = "  "

// Part is one element of a Message. Its fields are unexported, so a part
// can only be built by the constructors in this package.
type Part struct {
	kind      partKind
	role      Role
	text      string
	cause     error
	causeKind causeKind
	quoted    []Part
}

// Message is an error body as a sequence of parts. The zero value is an
// empty body.
type Message struct {
	parts []Part
}

// Segment is one flattened element: a role and its unredacted text. The zero
// value is an empty RoleText segment.
type Segment struct {
	Role Role
	Text string
}

// Segments is the result of flattening a Message.
type Segments []Segment

// Summary is the summary line of a report. It holds a constant or free text
// only; the zero value is an empty text.
type Summary struct {
	part Part
}

// Error is the general-purpose structured error used in place of
// fmt.Errorf on the paths in scope. It wraps exactly one non-nil cause.
type Error struct {
	msg   Message
	cause error
}

// Structured is implemented by errors whose body is a Message. Error()
// must return StructuredMessage().String().
type Structured interface {
	error
	StructuredMessage() Message
}

// JoinedError joins several errors like errors.Join, while keeping the
// structure of each child for redaction.
type JoinedError struct {
	errs []error
}

var (
	_ Structured     = (*Error)(nil)
	_ Structured     = (*JoinedError)(nil)
	_ slog.LogValuer = Message{}
)

// Const declares fixed wording. text must be a constant expression; a guard
// test enforces this because the type system cannot.
func Const(text string) Part {
	return rolePart(RoleConstant, text)
}

// Ident declares a name defined in the configuration. A guard test limits the
// positions allowed to call it.
func Ident(name string) Part {
	return rolePart(RoleIdentifier, name)
}

// Path declares a file system path. A guard test limits the positions allowed
// to call it.
func Path(path string) Part {
	return rolePart(RolePath, path)
}

// Text declares free text.
func Text(text string) Part {
	return rolePart(RoleText, text)
}

// Cause declares a wrapped error. When flattened, a cause that implements
// Structured contributes its own segments; any other cause contributes its
// Error() as one RoleText segment, and a nil cause renders as "<nil>".
func Cause(err error) Part {
	return causePart(causePlain, err)
}

// PathErrorCause is Cause, except that a cause whose dynamic type is
// *fs.PathError is split so its path becomes a RolePath segment. A guard test
// limits the positions allowed to call it.
func PathErrorCause(err error) Part {
	return causePart(causePathError, err)
}

// IndentedCause is Cause with the formatting GroupError applies to its cause:
// trailing CR and LF bytes are removed and every remaining newline is followed
// by a fixed indentation. Roles are kept.
func IndentedCause(err error) Part {
	return causePart(causeIndented, err)
}

// Quoted declares parts rendered the way fmt's %q renders their concatenation.
// Contract: the flattened text always equals strconv.Quote of the parts'
// unquoted text. The quotation marks become RoleConstant segments and every
// inner segment keeps its role with its text escaped by strconv.Quote's rules;
// when escaping segment by segment would differ from escaping the whole (a
// UTF-8 character split across segments), the whole quoted text becomes one
// RoleText segment. A cause cannot be quoted: it would lose its role and stay
// unreachable through Unwrap, so a cause part panics.
func Quoted(parts ...Part) Part {
	for _, p := range parts {
		if p.kind == partCause {
			panic("errmsg.Quoted: a cause part cannot be quoted")
		}
	}
	return Part{kind: partQuoted, quoted: slices.Clone(parts)}
}

func rolePart(role Role, text string) Part {
	return Part{kind: partRole, role: role, text: text}
}

func causePart(kind causeKind, err error) Part {
	return Part{kind: partCause, cause: err, causeKind: kind}
}

// NewMessage returns a Message holding parts in order.
func NewMessage(parts ...Part) Message {
	return Message{parts: slices.Clone(parts)}
}

// Merge returns one Message holding the parts of every message in order,
// without flattening them, so the declared roles are preserved.
func Merge(messages ...Message) Message {
	parts := make([]Part, 0, len(messages))
	for _, m := range messages {
		parts = append(parts, m.parts...)
	}
	return Message{parts: parts}
}

// String renders the message without redaction.
func (m Message) String() string {
	var b strings.Builder
	for _, s := range m.Segments() {
		b.WriteString(s.Text)
	}
	return b.String()
}

// LogValue renders the message without redaction, for handlers that do not
// redact structured messages themselves.
func (m Message) LogValue() slog.Value {
	return slog.StringValue(m.String())
}

// Segments flattens the message into segments. Each call evaluates the
// Error() of every unstructured cause again; use Freeze to evaluate once.
func (m Message) Segments() Segments {
	var out Segments
	for _, p := range m.parts {
		out = p.appendSegments(out)
	}
	return out
}

// Freeze flattens the message once and returns a Message holding the
// resulting segments as parts. The returned message has no causes, so its
// String and Segments never call a cause's Error() again.
func (m Message) Freeze() Message {
	segments := m.Segments()
	parts := make([]Part, len(segments))
	for i, s := range segments {
		parts[i] = rolePart(s.Role, s.Text)
	}
	return Message{parts: parts}
}

// appendSegments appends the segments of p to out.
func (p Part) appendSegments(out Segments) Segments {
	if p.kind == partQuoted {
		return appendQuotedSegments(out, p.quoted)
	}
	if p.kind != partCause {
		// Any role value, including one outside the declared set, is carried
		// as is; redaction treats an unknown role as RoleText.
		return append(out, Segment{Role: p.role, Text: p.text})
	}
	switch p.causeKind {
	case causePathError:
		if pe, ok := p.cause.(*fs.PathError); ok {
			// Same text as (*fs.PathError).Error().
			out = append(out,
				Segment{Role: RoleText, Text: pe.Op},
				Segment{Role: RoleConstant, Text: " "},
				Segment{Role: RolePath, Text: pe.Path},
				Segment{Role: RoleConstant, Text: ": "},
			)
			return appendCauseSegments(out, pe.Err)
		}
		return appendCauseSegments(out, p.cause)
	case causeIndented:
		return append(out, indentSegments(appendCauseSegments(nil, p.cause))...)
	default:
		return appendCauseSegments(out, p.cause)
	}
}

// appendQuotedSegments appends the segments of a Quoted part (see Quoted for
// the contract).
func appendQuotedSegments(out Segments, parts []Part) Segments {
	var inner Segments
	for _, p := range parts {
		inner = p.appendSegments(inner)
	}
	var raw, escaped strings.Builder
	quoted := Segments{{Role: RoleConstant, Text: quoteMark}}
	for _, s := range inner {
		q := strconv.Quote(s.Text)
		body := q[len(quoteMark) : len(q)-len(quoteMark)]
		raw.WriteString(s.Text)
		escaped.WriteString(body)
		quoted = append(quoted, Segment{Role: s.Role, Text: body})
	}
	quoted = append(quoted, Segment{Role: RoleConstant, Text: quoteMark})
	whole := strconv.Quote(raw.String())
	if quoteMark+escaped.String()+quoteMark != whole {
		return append(out, Segment{Role: RoleText, Text: whole})
	}
	return append(out, quoted...)
}

// appendCauseSegments appends the segments of one cause. Only the cause's own
// dynamic type is checked for Structured: searching deeper in the chain would
// drop the text that intermediate wrappers add, and String() would no longer
// match Error(). There is no depth limit, the same as fmt's %v.
func appendCauseSegments(out Segments, err error) Segments {
	if err == nil {
		return append(out, Segment{Role: RoleText, Text: "<nil>"})
	}
	if s, ok := err.(Structured); ok {
		return append(out, s.StructuredMessage().Segments()...)
	}
	return append(out, Segment{Role: RoleText, Text: err.Error()})
}

// indentSegments reproduces GroupError's cause formatting on segments:
// strings.ReplaceAll(strings.TrimRight(text, "\r\n"), "\n", "\n  ") applied
// to the concatenation equals the concatenation of the result. segments must
// be owned by the caller; it is modified in place.
func indentSegments(segments Segments) Segments {
	// Trim trailing CR/LF across segment boundaries: an originally empty
	// segment is kept and skipped, a segment of only CR/LF is dropped, and the
	// first segment with other bytes is trimmed and ends the scan.
	for i := len(segments) - 1; i >= 0; i-- {
		original := segments[i].Text
		trimmed := strings.TrimRight(original, "\r\n")
		if trimmed != "" {
			segments[i].Text = trimmed
			break
		}
		if original != "" {
			segments = slices.Delete(segments, i, i+1)
		}
	}
	for i := range segments {
		segments[i].Text = strings.ReplaceAll(segments[i].Text, "\n", "\n"+continuationIndent)
	}
	return segments
}

// ConstSummary declares a summary line of fixed wording. text must be a
// constant expression; a guard test enforces this because the type system
// cannot.
func ConstSummary(text string) Summary {
	return Summary{part: rolePart(RoleConstant, text)}
}

// TextSummary declares a summary line of free text.
func TextSummary(text string) Summary {
	return Summary{part: rolePart(RoleText, text)}
}

// String returns the summary text without redaction.
func (s Summary) String() string {
	return s.part.text
}

// Part returns the summary as one part, for a caller building a Message.
func (s Summary) Part() Part {
	return s.part
}

// NewError builds an Error from parts. Exactly one part must be a cause part
// and its cause must be non-nil; anything else is a programming error at the
// construction site and panics, so it can never surface when the error is
// reported.
func NewError(parts ...Part) *Error {
	var cause error
	causes := 0
	for _, p := range parts {
		if p.kind == partCause {
			causes++
			cause = p.cause
		}
	}
	if causes != 1 {
		panic("errmsg.NewError: exactly one cause part is required")
	}
	if cause == nil {
		panic("errmsg.NewError: the cause must not be nil")
	}
	return &Error{msg: NewMessage(parts...), cause: cause}
}

// Error renders the structured message without redaction.
func (e *Error) Error() string {
	return e.StructuredMessage().String()
}

// Unwrap returns the cause so errors.Is and errors.AsType reach it the same
// way they reach the operand of fmt.Errorf's %w.
func (e *Error) Unwrap() error {
	return e.cause
}

// StructuredMessage returns the error body.
func (e *Error) StructuredMessage() Message {
	return e.msg
}

// Join has the semantics of errors.Join: nil errors are discarded, and nil is
// returned when every error is nil. The children are rendered one per line,
// the same text errors.Join produces, and each keeps its structure.
func Join(errs ...error) error {
	kept := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			kept = append(kept, err)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	return &JoinedError{errs: kept}
}

// Error renders the structured message without redaction.
func (e *JoinedError) Error() string {
	return e.StructuredMessage().String()
}

// Unwrap returns the children so errors.Is and errors.AsType reach each one.
func (e *JoinedError) Unwrap() []error {
	return e.errs
}

// StructuredMessage returns the children separated by newlines.
func (e *JoinedError) StructuredMessage() Message {
	parts := make([]Part, 0, max(0, 2*len(e.errs)-1))
	for i, err := range e.errs {
		if i > 0 {
			parts = append(parts, Const("\n"))
		}
		parts = append(parts, Cause(err))
	}
	return Message{parts: parts}
}
