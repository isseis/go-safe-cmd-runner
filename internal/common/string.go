//nolint:revive // common is an appropriate name for shared utilities package
package common

import (
	"fmt"
	"strings"
	"unicode"
)

// EscapeControlChars escapes control characters and spaces in a string for safe display.
// This ensures terminal control characters don't corrupt the output.
//
// Uses unicode.IsControl to detect control characters, then escapes them using
// standard escape sequences (\n, \t, etc.) for common ones, or \xNN for others.
// Spaces are also escaped as \x20 for clarity in parameter display.
// Regular printable characters (except spaces) are left unchanged for readability.
func EscapeControlChars(s string) string {
	var result strings.Builder
	for _, r := range s {
		if r != ' ' && !unicode.IsControl(r) {
			// Not a space or a control character - output as-is
			result.WriteRune(r)
			continue
		}

		// Use standard escape sequences for common control characters
		switch r {
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\b':
			result.WriteString("\\b")
		case '\f':
			result.WriteString("\\f")
		case '\v':
			result.WriteString("\\v")
		case '\a':
			result.WriteString("\\a")
		default:
			// For space character and other control characters, use \xNN notation
			fmt.Fprintf(&result, "\\x%02x", r)
		}
	}
	return result.String()
}

// ParseKeyValue parses a string in "KEY=VALUE" format like environment variables.
// Returns the key, value, and a boolean indicating successful parsing.
// If the string is not in the correct format or key is empty, returns empty strings and false.
//
// Edge cases:
//   - "=VALUE" (empty key): returns key="", value="", ok=false (invalid)
//   - "KEY=" (empty value): returns key="KEY", value="", ok=true (valid)
//   - "KEY" (no equals): returns key="", value="", ok=false (invalid)
//   - "" (empty string): returns key="", value="", ok=false (invalid)
func ParseKeyValue(env string) (key, value string, ok bool) {
	key, value, found := strings.Cut(env, "=")
	if !found || key == "" {
		return "", "", false
	}
	return key, value, true
}
