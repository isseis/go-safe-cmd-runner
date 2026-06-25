package security

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// optFlags is a representative flag set exercising each arity, recursion, and
// spelling aliases, used by the parser tests below.
func optFlags() []FlagSpec {
	return []FlagSpec{
		{Names: []string{"-t", "--target-directory"}, Arity: ArityRequired, Value: ValueWrite},
		{Names: []string{"-C", "--directory"}, Arity: ArityRequired, Value: ValueNonPath},
		{Names: []string{"-r", "-R", "--recursive"}, Arity: ArityNone, Recursive: true},
		{Names: []string{"-f", "--force"}, Arity: ArityNone},
		{Names: []string{"--one-top-level"}, Arity: ArityOptional, Value: ValueWrite},
		{Names: []string{"-i", "--in-place"}, Arity: ArityOptional, Value: ValueNonPath},
	}
}

// TestParseArgs_Forms covers the flag forms the single parser unifies.
func TestParseArgs_Forms(t *testing.T) {
	cases := []struct {
		name        string
		args        []string
		wantValues  map[string][]string
		wantNonFlag []string
		wantRec     bool
	}{
		{
			name:        "long flag with attached value",
			args:        []string{"--target-directory=/dst", "a", "b"},
			wantValues:  map[string][]string{"-t": {"/dst"}},
			wantNonFlag: []string{"a", "b"},
		},
		{
			name:        "long flag with separate value",
			args:        []string{"--target-directory", "/dst", "a"},
			wantValues:  map[string][]string{"-t": {"/dst"}},
			wantNonFlag: []string{"a"},
		},
		{
			name:        "short flag with attached value",
			args:        []string{"-C/usr", "x"},
			wantValues:  map[string][]string{"-C": {"/usr"}},
			wantNonFlag: []string{"x"},
		},
		{
			name:        "short flag with separate value",
			args:        []string{"-t", "/dst"},
			wantValues:  map[string][]string{"-t": {"/dst"}},
			wantNonFlag: nil,
		},
		{
			name:        "short cluster of boolean and recursion flags",
			args:        []string{"-rf", "x"},
			wantValues:  map[string][]string{"-r": {}, "-f": {}},
			wantNonFlag: []string{"x"},
			wantRec:     true,
		},
		{
			name:        "double dash terminates option parsing",
			args:        []string{"--", "-t", "x"},
			wantValues:  map[string][]string{},
			wantNonFlag: []string{"-t", "x"},
		},
		{
			name:        "single dash is a non-flag argument",
			args:        []string{"-", "x"},
			wantValues:  map[string][]string{},
			wantNonFlag: []string{"-", "x"},
		},
		{
			name:        "value flag at cluster tail captures next token",
			args:        []string{"-rt", "/dst"},
			wantValues:  map[string][]string{"-r": {}, "-t": {"/dst"}},
			wantNonFlag: nil,
			wantRec:     true,
		},
		{
			name:        "duplicate value flag accumulates values in order",
			args:        []string{"-t", "/a", "-t", "/b"},
			wantValues:  map[string][]string{"-t": {"/a", "/b"}},
			wantNonFlag: nil,
		},
		{
			name:        "empty token is a non-flag argument",
			args:        []string{"", "x"},
			wantValues:  map[string][]string{},
			wantNonFlag: []string{"", "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseArgs(optFlags(), tc.args)
			assert.True(t, got.Recognized, "valid input must be recognized")
			assert.Equal(t, tc.wantRec, got.Recursive)
			assert.Equal(t, tc.wantNonFlag, got.NonFlagArgs)
			assert.Equal(t, tc.wantValues, got.Values)
		})
	}
}

// TestParseArgs_FailClosed verifies that no token is silently dropped: an unknown
// flag or a missing required value sets Recognized=false.
func TestParseArgs_FailClosed(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"unknown long flag", []string{"--unknown", "x"}},
		{"unknown long flag with value form", []string{"--unknown=v", "x"}},
		{"required value missing at end", []string{"-t"}},
		{"required long value missing at end", []string{"--target-directory"}},
		{"unknown short flag", []string{"-z"}},
		{"unknown char in cluster", []string{"-rz"}},
		{"required value flag at cluster tail with nothing to consume", []string{"-rt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseArgs(optFlags(), tc.args)
			assert.False(t, got.Recognized, "%v must fail closed", tc.args)
		})
	}

	// A fully-known input is recognized (happy path).
	ok := parseArgs(optFlags(), []string{"-rf", "-t", "/dst", "src"})
	assert.True(t, ok.Recognized)
}

// TestParseArgs_AliasNormalization verifies spelling variants of one flag normalize
// to the same canonical key.
func TestParseArgs_AliasNormalization(t *testing.T) {
	short := parseArgs(optFlags(), []string{"-t", "/dst"})
	long := parseArgs(optFlags(), []string{"--target-directory", "/dst"})
	assert.Equal(t, short.Values, long.Values, "spelling variants must produce the same Values")
	assert.Equal(t, []string{"/dst"}, short.Values["-t"])

	// A recursion alias normalizes to the canonical recursion key and sets Recursive.
	rLong := parseArgs(optFlags(), []string{"--recursive"})
	rShort := parseArgs(optFlags(), []string{"-R"})
	assert.True(t, rLong.Recursive)
	assert.True(t, rShort.Recursive)
	assert.True(t, rLong.HasFlag("-r"))
	assert.True(t, rShort.HasFlag("-r"))
}

