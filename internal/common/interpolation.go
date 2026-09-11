package common

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// InterpolationRole selects the transformation rules that the display-safe
// interpolation contract applies to a dynamic value before it is embedded in a
// Slack notification. The contract hides the value's origin: callers declare
// the role and never infer it from the content.
type InterpolationRole int

const (
	// InterpolationRoleFreeText applies the full contract: one-line
	// normalization, format-control removal, entity escaping and the length
	// limit. It is the zero value, so an unknown role falls back to the
	// strongest processing.
	InterpolationRoleFreeText InterpolationRole = iota
	// InterpolationRoleIdentifier keeps the full contract except that it never
	// truncates. Identifiers are rejected at the configuration boundary when
	// they are too long, because two names that differ only beyond the cut
	// would otherwise render as the same scope.
	InterpolationRoleIdentifier
	// InterpolationRoleEnvelopeValue applies the full contract including the
	// length limit. Hostname and Run ID are shorter than the limit in practice,
	// but they take the same path as free text rather than a special one.
	InterpolationRoleEnvelopeValue
	// InterpolationRoleBulkOutput is passed through unchanged. stdout and stderr
	// keep their existing truncation rules and are not one-line normalized or
	// entity-escaped by this contract.
	InterpolationRoleBulkOutput
)

// interpolationMaxBytes is the UTF-8 byte length limit shared by the free-text
// and envelope-value roles.
const interpolationMaxBytes = 500

// interpolatedEntities lists the entity references this contract generates.
// Truncation must not leave a proper prefix of one of them at the end.
var interpolatedEntities = [...]string{"&amp;", "&lt;", "&gt;"}

// Interpolate applies the display-safe interpolation contract to value for the
// given role and returns the value to embed in a notification. The
// transformation order is fixed: invalid UTF-8, then control characters and
// Unicode line separators, then format-control characters, then entity
// escaping, then truncation for the roles that allow it.
func Interpolate(value string, role InterpolationRole) string {
	switch role {
	case InterpolationRoleBulkOutput:
		return value
	case InterpolationRoleIdentifier:
		return transformInterpolated(value, false)
	case InterpolationRoleFreeText, InterpolationRoleEnvelopeValue:
		return transformInterpolated(value, true)
	default:
		// The zero value and any unknown role take the strongest processing.
		return transformInterpolated(value, true)
	}
}

// HasDisplayableContent reports whether value retains at least one rune that is
// not Unicode White_Space after passing the interpolation contract for
// identifiers. Configuration validation and notification context validation
// share this predicate so a name that renders as blank is rejected at the
// setting boundary and exposed at the display boundary by the same rule.
func HasDisplayableContent(value string) bool {
	interpolated := Interpolate(value, InterpolationRoleIdentifier)
	for _, r := range interpolated {
		if !unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// transformInterpolated performs the ordered transformations of the contract.
func transformInterpolated(value string, truncate bool) string {
	value = strings.ToValidUTF8(value, string(utf8.RuneError))

	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case unicode.IsControl(r), r == '\u2028', r == '\u2029':
			builder.WriteByte(' ')
		case unicode.Is(unicode.Cf, r):
			builder.WriteByte(' ')
		case r == '&':
			builder.WriteString("&amp;")
		case r == '<':
			builder.WriteString("&lt;")
		case r == '>':
			builder.WriteString("&gt;")
		default:
			builder.WriteRune(r)
		}
	}

	out := builder.String()
	if truncate {
		out = truncateInterpolated(out)
	}
	return out
}

// truncateInterpolated cuts value to the longest prefix that is at most
// interpolationMaxBytes long without splitting a rune or one of the generated
// entity references. Callers must pass valid UTF-8, which
// transformInterpolated guarantees.
func truncateInterpolated(value string) string {
	if len(value) <= interpolationMaxBytes {
		return value
	}

	prefix := value[:interpolationMaxBytes]
	for len(prefix) > 0 {
		r, size := utf8.DecodeLastRuneInString(prefix)
		if r != utf8.RuneError || size != 1 {
			break
		}
		prefix = prefix[:len(prefix)-1]
	}

	for _, entity := range interpolatedEntities {
		for n := len(entity) - 1; n >= 1; n-- {
			if strings.HasSuffix(prefix, entity[:n]) {
				return prefix[:len(prefix)-n]
			}
		}
	}
	return prefix
}
