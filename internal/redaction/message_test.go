package redaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
)

// segmentsAlone redacts each segment with RedactText and whole-value
// replacement on its own, ignoring roles and the other segments. It is the
// control the per-segment tests compare against: an input is only a useful
// cross-boundary case if this still leaves the secret visible.
func segmentsAlone(c *Config, segs errmsg.Segments) string {
	var b strings.Builder
	for _, s := range segs {
		r := c.RedactText(s.Text)
		if r == s.Text && c.patterns.IsSensitiveValue(s.Text) {
			r = c.placeholder
		}
		b.WriteString(r)
	}
	return b.String()
}

// wholeAsText redacts the whole rendering of m the way a plain string attribute
// is redacted. It is the control showing what the roles save from redaction.
func wholeAsText(c *Config, m errmsg.Message) string {
	s := m.String()
	r := c.RedactText(s)
	if r == s && c.patterns.IsSensitiveValue(s) {
		return c.placeholder
	}
	return r
}

func mustRedactMessage(t *testing.T, c *Config, m errmsg.Message) string {
	t.Helper()
	got, err := c.RedactMessage(m)
	require.NoError(t, err)
	return got
}

func TestRedactMessage_IdentifierSegmentsAreExempt(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name  string
		ident string
	}{
		{name: "sensitive word", ident: "api_key"},
		{name: "sensitive word inside a name", ident: "monkey-test"},
		{name: "key=value form", ident: "backup --password=x"},
		{name: "value format", ident: "AKIAIOSFODNN7EXAMPLE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident(tt.ident), errmsg.Const(" failed"))
			// The same text with no roles is redacted, so the exemption is what
			// keeps it.
			require.NotEqual(t, m.String(), wholeAsText(cfg, m))

			assert.Equal(t, m.String(), mustRedactMessage(t, cfg, m))
		})
	}
}

func TestRedactMessage_PathSegmentsFollowRedactTextOnly(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("a sensitive word alone does not rewrite a path", func(t *testing.T) {
		const path = "/etc/monkey/key.pem"
		// Whole-value replacement alone would react to this text.
		require.Equal(t, path, cfg.RedactText(path))
		require.True(t, cfg.patterns.IsSensitiveValue(path))

		m := errmsg.NewMessage(errmsg.Const("open "), errmsg.Path(path), errmsg.Const(": permission denied"))
		assert.Equal(t, "open /etc/monkey/key.pem: permission denied", mustRedactMessage(t, cfg, m))
	})

	t.Run("a value RedactText reacts to is masked", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Const("open "), errmsg.Path("/tmp/token=abc123/out"), errmsg.Const(" (no such file)"))
		assert.Equal(t, "open /tmp/token=[REDACTED] (no such file)", mustRedactMessage(t, cfg, m))
	})
}

func TestRedactMessage_TextSegmentsFollowRedactTextAndWholeValue(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("whole-value replacement covers only its own segment", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident("backup"), errmsg.Const(": "), errmsg.Text("config for monkey"))
		assert.Equal(t, "group backup: [REDACTED]", mustRedactMessage(t, cfg, m))
	})

	t.Run("RedactText result is kept within the segment", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident("backup"), errmsg.Const(": "), errmsg.Text("open x: token=abc123"))
		assert.Equal(t, "group backup: open x: token=[REDACTED]", mustRedactMessage(t, cfg, m))
	})
}

func TestRedactMessage_WholeValueIsPerSegment(t *testing.T) {
	cfg := DefaultConfig()
	m := errmsg.NewMessage(errmsg.Const("command "), errmsg.Ident("rotate_api_key"), errmsg.Const(" failed: "), errmsg.Text("exit status 1"))
	// As one string the sensitive word in the name would replace everything.
	require.Equal(t, DefaultPlaceholder, wholeAsText(cfg, m))

	assert.Equal(t, "command rotate_api_key failed: exit status 1", mustRedactMessage(t, cfg, m))
}

