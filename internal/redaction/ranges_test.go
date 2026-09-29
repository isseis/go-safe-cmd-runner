package redaction

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testPEMBlock is a complete private key block, which pemPrivate masks whole.
const testPEMBlock = "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA\n-----END RSA PRIVATE KEY-----"

// rangeSeeds are the inputs the differential test adds to the ones collected
// from the existing RedactText and Mask tests. Each targets a part of the
// range derivation those tests were not written to reach.
var rangeSeeds = []string{
	// Kept prefixes and suffixes.
	"Authorization: Bearer abc123",
	"Authorization: basic dXNlcjpwYXNz",
	"Authorization:",
	"Authorization: \nnext line",
	"Bearer abc.def-ghi",
	// The key-name "Bearer " rule needs exactly one space, so these reach the
	// value-format bearerToken step instead.
	"Bearer\tabc.def-ghi",
	"auth Bearer  abc123==",
	"password: hunter2",
	`{"private_key_id": "0123456789abcdef0123456789abcdef", "x": 1}`,
	"token eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJl rest",
	"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.c2lnbmF0dXJl",
	"fetch https://user:pass@example.com/path failed",
	// Value formats whose existing tests build their inputs at run time, out of
	// reach of the literal collection below.
	"using github_pat_11ABCDEFG0123456789_abcdefghijklmnopqrstuvwxyz now",
	"app token xapp-1-A0123456789-0123456789012-abcdef0123456789 here",
	"refresh xoxe-1-abcdefghijk done",
	// A later stage overlapping an earlier stage's placeholder.
	"password=" + testPEMBlock,
	"password=x" + testPEMBlock + " tail",
	`password="` + testPEMBlock + `"`,
	"Bearer AKIAIOSFODNN7EXAMPLE",
	"Authorization: Bearer AKIAIOSFODNN7EXAMPLE",
	"api_key=ghp_abcdefghijklmnopqrstuvwxyzABCDEFGHIJ",
	// A kept part overlapping an earlier stage's placeholder.
	testPEMBlock + "password=x",
	testPEMBlock + "\npassword=x",
	// Empty quoted values, which RedactText fills with a placeholder.
	`password=""`,
	`"password":""`,
	`password=''`,
	`a password="" b token='' c`,
	// The placeholder taking no part in a later match.
	testPEMBlock + "ABCDEFGHIJKLMNOP",
	testPEMBlock + "AKIAIOSFODNN7EXAMPLE",
	testPEMBlock + "Bearer abc",
	"AKIA" + testPEMBlock,
	// A BEGIN line with no END line runs to the end of the text.
	"key: -----BEGIN PRIVATE KEY-----\nMIIE\nrest of output",
	// The configured webhook host next to a placeholder.
	"https://hooks.example.com/[REDACTED]",
	"[REDACTED]https://hooks.example.com/x",
	"https://hooks.example.com/" + testPEMBlock,
	"post to https://hooks.slack.com/services/T000/B000/XXXX failed",
	"post to https://mattermost.example.com/hooks/abcdef.",
	"post to https://[2001:db8::1]:8443/hooks/abcdef",
}

// rangeTestConfigs returns the configurations the differential test runs every
// input against: the default rules, and each webhook host the existing tests
// configure plus the one rangeSeeds uses.
func rangeTestConfigs(tb testing.TB) []*Config {
	tb.Helper()
	configs := []*Config{DefaultConfig()}
	for _, host := range []string{"hooks.example.com", "hooks.slack.com", "mattermost.example.com", "2001:db8::1"} {
		c, err := NewConfig(WithWebhookHost(host))
		require.NoError(tb, err)
		configs = append(configs, c)
	}
	return configs
}

// existingRedactionTestInputs returns every string constant in the existing
// RedactText and Mask tests, so the differential test covers all their inputs
// without a copy that could fall behind them. Expected outputs and names come
// along too; they are just more inputs.
func existingRedactionTestInputs(tb testing.TB) []string {
	tb.Helper()
	var inputs []string
	fset := token.NewFileSet()
	for _, file := range []string{"redactor_test.go", "value_detector_test.go"} {
		f, err := parser.ParseFile(fset, file, nil, 0)
		require.NoError(tb, err)
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !strings.HasPrefix(fn.Name.Name, "TestRedactText_") && !strings.HasPrefix(fn.Name.Name, "TestValueDetector_") {
				continue
			}
			inputs = append(inputs, stringConstants(fn.Body)...)
		}
	}
	require.NotEmpty(tb, inputs, "the seed collection must find the existing test inputs")
	return inputs
}

