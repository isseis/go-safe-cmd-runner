package security

import "strings"

// ParseResult is the output of the single getopt parser. Flag values are keyed by the
// flag's canonical name (FlagSpec.Names[0]), so a spelling variant ("-t" vs
// "--target-directory") yields the same key. A no-argument flag is recorded with an
// empty (non-nil) slice, so its presence is the existence of the key, not a non-empty
// length: use HasFlag for presence.
type ParseResult struct {
	Values      map[string][]string // canonical flag name -> captured values (empty slice for no-arg flags)
	Recursive   bool                // a recursion flag appeared at least once
	NonFlagArgs []string            // arguments that are neither a flag nor a flag's value
	Recognized  bool                // every token was classified (false is fail-closed)
}

// HasFlag reports whether the flag with the given canonical key (FlagSpec.Names[0])
// appeared. It must be used for presence checks: len(Values[k]) > 0 misdetects a
// no-argument flag, which is stored as an empty slice.
func (r ParseResult) HasFlag(canonicalKey string) bool {
	_, ok := r.Values[canonicalKey]
	return ok
}

// argParser holds the state shared across one parseArgs call: the flag lookup table
// and the argv being scanned (both read-only after setup), plus the result being
// accumulated. The cursor into argv is NOT stored here; it is a local owned by the
// parseArgs loop, which is its single writer. Helpers read the cursor and report back
// (via consumedNext) whether they took the following token, instead of mutating it.
// Using a struct with methods (rather than passing a closure and indices between free
// functions) is the idiomatic Go way to share this.
type argParser struct {
	byName map[string]*FlagSpec
	args   []string
	res    ParseResult
}

// parseArgs is the single getopt parser shared by every command. It consumes a flag
// set (only the flags; it never inspects Kind or ToExtraction) and classifies argv.
//
// Contract: it never silently drops a token. Each argument becomes a recognized flag,
// a captured flag value, or a NonFlagArg. An unknown flag or a missing required value
// sets Recognized=false (fail-closed) rather than guessing. It handles --flag=value,
// an attached short value (-C/usr), a short cluster (-rf), the -- option terminator,
// optional arguments (attached form only), and spelling-alias normalization (values
// recorded under the canonical name). It is pure: linear in total argv length, with
// no filesystem, environment, or process-identity access.
func parseArgs(flags []FlagSpec, args []string) ParseResult {
	p := &argParser{
		byName: make(map[string]*FlagSpec),
		args:   args,
		res:    ParseResult{Values: make(map[string][]string), Recognized: true},
	}
	for _, f := range flags {
		for _, n := range f.Names {
			p.byName[n] = &f
		}
	}

	// i is the cursor and the loop is its only writer. Each token is the "--"
	// terminator, a non-flag argument, or a flag token. A flag that takes a separate
	// value also consumes the next token; parseFlagToken reports that via consumedNext
	// so the loop skips it. A non-recognized token records fail-closed but does not
	// stop the scan.
	endOpts := false
	for i := 0; i < len(p.args); i++ {
		a := p.args[i]
		switch {
		case !endOpts && a == "--":
			endOpts = true
		case endOpts || len(a) < 2 || a[0] != '-':
			p.res.NonFlagArgs = append(p.res.NonFlagArgs, a)
		default:
			consumedNext, ok := p.parseFlagToken(i)
			if !ok {
				p.res.Recognized = false
			}
			if consumedNext {
				i++
			}
		}
	}
	return p.res
}

// mark records a flag's presence under its canonical key and appends any captured
// values (none for a bare boolean or value-less optional flag).
func (p *argParser) mark(f *FlagSpec, vals ...string) {
	key := f.Names[0]
	if _, ok := p.res.Values[key]; !ok {
		p.res.Values[key] = []string{}
	}
	p.res.Values[key] = append(p.res.Values[key], vals...)
	if f.Recursive {
		p.res.Recursive = true
	}
}

// parseFlagToken classifies the option token at args[i] (known to start with '-' and
// not be "--"). consumedNext is true when the flag took args[i+1] as its value, so the
// caller skips it. ok is false when the token cannot be fully recognized (unknown flag
// or missing required value), so the caller can set Recognized=false.
func (p *argParser) parseFlagToken(i int) (consumedNext, ok bool) {
	a := p.args[i]
	name, val, hasEq := strings.Cut(a, "=")
	if f, found := p.byName[name]; found {
		return p.markNamedFlag(f, i, val, hasEq)
	}
	// An unknown long flag (--foo) may take a value, so fail closed.
	if strings.HasPrefix(a, "--") {
		return false, false
	}
	return p.parseShortCluster(i)
}

// markNamedFlag records an exactly-spelled flag and consumes its value per arity.
// consumedNext is true when a required value was taken from args[i+1]; ok is false only
// when a required value is missing.
func (p *argParser) markNamedFlag(f *FlagSpec, i int, val string, hasEq bool) (consumedNext, ok bool) {
	switch f.Arity {
	case ArityRequired:
		switch {
		case hasEq:
			p.mark(f, val)
		case i+1 < len(p.args):
			p.mark(f, p.args[i+1])
			return true, true
		default:
			return false, false // required value missing at end of argv
		}
	case ArityOptional:
		if hasEq {
			p.mark(f, val)
		} else {
			p.mark(f) // present without a value; never consume the next token
		}
	default: // ArityNone
		p.mark(f) // boolean/recursion flag; any attached =value is ignored (legacy parity)
	}
	return false, true
}

// parseShortCluster parses a short-flag cluster at args[i] (e.g. -rf, or a value flag
// with an attached value like -C/usr). A value flag inside the cluster captures the
// rest of the token as its value, or args[i+1] when nothing is attached (consumedNext);
// dropping that value would default a destination to the cwd (fail-open). ok is false
// on an unknown cluster char or a required value with nothing to consume.
func (p *argParser) parseShortCluster(i int) (consumedNext, ok bool) {
	a := p.args[i]
	for k, c := range a[1:] {
		f, found := p.byName["-"+string(c)]
		if !found {
			return false, false
		}
		if f.Arity == ArityNone {
			p.mark(f)
			continue
		}
		// c begins at byte 1+k within a (1 for the leading dash); the attached value is
		// everything after c. Use len(string(c)) rather than a fixed offset so a
		// multi-byte rune is not sliced in half.
		rest := a[1+k+len(string(c)):]
		switch {
		case rest != "":
			p.mark(f, rest)
		case f.Arity == ArityRequired && i+1 < len(p.args):
			p.mark(f, p.args[i+1])
			return true, true
		case f.Arity == ArityOptional:
			p.mark(f) // optional, no attached value; do not consume the next token
		default:
			return false, false // required value flag with nothing to consume
		}
		return false, true // a value flag consumes the remainder of the cluster
	}
	return false, true
}