func TestRedactMessage_CrossBoundaryDetection(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name   string
		parts  []errmsg.Part
		secret string
		want   string
	}{
		{
			name:   "key=value with the key in an identifier",
			parts:  []errmsg.Part{errmsg.Ident("API_KEY"), errmsg.Const("="), errmsg.Text("s3cretvalue")},
			secret: "s3cretvalue",
			want:   "API_KEY=[REDACTED]",
		},
		{
			name:   "key=value with the key in a constant",
			parts:  []errmsg.Part{errmsg.Const("password="), errmsg.Text("hunter2")},
			secret: "hunter2",
			want:   "password=[REDACTED]",
		},
		{
			name:   "token after Bearer",
			parts:  []errmsg.Part{errmsg.Const("Bearer "), errmsg.Text("opaque-credential")},
			secret: "opaque-credential",
			want:   "Bearer [REDACTED]",
		},
		{
			name:   "token after Basic",
			parts:  []errmsg.Part{errmsg.Const("Basic "), errmsg.Text("dXNlcjpwYXNz")},
			secret: "dXNlcjpwYXNz",
			want:   "Basic [REDACTED]",
		},
		{
			name:   "Authorization header value",
			parts:  []errmsg.Part{errmsg.Const("Authorization: "), errmsg.Text("opaque-credential")},
			secret: "opaque-credential",
			want:   "Authorization: [REDACTED]",
		},
		{
			name:   "value format split between an identifier and text",
			parts:  []errmsg.Part{errmsg.Ident("AKIAIOSFODNN7"), errmsg.Text("EXAMPLE")},
			secret: "EXAMPLE",
			want:   "AKIAIOSFODNN7[REDACTED]",
		},
		{
			name:   "value format with an identifier in the middle",
			parts:  []errmsg.Part{errmsg.Text("AKIA"), errmsg.Ident("IOSFODNN7"), errmsg.Text("EXAMPLE")},
			secret: "EXAMPLE",
			want:   "[REDACTED]IOSFODNN7[REDACTED]",
		},
		{
			// The first segment is replaced whole on its own; the crossing
			// detection covers only its tail, and must not bring back the rest.
			name:   "segment replaced whole on its own and crossed by a detection",
			parts:  []errmsg.Part{errmsg.Text("monkey AKIA"), errmsg.Text("IOSFODNN7EXAMPLE")},
			secret: "IOSFODNN7EXAMPLE",
			want:   "[REDACTED]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := errmsg.NewMessage(tt.parts...)
			// Only the whole rendering reveals the secret: redacting each
			// segment on its own leaves it visible.
			require.Contains(t, segmentsAlone(cfg, m.Segments()), tt.secret)
			require.NotContains(t, cfg.RedactText(m.String()), tt.secret)

			assert.Equal(t, tt.want, mustRedactMessage(t, cfg, m))
		})
	}

	t.Run("a detection spanning only identifiers replaces nothing", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Ident("AKIAIOSF"), errmsg.Ident("ODNN7EXAMPLE"))
		require.Equal(t, DefaultPlaceholder, cfg.RedactText(m.String()))

		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", mustRedactMessage(t, cfg, m))
	})

	t.Run("a value extending into the next segment becomes one placeholder", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Text("password=secret"), errmsg.Text("suffix"))
		// Each segment alone gives two outputs with the tail left visible.
		require.Equal(t, "password=[REDACTED]suffix", segmentsAlone(cfg, m.Segments()))

		assert.Equal(t, "password=[REDACTED]", mustRedactMessage(t, cfg, m))
	})
}

// TestRedactMessage_BoundaryRules pins the parts of the cross-boundary rules
// the detection-kind table does not reach: constants are not exempt from a
// crossing detection, and insertions follow the attachment rule.
func TestRedactMessage_BoundaryRules(t *testing.T) {
	cfg := DefaultConfig()
	tests := []struct {
		name  string
		parts []errmsg.Part
		want  string
	}{
		{
			name: "a detection keyed by an identifier replaces the next constant word",
			parts: []errmsg.Part{
				errmsg.Const("failed to execute group "), errmsg.Ident("password"), errmsg.Const(": "),
				errmsg.Const("command "), errmsg.Ident("backup"), errmsg.Const(" failed"),
			},
			want: "failed to execute group password: [REDACTED] backup failed",
		},
		{
			name:  "an insertion strictly inside an identifier is dropped",
			parts: []errmsg.Part{errmsg.Ident(`password=""`)},
			want:  `password=""`,
		},
		{
			name:  "an insertion strictly inside a constant stays in it",
			parts: []errmsg.Part{errmsg.Const(`password=""`), errmsg.Text("x")},
			want:  `password="[REDACTED]"x`,
		},
		{
			name:  "an insertion before an identifier goes to the previous segment",
			parts: []errmsg.Part{errmsg.Const(`password="`), errmsg.Ident(`"`)},
			want:  `password="[REDACTED]"`,
		},
		{
			// The text alone would also get a placeholder between its empty
			// quotes; the whole rendering does not, and neither does the result.
			name:  "a segment's own insertion is not added when it is affected",
			parts: []errmsg.Part{errmsg.Const("password="), errmsg.Text(`"password":""`)},
			want:  `password="[REDACTED]":""`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, mustRedactMessage(t, cfg, errmsg.NewMessage(tt.parts...)))
		})
	}
}

