package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSpecCompleteness enforces the declarative-table invariants the design relies on:
// every flag has at least one spelling, a no-argument flag carries no value role, and
// every argument-taking flag declares a concrete role. The last rule is the structural
// guard against a path-carrying flag being left unclassified and silently ignored.
func TestSpecCompleteness(t *testing.T) {
	for cmd, spec := range commandFlagSpecs {
		for _, f := range spec.Flags {
			assert.NotEmpty(t, f.Names, "%s: a FlagSpec has no names", cmd)
			switch f.Arity {
			case ArityNone:
				assert.Equal(t, ValueUnset, f.Value, "%s %v: a no-argument flag must have ValueUnset", cmd, f.Names)
			case ArityRequired, ArityOptional:
				assert.NotEqual(t, ValueUnset, f.Value,
					"%s %v: an argument-taking flag must declare a concrete role", cmd, f.Names)
			default:
				t.Fatalf("%s %v: unknown arity %d", cmd, f.Names, f.Arity)
			}
		}
	}
}

// TestSpecNoDuplicateNames verifies no spelling appears in more than one FlagSpec of a
// command. A duplicate would make the parser's name->spec map ambiguous and usually
// signals a flag mistakenly listed as both value-taking and boolean.
func TestSpecNoDuplicateNames(t *testing.T) {
	for cmd, spec := range commandFlagSpecs {
		seen := make(map[string]bool)
		for _, f := range spec.Flags {
			for _, n := range f.Names {
				assert.False(t, seen[n], "%s: flag name %q declared more than once", cmd, n)
				seen[n] = true
			}
		}
	}
}

// TestEveryCommandHasExtractor verifies the production invariant that every registry
// entry has a ToExtraction wired, so the dispatcher never calls a nil function. This is
// a production-only check (no dependency on the test-tagged legacy oracle), so it also
// compiles under a plain `go test` without the build tag. The legacy<->new command-set
// equality is checked separately by TestLegacyZoningSpecsCoverage (which lives with the
// frozen oracle behind //go:build test).
func TestEveryCommandHasExtractor(t *testing.T) {
	for cmd, spec := range commandFlagSpecs {
		assert.NotNil(t, spec.ToExtraction, "%s: CommandFlagSpec has a nil ToExtraction (the dispatcher would panic)", cmd)
	}
}

// TestArityInvariant pins the parsing consequence of each declared arity: a required
// flag consumes a separated following token as its value, while an optional or boolean
// flag never does (GNU getopt: optional args attach only). This guards spec/parser
// consistency at the per-flag level. The complementary check -- that each flag's arity
// matches the legacy extractor's actual token consumption -- is enforced end-to-end by
// TestExtractionDifferential, which exercises the separated `-x v` form for every flag
// and would reveal any operand shift caused by an arity mismatch.
func TestArityInvariant(t *testing.T) {
	const next = "NEXTVALUE"
	for cmd, spec := range commandFlagSpecs {
		for _, f := range spec.Flags {
			if !assert.NotEmpty(t, f.Names, "%s: a FlagSpec has no names", cmd) {
				continue // guard f.Names[0]; TestSpecCompleteness reports this in detail
			}
			key := f.Names[0]
			for _, name := range f.Names {
				res := parseArgs(spec.Flags, []string{name, next})
				switch f.Arity {
				case ArityRequired:
					assert.Equal(t, []string{next}, res.Values[key],
						"%s %s: a required flag must consume the next token", cmd, name)
					assert.NotContains(t, res.NonFlagArgs, next,
						"%s %s: a consumed value must not remain a positional", cmd, name)
				case ArityOptional:
					assert.Contains(t, res.NonFlagArgs, next,
						"%s %s: an optional flag must not consume the next token", cmd, name)
				case ArityNone:
					assert.Contains(t, res.NonFlagArgs, next,
						"%s %s: a boolean flag must not consume the next token", cmd, name)
				}
			}
		}
	}
}

// TestAliasAddition demonstrates the declarative-table goal: all
// spellings of one value flag share a single entry and a single canonical key, and
// adding a new spelling to Names is the only change needed for that spelling to work --
// no parsing-code branch.
func TestAliasAddition(t *testing.T) {
	base := []FlagSpec{valueFlag(ValueWrite, "-t", "--target-directory")}
	for _, name := range []string{"-t", "--target-directory"} {
		res := parseArgs(base, []string{name, "/dst"})
		assert.Equal(t, []string{"/dst"}, res.Values["-t"], "%s should resolve to canonical -t", name)
	}

	extended := []FlagSpec{valueFlag(ValueWrite, "-t", "--target-directory", "--to")}
	res := parseArgs(extended, []string{"--to", "/dst"})
	assert.Equal(t, []string{"/dst"}, res.Values["-t"], "a newly added alias --to must resolve to canonical -t")
}
