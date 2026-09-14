// Package identifier declares group names and command names as a type so that
// renderers can exempt them from value-based redaction. The type is a leaf:
// it imports only the standard library, and its single unexported field keeps
// the construction of named values inside this package.
package identifier

import "log/slog"

// Identifier declares that a string is an identifier: a group name or a
// command name written by a human in the TOML configuration. Code outside
// this package cannot set the unexported field, so a named value can only be
// built through NewIdentifier; the zero value is an empty name.
type Identifier struct {
	name string
}

// Compile-time guard to ensure Identifier implements slog.LogValuer.
var _ slog.LogValuer = Identifier{}

// NewIdentifier declares name as an identifier. It performs no validation and
// no normalization: name validation belongs to the configuration boundary and
// display safety belongs to the interpolation contract.
func NewIdentifier(name string) Identifier {
	return Identifier{name: name}
}

// Name returns the declared name.
func (i Identifier) Name() string {
	return i.name
}

// String returns the declared name so that fmt.Stringer rendering agrees with
// Name.
func (i Identifier) String() string {
	return i.name
}

// LogValue encodes the identifier as a string so that handlers which resolve
// LogValuer values render exactly the declared name.
func (i Identifier) LogValue() slog.Value {
	return slog.StringValue(i.name)
}