func TestRedactMessage_InsertionAtEmptySegment(t *testing.T) {
	cfg := DefaultConfig()
	const whole = `password=""`
	require.Equal(t, `password="[REDACTED]"`, cfg.RedactText(whole))

	t.Run("empty text between constants", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Const(`password="`), errmsg.Text(""), errmsg.Const(`"`))
		assert.Equal(t, cfg.RedactText(whole), mustRedactMessage(t, cfg, m))
	})

	t.Run("boundary between two identifiers", func(t *testing.T) {
		m := errmsg.NewMessage(errmsg.Ident(`password="`), errmsg.Ident(`"`))
		assert.Equal(t, cfg.RedactText(whole), mustRedactMessage(t, cfg, m))
	})
}

func TestRedactMessage_UnstructuredCauseWholeValueReplaced(t *testing.T) {
	cfg := DefaultConfig()
	m := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident("backup"), errmsg.Const(": "), errmsg.Cause(errors.New("monkey business")))

	assert.Equal(t, "group backup: [REDACTED]", mustRedactMessage(t, cfg, m))
}

func TestRedactMessage_OutOfScopeBodyMatchesRedactText(t *testing.T) {
	cfg := DefaultConfig()
	const summary = "failed to read key file"
	cause := errors.New("open x: token=abc123")
	// The summary would be replaced whole if it were free text.
	require.True(t, cfg.patterns.IsSensitiveValue(summary))

	m := errmsg.NewMessage(errmsg.ConstSummary(summary).Part(), errmsg.Const(": "), errmsg.Cause(cause))
	assert.Equal(t, summary+": "+cfg.RedactText(cause.Error()), mustRedactMessage(t, cfg, m))
}

func TestRedactMessage_TextSummaryWholeValueReplaced(t *testing.T) {
	cfg := DefaultConfig()
	const summary = "group monkey failed"
	require.Equal(t, summary, cfg.RedactText(summary))

	m := errmsg.NewMessage(errmsg.TextSummary(summary).Part())
	assert.Equal(t, DefaultPlaceholder, mustRedactMessage(t, cfg, m))
}

func TestRedactSegments_ZeroRoleFallsBackToText(t *testing.T) {
	cfg := DefaultConfig()
	got, err := cfg.redactSegments(errmsg.Segments{{Text: "monkey"}})
	require.NoError(t, err)
	assert.Equal(t, DefaultPlaceholder, got)
}

func TestRedactSegments_UnknownRoleFallsBackToText(t *testing.T) {
	cfg := DefaultConfig()
	const text = "monkey"
	// Only whole-value replacement reacts to this text.
	require.Equal(t, text, cfg.RedactText(text))

	got, err := cfg.redactSegments(errmsg.Segments{{Role: errmsg.Role(99), Text: text}})
	require.NoError(t, err)
	assert.Equal(t, DefaultPlaceholder, got)
}

func TestRedactMessage_UnvalidatedConfigSuppresses(t *testing.T) {
	var zero Config
	got, err := zero.RedactMessage(errmsg.NewMessage(errmsg.Const("harmless")))
	require.NoError(t, err)
	assert.Equal(t, RedactionFailurePlaceholder, got)
}

// placeholderMismatchConfig returns a Config whose ranges cannot reproduce
// RedactText: RedactText keeps the placeholder compiled into its rules while
// the ranges are rendered with the changed field.
func placeholderMismatchConfig(t *testing.T) *Config {
	t.Helper()
	cfg, err := NewConfig()
	require.NoError(t, err)
	cfg.placeholder = "<masked>"
	return cfg
}

func TestRedactMessage_RangeMismatchReportsFailure(t *testing.T) {
	cfg := placeholderMismatchConfig(t)
	// The constant is kept and the text does not change on its own, so the
	// whole-rendering check is the one that fails.
	m := errmsg.NewMessage(errmsg.Const("password="), errmsg.Text("hunter2"))
	require.Equal(t, "hunter2", cfg.RedactText("hunter2"))

	got, err := cfg.RedactMessage(m)

	mismatch, ok := errors.AsType[*ErrMessageRangeMismatch](err)
	require.True(t, ok, "want *ErrMessageRangeMismatch, got %v", err)
	assert.Equal(t, 1, mismatch.RangeCount)
	assert.Equal(t, RedactionFailurePlaceholder, got)
}