// stringConstants returns the string literals under node and the values of the
// constant string concatenations and local string constants it can fold.
func stringConstants(node ast.Node) []string {
	consts := map[string]constant.Value{}
	var eval func(e ast.Expr) (constant.Value, bool)
	eval = func(e ast.Expr) (constant.Value, bool) {
		switch e := e.(type) {
		case *ast.BasicLit:
			if e.Kind != token.STRING {
				return nil, false
			}
			s, err := strconv.Unquote(e.Value)
			if err != nil {
				return nil, false
			}
			return constant.MakeString(s), true
		case *ast.ParenExpr:
			return eval(e.X)
		case *ast.Ident:
			v, ok := consts[e.Name]
			return v, ok
		case *ast.BinaryExpr:
			if e.Op != token.ADD {
				return nil, false
			}
			x, okX := eval(e.X)
			y, okY := eval(e.Y)
			if !okX || !okY {
				return nil, false
			}
			return constant.BinaryOp(x, token.ADD, y), true
		}
		return nil, false
	}

	var out []string
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.GenDecl:
			if n.Tok == token.CONST {
				for _, spec := range n.Specs {
					vs := spec.(*ast.ValueSpec)
					for i, name := range vs.Names {
						if i < len(vs.Values) {
							if v, ok := eval(vs.Values[i]); ok {
								consts[name.Name] = v
							}
						}
					}
				}
			}
		case ast.Expr:
			if v, ok := eval(n); ok {
				out = append(out, constant.StringVal(v))
			}
		}
		return true
	})
	return out
}

// assertRangesReproduceRedactText checks the contract of redactedRanges for one
// input: the ranges are in bounds, sorted and non-overlapping, and replacing
// them reproduces RedactText.
func assertRangesReproduceRedactText(tb testing.TB, c *Config, text string) {
	tb.Helper()
	ranges := c.redactedRanges(text)
	prevEnd := 0
	for _, r := range ranges {
		require.Truef(tb, prevEnd <= r.start && r.start <= r.end && r.end <= len(text),
			"ranges must be in bounds, sorted and non-overlapping: %v for %q", ranges, text)
		prevEnd = r.end
	}
	require.Equalf(tb, c.RedactText(text), replaceSpans(text, ranges, c.placeholder),
		"ranges %v for %q", ranges, text)
}

// TestRedactedRanges_MatchesRedactText is the differential test pinning
// redactedRanges to RedactText over every input of the existing RedactText and
// Mask tests and the inputs in rangeSeeds.
func TestRedactedRanges_MatchesRedactText(t *testing.T) {
	inputs := append(existingRedactionTestInputs(t), rangeSeeds...)
	for _, c := range rangeTestConfigs(t) {
		for _, text := range inputs {
			assertRangesReproduceRedactText(t, c, text)
		}
	}
}

func FuzzRedactedRangesMatchesRedactText(f *testing.F) {
	for _, text := range append(existingRedactionTestInputs(f), rangeSeeds...) {
		f.Add(text)
	}
	configs := rangeTestConfigs(f)
	f.Fuzz(func(t *testing.T, text string) {
		for _, c := range configs {
			assertRangesReproduceRedactText(t, c, text)
		}
	})
}

