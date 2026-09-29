package redaction

import (
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"

	"github.com/isseis/go-safe-cmd-runner/internal/errmsg"
)

// RedactMessage renders m with per-segment redaction and the cross-boundary
// contract, using this Config's rules and replacement string. It returns an
// error when flattening m panics or when the runtime range check fails; the
// returned string is then RedactionFailurePlaceholder, never a partial
// rendering.
//
// Contract: every byte RedactText would replace in the unredacted rendering is
// replaced, except bytes of RoleIdentifier segments, and a RoleText segment
// that RedactText leaves unchanged but that names a sensitive word is replaced
// whole, judged on that segment alone.
func (c *Config) RedactMessage(m errmsg.Message) (string, error) {
	if !c.validated {
		// Same suppression as RedactText: a Config that skipped NewConfig has no
		// rules and would pass the text through.
		return RedactionFailurePlaceholder, nil
	}
	segs, err := flattenMessage(m)
	if err != nil {
		return RedactionFailurePlaceholder, err
	}
	return c.redactSegments(segs)
}

// flattenMessage calls m.Segments exactly once, so a cause's Error() is not
// evaluated twice, and turns a panic in it into an error that carries only the
// panic value's type.
func flattenMessage(m errmsg.Message) (segs errmsg.Segments, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			segs = nil
			err = &ErrMessageFlattenPanic{PanicType: fmt.Sprintf("%T", rec)}
		}
	}()
	return m.Segments(), nil
}

// redactSegments renders an already flattened segment list. Each segment is
// first redacted on its own according to its role. A detection of RedactText
// over the whole rendering that the segment's own redaction does not already
// cover - one crossing a segment boundary - then hides the bytes it spans in
// every segment it touches except RoleIdentifier ones, one placeholder per
// maximal hidden run.
func (c *Config) redactSegments(segs errmsg.Segments) (string, error) {
	l := newSegmentLayout(segs)

	// Each segment on its own.
	for i, s := range segs {
		out, spans, err := c.redactSegment(s)
		if err != nil {
			return RedactionFailurePlaceholder, err
		}
		l.outs[i] = out
		l.own[i] = shiftRanges(spans, l.starts[i])
	}

	// The whole rendering, checked against RedactText before any of it is used.
	ranges := c.redactedRanges(l.full)
	if replaceSpans(l.full, ranges, c.placeholder) != c.RedactText(l.full) {
		return RedactionFailurePlaceholder, &ErrMessageRangeMismatch{RangeCount: len(ranges)}
	}

	l.hideCrossBoundary(ranges)
	return l.render(c.placeholder), nil
}

// segmentLayout is a flattened message laid out as one string, with the
// per-segment redaction results and the bytes to hide across boundaries.
type segmentLayout struct {
	segs errmsg.Segments
	// full is the unredacted rendering; starts[i] is where segment i begins in
	// it, and starts[len(segs)] is len(full).
	full   string
	starts []int
	// outs[i] is segment i redacted on its own; own[i] is what that redaction
	// replaced, in full's coordinates.
	outs []string
	own  [][]byteRange
	// affected[i] reports that segment i is rendered from its bytes outside
	// hidden rather than from outs[i].
	affected []bool
	// hidden holds the maximal runs replaced by one placeholder each, sorted.
	hidden []byteRange
}

func newSegmentLayout(segs errmsg.Segments) *segmentLayout {
	n := len(segs)
	starts := make([]int, n+1)
	var b strings.Builder
	for i, s := range segs {
		starts[i] = b.Len()
		b.WriteString(s.Text)
	}
	starts[n] = b.Len()
	return &segmentLayout{
		segs:     segs,
		full:     b.String(),
		starts:   starts,
		outs:     make([]string, n),
		own:      make([][]byteRange, n),
		affected: make([]bool, n),
	}
}

// hideCrossBoundary marks the segments that the whole-rendering ranges affect
// beyond their own redaction, and collects the bytes to hide in them. ranges
// must be sorted and non-overlapping.
func (l *segmentLayout) hideCrossBoundary(ranges []byteRange) {
	n := len(l.segs)
	// touched[i] holds the parts of ranges that fall to segment i; boundary
	// holds insertions between two RoleIdentifier segments, which no segment
	// can take.
	touched := make([][]byteRange, n)
	var boundary []byteRange
	k := 0 // first segment that can overlap the current range
	for _, r := range ranges {
		if r.start == r.end {
			i, ok := attachInsertion(l.segs, l.starts, r.start)
			if !ok {
				boundary = append(boundary, r)
				continue
			}
			if l.segs[i].Role == errmsg.RoleIdentifier {
				// Strictly inside an identifier: its bytes are rendered as
				// they are, and an insertion hides none of them.
				continue
			}
			touched[i] = append(touched[i], r)
			if !coversPoint(l.own[i], r.start) {
				l.affected[i] = true
			}
			continue
		}
		for k < n && l.starts[k+1] <= r.start {
			k++
		}
		for j := k; j < n && l.starts[j] < r.end; j++ {
			lo, hi := max(r.start, l.starts[j]), min(r.end, l.starts[j+1])
			if lo >= hi || l.segs[j].Role == errmsg.RoleIdentifier {
				continue
			}
			touched[j] = append(touched[j], byteRange{start: lo, end: hi})
			if !coversRange(l.own[j], r) {
				l.affected[j] = true
			}
		}
	}

	hidden := boundary
	for i := range n {
		if l.affected[i] {
			hidden = append(hidden, touched[i]...)
			hidden = append(hidden, l.own[i]...)
		}
	}
	l.hidden = mergeRanges(hidden)
}

