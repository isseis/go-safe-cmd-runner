package redaction

import "strings"

// byteRange is a half-open byte range [start, end) of the original text.
type byteRange struct {
	start, end int
}

// redactedRanges returns the ranges of text that RedactText replaces, in the
// coordinates of text, sorted and non-overlapping.
//
// Contract: replacing each returned range with c.placeholder (inserting it at a
// zero-width range) yields exactly c.RedactText(text). Ranges are ordered by
// start, then by end, so a zero-width range precedes a ranged one at the same
// position.
//
// The rules are the ones RedactText runs - the same compiledPattern and
// ValueDetector - applied in the same order. RedactText is not written in
// terms of this function, so the order of the three stages appears in both;
// the differential test in ranges_test.go and RedactMessage's runtime check are
// what keep the two in step. The caller must hold a validated Config.
func (c *Config) redactedRanges(text string) []byteRange {
	if text == "" {
		return nil
	}
	t := newPieceTable(text, c.placeholder)

	// Same stage order as RedactText: PEM private key blocks, then the key-name
	// rules, then value-format detection.
	if c.valueDetector != nil {
		for _, s := range privateKeyBlockSteps() {
			t.apply(s.replacedSpans)
		}
	}
	for i := range c.compiled {
		t.apply(c.compiled[i].replacedSpans)
	}
	if c.valueDetector != nil {
		for _, s := range c.valueDetector.maskSteps() {
			t.apply(s.replacedSpans)
		}
	}
	return t.ranges()
}

// pieceKind tells what a piece of the current rendering stands for.
type pieceKind int

const (
	// pieceOriginal is an unchanged run of the original text.
	pieceOriginal pieceKind = iota
	// piecePlaceholder is a placeholder a stage inserted in place of its origin.
	piecePlaceholder
	// pieceOrphan is the part of a placeholder left over when a later stage
	// replaced only some of it. Its origin is already covered by the placeholder
	// that absorbed the rest, so it stands for no original bytes and is not
	// reported as a range.
	pieceOrphan
)

// piece is one run of the current rendering.
type piece struct {
	kind pieceKind
	// origin is the range of the original text the piece stands for. It is
	// zero-width for an orphan, and for a placeholder inserted at a point.
	origin byteRange
	// lit is the rendered text of a placeholder or orphan piece. An original
	// piece renders the original text over origin.
	lit string
}

// pieceTable holds the rendering after each stage as runs of the original text
// and of inserted placeholders, so a range found in the rendering can be mapped
// back to the original text. Its size grows with the number of replacements,
// not with the length of the text.
//
// Invariants: pieces are non-empty, in rendering order, and their origins are
// non-decreasing and non-overlapping.
type pieceTable struct {
	text        string
	placeholder string
	pieces      []piece
	// rendered caches the rendering of pieces.
	rendered string
}

func newPieceTable(text, placeholder string) *pieceTable {
	return &pieceTable{
		text:        text,
		placeholder: placeholder,
		pieces:      []piece{{kind: pieceOriginal, origin: byteRange{start: 0, end: len(text)}}},
		rendered:    text,
	}
}

func (t *pieceTable) size(p piece) int {
	if p.kind == pieceOriginal {
		return p.origin.end - p.origin.start
	}
	return len(p.lit)
}

// render returns the text of the current rendering.
func (t *pieceTable) render() string {
	var b strings.Builder
	b.Grow(len(t.text))
	for _, p := range t.pieces {
		if p.kind == pieceOriginal {
			b.WriteString(t.text[p.origin.start:p.origin.end])
		} else {
			b.WriteString(p.lit)
		}
	}
	return b.String()
}

// apply runs one stage: find returns the spans of the current rendering that
// the stage replaces with the placeholder, sorted and non-overlapping, a
// zero-width span meaning an insertion.
//
// A span that touches a placeholder takes over that placeholder's whole origin,
// because the stage replaces the placeholder text and RedactText never splits a
// placeholder back into the bytes it hid. Any part of that placeholder outside
// the span stays in the rendering as an orphan. An insertion strictly inside a
// placeholder is treated the same way.
func (t *pieceTable) apply(find func(string) []byteRange) {
	spans := find(t.rendered)
	if len(spans) == 0 {
		return
	}
	in := t.pieces
	out := make([]piece, 0, len(in)+2*len(spans))
	i, pos := 0, 0 // in[i] starts at rendering offset pos
	for _, sp := range spans {
		// Copy the pieces that end at or before the span starts.
		for i < len(in) && pos+t.size(in[i]) <= sp.start {
			pos += t.size(in[i])
			out = append(out, in[i])
			i++
		}

		// The span starts inside in[i]: split off the part before it.
		if pos < sp.start {
			p := in[i]
			k := sp.start - pos
			if p.kind == pieceOriginal {
				out = append(out, originalPiece(p.origin.start, p.origin.start+k))
				in[i] = originalPiece(p.origin.start+k, p.origin.end)
			} else {
				out = append(out, orphanPiece(p.lit[:k], p.origin.start))
				in[i] = piece{kind: p.kind, origin: p.origin, lit: p.lit[k:]}
			}
			pos = sp.start
			if sp.start == sp.end && p.kind != pieceOriginal {
				// An insertion inside a placeholder replaces all of it.
				out = append(out, t.placeholderPiece(p.origin))
				in[i] = orphanPiece(p.lit[k:], p.origin.end)
				continue
			}
		}

		if sp.start == sp.end {
			at := t.pointOrigin(out, in, i)
			out = append(out, t.placeholderPiece(byteRange{start: at, end: at}))
			continue
		}

		// Absorb the pieces the span covers into one placeholder.
		merged := byteRange{start: -1}
		for i < len(in) && pos < sp.end {
			p := in[i]
			n := t.size(p)
			covered := p.origin
			if pos+n > sp.end {
				// The span ends inside p: keep the part after it.
				k := sp.end - pos
				if p.kind == pieceOriginal {
					covered = byteRange{start: p.origin.start, end: p.origin.start + k}
					in[i] = originalPiece(p.origin.start+k, p.origin.end)
				} else {
					in[i] = orphanPiece(p.lit[k:], p.origin.end)
				}
				n = k
			} else {
				i++
			}
			pos += n
			if merged.start < 0 {
				merged = covered
			} else {
				merged.end = max(merged.end, covered.end)
			}
		}
		out = append(out, t.placeholderPiece(merged))
	}
	out = append(out, in[i:]...)
	t.pieces = out
	t.rendered = t.render()
}

// pointOrigin returns the original position of an insertion made at the
// boundary between the last piece of out and in[i].
func (t *pieceTable) pointOrigin(out, in []piece, i int) int {
	if i < len(in) {
		return in[i].origin.start
	}
	if len(out) > 0 {
		return out[len(out)-1].origin.end
	}
	return 0
}

// ranges returns the origins of the placeholders in the current rendering.
func (t *pieceTable) ranges() []byteRange {
	var rs []byteRange
	for _, p := range t.pieces {
		if p.kind == piecePlaceholder {
			rs = append(rs, p.origin)
		}
	}
	return rs
}

func originalPiece(start, end int) piece {
	return piece{kind: pieceOriginal, origin: byteRange{start: start, end: end}}
}

func orphanPiece(lit string, at int) piece {
	return piece{kind: pieceOrphan, origin: byteRange{start: at, end: at}, lit: lit}
}

func (t *pieceTable) placeholderPiece(origin byteRange) piece {
	return piece{kind: piecePlaceholder, origin: origin, lit: t.placeholder}
}