// TestRedactedRanges_ZeroWidthRange covers the insertion RedactText makes for
// an empty quoted value: it must come back as a zero-width range, not be
// dropped for having no bytes to replace.
func TestRedactedRanges_ZeroWidthRange(t *testing.T) {
	c := DefaultConfig()

	t.Run("empty quoted value", func(t *testing.T) {
		const text = `password=""`
		require.Equal(t, `password="[REDACTED]"`, c.RedactText(text))
		assert.Equal(t, []byteRange{{start: 10, end: 10}}, c.redactedRanges(text))
	})

	t.Run("insertion before a ranged replacement at the same position", func(t *testing.T) {
		// No default rule produces this shape, so the stages are given directly:
		// the first inserts at 3, the second replaces the three original bytes
		// right after that placeholder.
		tbl := newPieceTable("abcdefgh", "<P>")
		tbl.apply(fixedSpans(byteRange{start: 3, end: 3}))
		tbl.apply(fixedSpans(byteRange{start: 6, end: 9}))
		require.Equal(t, "abc<P><P>gh", tbl.render())
		assert.Equal(t, []byteRange{{start: 3, end: 3}, {start: 3, end: 6}}, tbl.ranges())
	})
}

// TestRedactedRanges_StageOverlap covers a stage whose match takes in a
// placeholder an earlier stage inserted, and one that only keeps part of it.
func TestRedactedRanges_StageOverlap(t *testing.T) {
	c := DefaultConfig()

	t.Run("later stage replaces an earlier placeholder", func(t *testing.T) {
		// The PEM stage replaces the block first; the key-name value then spans
		// "x" and that placeholder, so one range must cover both.
		text := "password=x" + testPEMBlock
		require.Equal(t, "password=[REDACTED]", c.RedactText(text))
		assert.Equal(t, []byteRange{{start: 9, end: len(text)}}, c.redactedRanges(text))
	})

	t.Run("kept part overlapping an earlier placeholder leaves it whole", func(t *testing.T) {
		// The key-name match starts on the placeholder's closing bracket, which
		// it keeps as the key boundary.
		text := testPEMBlock + "password=x"
		require.Equal(t, "[REDACTED]password=[REDACTED]", c.RedactText(text))
		n := len(testPEMBlock)
		assert.Equal(t, []byteRange{{start: 0, end: n}, {start: n + 9, end: n + 10}}, c.redactedRanges(text))
	})

	// No default rule matches part of a placeholder, so the remaining cases give
	// the stages directly. The rendering must still follow the stages exactly,
	// and the range covers the whole of the placeholder the stage touched.
	partial := []struct {
		name       string
		second     byteRange
		wantRender string
		wantRanges []byteRange
	}{
		{
			name: "span covering the tail of a placeholder", second: byteRange{start: 3, end: 6},
			wantRender: "ab<<P>fghij", wantRanges: []byteRange{{start: 2, end: 5}},
		},
		{
			name: "span covering the head of a placeholder", second: byteRange{start: 1, end: 3},
			wantRender: "a<P>P>efghij", wantRanges: []byteRange{{start: 1, end: 4}},
		},
		{
			name: "insertion inside a placeholder", second: byteRange{start: 3, end: 3},
			wantRender: "ab<<P>P>efghij", wantRanges: []byteRange{{start: 2, end: 4}},
		},
	}
	for _, tt := range partial {
		t.Run(tt.name, func(t *testing.T) {
			const text = "abcdefghij"
			first := byteRange{start: 2, end: 4}
			tbl := newPieceTable(text, "<P>")
			tbl.apply(fixedSpans(first))
			require.Equal(t, replaceSpans(text, []byteRange{first}, "<P>"), tbl.render())
			tbl.apply(fixedSpans(tt.second))
			require.Equal(t, replaceSpans("ab<P>efghij", []byteRange{tt.second}, "<P>"), tbl.render())
			assert.Equal(t, tt.wantRender, tbl.render())
			assert.Equal(t, tt.wantRanges, tbl.ranges())
		})
	}
}

// TestRedactedRanges_OneRangePerReplacement checks that the ranges follow the
// replacements, not the length of the text.
func TestRedactedRanges_OneRangePerReplacement(t *testing.T) {
	c := DefaultConfig()
	filler := strings.Repeat("plain words ", 20000)
	text := filler + "password=x " + filler + "AKIAIOSFODNN7EXAMPLE " + filler
	assert.Len(t, c.redactedRanges(text), 2)
	assertRangesReproduceRedactText(t, c, text)
}

// fixedSpans returns a stage that replaces the given spans whatever the text.
func fixedSpans(spans ...byteRange) func(string) []byteRange {
	return func(string) []byteRange { return spans }
}