// render walks the segments in position order. An unaffected segment
// contributes its own redaction; an affected one contributes its bytes outside
// hidden, and each hidden run yields one placeholder where it starts.
func (l *segmentLayout) render(placeholder string) string {
	var b strings.Builder
	b.Grow(len(l.full))
	mi, pos := 0, 0 // next hidden run; end of the hidden bytes emitted so far
	emitUpTo := func(q int) {
		for mi < len(l.hidden) && l.hidden[mi].start <= q {
			b.WriteString(placeholder)
			pos = max(pos, l.hidden[mi].end)
			mi++
		}
	}
	for i := range l.segs {
		st, en := l.starts[i], l.starts[i+1]
		if !l.affected[i] {
			emitUpTo(st)
			b.WriteString(l.outs[i])
			continue
		}
		q := max(st, pos)
		for {
			emitUpTo(q)
			q = max(q, pos)
			if q >= en {
				break
			}
			next := en
			if mi < len(l.hidden) && l.hidden[mi].start < en {
				next = l.hidden[mi].start
			}
			b.WriteString(l.full[q:next])
			q = next
		}
	}
	emitUpTo(len(l.full))
	return b.String()
}

// redactSegment redacts one segment on its own according to its role, and
// returns the result with the spans of s.Text it replaced. Any role outside the
// declared set is treated as RoleText, the role that assumes least.
func (c *Config) redactSegment(s errmsg.Segment) (string, []byteRange, error) {
	switch s.Role {
	case errmsg.RoleIdentifier, errmsg.RoleConstant:
		return s.Text, nil, nil
	case errmsg.RolePath:
		return c.redactText(s.Text)
	default:
		out, spans, err := c.redactText(s.Text)
		if err != nil || out != s.Text {
			return out, spans, err
		}
		// Whole-value replacement is judged per segment, so a sensitive word in
		// another segment of the same message does not reach this one.
		if c.patterns.IsSensitiveValue(s.Text) {
			return c.placeholder, []byteRange{{start: 0, end: len(s.Text)}}, nil
		}
		return out, nil, nil
	}
}

// redactText returns c.RedactText(text) with the spans it replaced, after
// checking that those spans reproduce it.
func (c *Config) redactText(text string) (string, []byteRange, error) {
	out := c.RedactText(text)
	if out == text {
		return text, nil, nil
	}
	spans := c.redactedRanges(text)
	if replaceSpans(text, spans, c.placeholder) != out {
		return "", nil, &ErrMessageRangeMismatch{RangeCount: len(spans)}
	}
	return out, spans, nil
}

// attachInsertion returns the segment an insertion at p belongs to: the
// segment p is strictly inside; otherwise, at a boundary, the segment starting
// at p (the one ending at p when p is the end), unless it is a RoleIdentifier
// segment, in which case the other neighbour, unless that is one too. ok is
// false when both neighbours are RoleIdentifier segments.
func attachInsertion(segs errmsg.Segments, starts []int, p int) (int, bool) {
	n := len(segs)
	next := sort.Search(n, func(i int) bool { return starts[i] >= p })
	if next > 0 && starts[next-1] < p && p < starts[next] {
		return next - 1, true
	}
	if next < n && segs[next].Role != errmsg.RoleIdentifier {
		return next, true
	}
	if prev := next - 1; prev >= 0 && segs[prev].Role != errmsg.RoleIdentifier {
		return prev, true
	}
	return 0, false
}

// coversPoint reports whether an insertion at p is already produced by spans:
// an insertion at the same position, or a replaced span strictly around it.
func coversPoint(spans []byteRange, p int) bool {
	for _, s := range spans {
		if (s.start == p && s.end == p) || (s.start < p && p < s.end) {
			return true
		}
	}
	return false
}

// coversRange reports whether every byte of r lies in spans, which must be
// sorted and non-overlapping.
func coversRange(spans []byteRange, r byteRange) bool {
	pos := r.start
	for _, s := range spans {
		if s.start > pos {
			break
		}
		pos = max(pos, s.end)
		if pos >= r.end {
			return true
		}
	}
	return false
}

// shiftRanges returns spans moved by off.
func shiftRanges(spans []byteRange, off int) []byteRange {
	if len(spans) == 0 {
		return nil
	}
	out := make([]byteRange, len(spans))
	for i, s := range spans {
		out[i] = byteRange{start: s.start + off, end: s.end + off}
	}
	return out
}

// mergeRanges sorts rs and joins ranges that overlap or touch, so an insertion
// next to a hidden run, several insertions at one position, and adjacent runs
// each become one range. rs is modified.
func mergeRanges(rs []byteRange) []byteRange {
	if len(rs) == 0 {
		return nil
	}
	slices.SortFunc(rs, func(a, b byteRange) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return a.end - b.end
	})
	out := rs[:1]
	for _, r := range rs[1:] {
		last := &out[len(out)-1]
		if r.start <= last.end {
			last.end = max(last.end, r.end)
			continue
		}
		out = append(out, r)
	}
	return out
}

// redactMessageAttribute returns the attribute to use when value carries
// exactly an errmsg.Message; handled is false when it does not. Only the
// dynamic type decides - neither the key nor the text is inspected. On failure
// the attribute carries RedactionFailurePlaceholder and collector, when
// non-nil, records the error.
func (c *Config) redactMessageAttribute(key string, value slog.Value, collector ErrorCollector) (attr slog.Attr, handled bool) {
	m, ok := value.Any().(errmsg.Message)
	if !ok {
		return slog.Attr{}, false
	}
	text, err := c.RedactMessage(m)
	if err != nil {
		if collector != nil {
			collector.RecordFailure(key, err)
		}
		return slog.String(key, RedactionFailurePlaceholder), true
	}
	return slog.String(key, text), true
}
