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

// argParser holds the mutable state threaded through one parseArgs call: the flag
// lookup table, the argv being scanned, the cursor into it, and the result being
// built. Keeping this state on a struct (rather than passing a closure plus indices
// between free functions) is the idiomatic Go way to share it across the helpers.
type argParser struct {
	byName map[string]*FlagSpec
	args   []string
	i      int
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
	for i := range flags {
		f := &flags[i]
		for _, n := range f.Names {
			p.byName[n] = f
		}
	}

	endOpts := false
	for p.i = 0; p.i < len(p.args); p.i++ {
		a := p.args[p.i]
		switch {
		case !endOpts && a == "--":
			endOpts = true
		case endOpts || len(a) < 2 || a[0] != '-':
			p.res.NonFlagArgs = append(p.res.NonFlagArgs, a)
		default:
			if !p.parseFlagToken(a) {
				p.res.Recognized = false
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

// parseFlagToken classifies one option token (a, already known to start with '-' and
// not be "--"). It advances the cursor when a value flag consumes the next token, and
// returns false when the token cannot be fully recognized (unknown flag or missing
// required value), so the caller can set Recognized=false.
func (p *argParser) parseFlagToken(a string) bool {
	name, val, hasEq := strings.Cut(a, "=")
	if f, ok := p.byName[name]; ok {
		return p.markNamedFlag(f, val, hasEq)
	}
	// An unknown long flag (--foo) may take a value, so fail closed.
	if strings.HasPrefix(a, "--") {
		return false
	}
	return p.parseShortCluster(a)
}

// markNamedFlag records an exactly-spelled flag and consumes its value per arity. It
// returns false only when a required value is missing.
func (p *argParser) markNamedFlag(f *FlagSpec, val string, hasEq bool) bool {
	switch f.Arity {
	case ArityRequired:
		switch {
		case hasEq:
			p.mark(f, val)
		case p.i+1 < len(p.args):
			p.mark(f, p.args[p.i+1])
			p.i++
		default:
			return false // required value missing at end of argv
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
	return true
}

// parseShortCluster parses a short-flag cluster (e.g. -rf, or a value flag with an
// attached value like -C/usr). A value flag inside the cluster captures the rest of
// the token as its value (or, when required and nothing is attached, the next token);
// dropping that value would default a destination to the cwd (fail-open). It returns
// false on an unknown cluster char or a required value with nothing to consume.
func (p *argParser) parseShortCluster(a string) bool {
	for k, c := range a[1:] {
		f, ok := p.byName["-"+string(c)]
		if !ok {
			return false
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
		case f.Arity == ArityRequired && p.i+1 < len(p.args):
			p.mark(f, p.args[p.i+1])
			p.i++
		case f.Arity == ArityOptional:
			p.mark(f) // optional, no attached value; do not consume the next token
		default:
			return false // required value flag with nothing to consume
		}
		return true // a value flag consumes the remainder of the cluster
	}
	return true
}
