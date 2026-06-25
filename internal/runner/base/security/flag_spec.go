package security

// FlagArity describes whether a flag takes an argument and, if it does, whether the
// argument is required or optional.
type FlagArity int

// Flag arities recognized by the parser.
const (
	ArityNone     FlagArity = iota // boolean flag: takes no argument
	ArityRequired                  // takes a required argument (attached form or the next token)
	ArityOptional                  // optional argument: attached form only (--flag=v / -ov), never the next token
)

// ValueRole classifies the value captured by an argument-taking flag. ValueUnset is
// the zero value and marks an unclassified value flag; the completeness meta-test
// rejects it, so every argument-taking flag must declare a concrete role.
type ValueRole int

// Value roles for argument-taking flags.
const (
	ValueUnset   ValueRole = iota // unclassified (zero-value sentinel; fails the completeness meta-test)
	ValueNonPath                  // a non-path value that may be ignored (explicitly classified)
	ValueWrite                    // the value is a write-destination operand
	ValueRead                     // the value is a read-source operand
)

// FlagSpec is the declarative specification of one logical flag. Every spelling
// (short and long) is collected in Names; Names[0] is the canonical key under which
// the parser records the flag. Names must hold at least one element.
type FlagSpec struct {
	Names     []string
	Arity     FlagArity
	Recursive bool      // a recursion flag (e.g. -r / -R / -a)
	Value     ValueRole // role of the captured value when Arity is Required/Optional; ValueUnset when ArityNone
}

// CommandFlagSpec is one command's declarative flag set plus the thin function that
// maps a parsed result into an extraction. The raw argv is passed to ToExtraction
// only for the non-getopt value grammars (dd's if=/of= key=value, chattr's attribute
// tokens); getopt-conformant commands read flag values from the ParseResult alone.
type CommandFlagSpec struct {
	Kind         LocationKind
	Flags        []FlagSpec
	ToExtraction func(ParseResult, []string) extraction
}