// The ranges of a segment's own redaction decide which bytes it hides when a
// detection crosses its boundary, so they are checked like the whole
// rendering's. Through RedactMessage the whole-rendering check fails on the same
// input as well, so the segment check is exercised directly.
func TestRedactSegmentText_RangeMismatchReportsFailure(t *testing.T) {
	cfg := placeholderMismatchConfig(t)

	_, _, err := cfg.redactText("password=hunter2")

	_, ok := errors.AsType[*ErrMessageRangeMismatch](err)
	assert.True(t, ok, "want *ErrMessageRangeMismatch, got %v", err)
}

// panickingStructured is an error whose StructuredMessage panics with a value
// built from secret, as a cause's formatting code might.
type panickingStructured struct {
	secret string
}

func (p panickingStructured) Error() string { return "unused" }

func (p panickingStructured) StructuredMessage() errmsg.Message {
	panic(fmt.Errorf("formatting failed for %s", p.secret))
}

func TestRedactMessage_FlattenPanicReportsFailure(t *testing.T) {
	cfg := DefaultConfig()
	m := errmsg.NewMessage(errmsg.Const("failed: "), errmsg.Cause(panickingStructured{secret: "hunter2"}))

	got, err := cfg.RedactMessage(m)

	_, ok := errors.AsType[*ErrMessageFlattenPanic](err)
	require.True(t, ok, "want *ErrMessageFlattenPanic, got %v", err)
	assert.Equal(t, RedactionFailurePlaceholder, got)
}

// handleMessage logs attr through a RedactingHandler with the given Config and
// collector and returns the attribute the next handler received.
func handleMessage(t *testing.T, cfg *Config, collector ErrorCollector, attr slog.Attr) slog.Attr {
	t.Helper()
	mock := newMockHandler()
	handler := NewRedactingHandler(mock, cfg, nil)
	if collector != nil {
		handler = handler.WithErrorCollector(collector)
	}
	slog.New(handler).LogAttrs(context.Background(), slog.LevelInfo, "test message", attr)

	require.Len(t, mock.records, 1)
	var got []slog.Attr
	mock.records[0].Attrs(func(a slog.Attr) bool {
		got = append(got, a)
		return true
	})
	require.Len(t, got, 1)
	return got[0]
}

func TestRedactingHandler_MessageAttributeBecomesString(t *testing.T) {
	m := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident("monkey"), errmsg.Const(": "), errmsg.Text("exit status 1"))
	// Redacted as one string, the name would take the whole value with it.
	require.Equal(t, DefaultPlaceholder, wholeAsText(DefaultConfig(), m))

	got := handleMessage(t, DefaultConfig(), nil, slog.Any("error_message", m))

	assert.Equal(t, slog.KindString, got.Value.Kind())
	assert.Equal(t, "group monkey: exit status 1", got.Value.String())
}

func TestRedactingHandler_SensitiveKeyReplacesMessage(t *testing.T) {
	m := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident("backup"), errmsg.Const(" failed"))
	// Under an ordinary key nothing in this message is redacted.
	require.Equal(t, m.String(), handleMessage(t, DefaultConfig(), nil, slog.Any("error_message", m)).Value.String())

	got := handleMessage(t, DefaultConfig(), nil, slog.Any("password", m))
	assert.Equal(t, DefaultPlaceholder, got.Value.String())
}

func TestRedactingHandler_PointerMessageIsRedactedWhole(t *testing.T) {
	m := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident("monkey"), errmsg.Const(" failed"))
	// The value itself takes the per-segment path and keeps the name.
	require.Equal(t, m.String(), handleMessage(t, DefaultConfig(), nil, slog.Any("error_message", m)).Value.String())

	got := handleMessage(t, DefaultConfig(), nil, slog.Any("error_message", &m))
	assert.Equal(t, DefaultPlaceholder, got.Value.String())
}

func TestRedactingHandler_MessageFailureIsRecorded(t *testing.T) {
	t.Run("flatten panic", func(t *testing.T) {
		collector := NewInMemoryErrorCollector(0)
		m := errmsg.NewMessage(errmsg.Cause(panickingStructured{secret: "hunter2"}))

		got := handleMessage(t, DefaultConfig(), collector, slog.Any("error_message", m))

		assert.Equal(t, RedactionFailurePlaceholder, got.Value.String())
		failures := collector.GetFailures()
		require.Len(t, failures, 1)
		assert.Equal(t, "error_message", failures[0].Key)
		_, ok := errors.AsType[*ErrMessageFlattenPanic](failures[0].Err)
		assert.True(t, ok, "want *ErrMessageFlattenPanic, got %v", failures[0].Err)
	})

	t.Run("range mismatch", func(t *testing.T) {
		collector := NewInMemoryErrorCollector(0)
		m := errmsg.NewMessage(errmsg.Const("password="), errmsg.Text("hunter2"))

		got := handleMessage(t, placeholderMismatchConfig(t), collector, slog.Any("error_message", m))

		assert.Equal(t, RedactionFailurePlaceholder, got.Value.String())
		failures := collector.GetFailures()
		require.Len(t, failures, 1)
		_, ok := errors.AsType[*ErrMessageRangeMismatch](failures[0].Err)
		assert.True(t, ok, "want *ErrMessageRangeMismatch, got %v", failures[0].Err)
	})
}