// TestParseArgs_OptionalArg verifies optional-argument flags take a value only in the
// attached form and never consume a separate following token.
func TestParseArgs_OptionalArg(t *testing.T) {
	// Bare long optional flag: present, no value; the following token is a non-flag arg.
	bare := parseArgs(optFlags(), []string{"--one-top-level", "a.tar"})
	assert.True(t, bare.Recognized)
	assert.True(t, bare.HasFlag("--one-top-level"))
	assert.Empty(t, bare.Values["--one-top-level"], "bare optional flag has no value")
	assert.Equal(t, []string{"a.tar"}, bare.NonFlagArgs, "the next token must not be consumed")

	// Attached long form captures the value.
	attached := parseArgs(optFlags(), []string{"--one-top-level=/top", "a.tar"})
	assert.Equal(t, []string{"/top"}, attached.Values["--one-top-level"])
	assert.Equal(t, []string{"a.tar"}, attached.NonFlagArgs)

	// Optional short flag inside a cluster takes the remainder as its value (sed -ir).
	cluster := parseArgs(optFlags(), []string{"-ir"})
	assert.True(t, cluster.Recognized)
	assert.Equal(t, []string{"r"}, cluster.Values["-i"], "-ir means -i with attached value r, not -i -r")

	// Bare optional short flag: present, no value; the next token is not consumed.
	bareShort := parseArgs(optFlags(), []string{"-i", "x"})
	assert.True(t, bareShort.HasFlag("-i"))
	assert.Empty(t, bareShort.Values["-i"])
	assert.Equal(t, []string{"x"}, bareShort.NonFlagArgs)
}

// TestParseArgs_NoArgFlagIgnoresAttachedValue pins that a no-argument flag given an
// attached =value (e.g. --force=x) stays recognized and present but carries no value;
// the attached value is dropped. This matches legacy scanFlags: tightening it to
// fail-closed would break behavioral parity.
func TestParseArgs_NoArgFlagIgnoresAttachedValue(t *testing.T) {
	got := parseArgs(optFlags(), []string{"--force=x", "a"})
	assert.True(t, got.Recognized)
	assert.True(t, got.HasFlag("-f"))
	assert.Empty(t, got.Values["-f"], "attached value on a no-arg flag is dropped")
	assert.Equal(t, []string{"a"}, got.NonFlagArgs)
}

// TestParseArgs_ShortClusterAttachedEquals locks getopt short-option semantics: inside
// a short cluster the '=' is part of the attached value (-rt=/dst means -t with value
// "=/dst"), not a key/value separator. This matches GNU getopt for short options and
// the legacy scanFlags behavior, so it must not be "fixed" by stripping the '=': doing
// so would break behavioral parity and could turn a captured value into an empty one
// that falls through to consume the next token.
func TestParseArgs_ShortClusterAttachedEquals(t *testing.T) {
	got := parseArgs(optFlags(), []string{"-rt=/dst", "src"})
	assert.True(t, got.Recognized)
	assert.True(t, got.Recursive)
	assert.Equal(t, []string{"=/dst"}, got.Values["-t"], "short-option '=' is part of the value, not a separator")
	assert.Equal(t, []string{"src"}, got.NonFlagArgs, "the next token must not be consumed")
}

// TestParseArgs_MalformedUTF8 pins that argv tokens containing invalid UTF-8 bytes
// (possible on Unix) are handled without panicking. A standalone malformed byte is an
// unknown short flag and fails closed; malformed bytes trailing a value flag are
// captured as that flag's opaque value without a slice-bounds panic.
func TestParseArgs_MalformedUTF8(t *testing.T) {
	// A lone malformed byte is not a known flag -> fail closed, no panic.
	bad := parseArgs(optFlags(), []string{string([]byte{'-', 0xff})})
	assert.False(t, bad.Recognized, "a malformed-byte short flag must fail closed")

	// Malformed bytes attached to a value flag are captured as the value, no panic.
	val := parseArgs(optFlags(), []string{string([]byte{'-', 't', 0xff})})
	assert.True(t, val.Recognized)
	assert.Equal(t, []string{string([]byte{0xff})}, val.Values["-t"])
}

// TestParseArgs_HasFlag verifies presence detection for a no-argument flag, where the
// value slice is empty (the len>0 trap).
func TestParseArgs_HasFlag(t *testing.T) {
	got := parseArgs(optFlags(), []string{"-f"})
	assert.True(t, got.HasFlag("-f"), "a no-arg flag that appeared is present")
	assert.Empty(t, got.Values["-f"], "a no-arg flag is stored as an empty slice")
	assert.False(t, got.HasFlag("-t"), "a flag that did not appear is absent")
}

// TestParseArgs_Pathological confirms large and deeply clustered inputs are handled
// linearly without panicking.
func TestParseArgs_Pathological(t *testing.T) {
	many := make([]string, 10000)
	for i := range many {
		many[i] = "p"
	}
	got := parseArgs(optFlags(), many)
	assert.True(t, got.Recognized)
	assert.Len(t, got.NonFlagArgs, 10000)

	longCluster := "-" + strings.Repeat("rf", 5000) // all known boolean/recursion chars
	gotCluster := parseArgs(optFlags(), []string{longCluster})
	assert.True(t, gotCluster.Recognized)
	assert.True(t, gotCluster.Recursive)
}