func TestRedactingHandler_FlattenPanicDoesNotLeakPanicValueToShutdownReport(t *testing.T) {
	const secret = "hunter2-in-panic-value"
	collector := NewInMemoryErrorCollector(0)
	m := errmsg.NewMessage(errmsg.Cause(panickingStructured{secret: secret}))

	handleMessage(t, DefaultConfig(), collector, slog.Any("error_message", m))

	failures := collector.GetFailures()
	require.Len(t, failures, 1)
	assert.NotContains(t, failures[0].Err.Error(), secret)

	var buf bytes.Buffer
	require.NoError(t, NewShutdownReporter(collector, &buf, nil).Report())
	// The report does describe the failure; it only leaves the value out.
	assert.Contains(t, buf.String(), "Attribute: error_message")
	assert.Contains(t, buf.String(), failures[0].Err.Error())
	assert.NotContains(t, buf.String(), secret)
}

func TestRedactLogAttribute_StructuredMessage(t *testing.T) {
	cfg := DefaultConfig()
	m := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident("monkey"), errmsg.Const(": "), errmsg.Text("open x: token=abc123"))

	t.Run("message is rendered with per-segment redaction", func(t *testing.T) {
		got := cfg.RedactLogAttribute(slog.Any("error_message", m))
		assert.Equal(t, slog.KindString, got.Value.Kind())
		// Neither the unredacted text nor the whole-value placeholder.
		assert.Equal(t, "group monkey: open x: token=[REDACTED]", got.Value.String())
	})

	t.Run("sensitive key replaces the message", func(t *testing.T) {
		plain := errmsg.NewMessage(errmsg.Const("group "), errmsg.Ident("backup"), errmsg.Const(" failed"))
		require.Equal(t, plain.String(), cfg.RedactLogAttribute(slog.Any("error_message", plain)).Value.String())

		got := cfg.RedactLogAttribute(slog.Any("password", plain))
		assert.Equal(t, DefaultPlaceholder, got.Value.String())
	})

	t.Run("failure suppresses the value", func(t *testing.T) {
		panicking := errmsg.NewMessage(errmsg.Cause(panickingStructured{secret: "hunter2"}))
		got := cfg.RedactLogAttribute(slog.Any("error_message", panicking))
		assert.Equal(t, RedactionFailurePlaceholder, got.Value.String())
	})

	t.Run("other log valuers are suppressed", func(t *testing.T) {
		for name, v := range map[string]any{
			"pointer to message": &m,
			"other log valuer":   sensitiveLogValuer{data: "password=secret123"},
			"panicking valuer":   panickingLogValuer{},
		} {
			t.Run(name, func(t *testing.T) {
				got := cfg.RedactLogAttribute(slog.Any("detail", v))
				assert.Equal(t, RedactionFailurePlaceholder, got.Value.String())
			})
		}
	})
}

// BenchmarkRedactMessage renders the final execution error of a run in which
// 100 groups failed: 100 structured errors joined, tens of KiB in total. The
// budget is 200 ms per rendering.
func BenchmarkRedactMessage(b *testing.B) {
	cfg := DefaultConfig()
	errs := make([]error, 0, 100)
	for i := range 100 {
		cause := fmt.Errorf("command %q exited with status 1: stderr: %s token=abc%03d",
			fmt.Sprintf("step_%03d", i), strings.Repeat("output line of the failed command; ", 8), i)
		errs = append(errs, errmsg.NewError(
			errmsg.Const("group "), errmsg.Ident(fmt.Sprintf("group_%03d", i)),
			errmsg.Const(" failed in "), errmsg.Path(fmt.Sprintf("/var/lib/runner/work/group_%03d", i)),
			errmsg.Const(": "), errmsg.Cause(cause),
		))
	}
	m := errmsg.NewMessage(errmsg.Const("error running commands: "), errmsg.Cause(errmsg.Join(errs...)))
	b.SetBytes(int64(len(m.String())))

	b.ResetTimer()
	for range b.N {
		if _, err := cfg.RedactMessage(m); err != nil {
			b.Fatal(err)
		}
	}
}
