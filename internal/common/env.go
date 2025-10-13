//nolint:revive // common is an appropriate name for shared utilities package
package common

import "strings"

// ParseEnvVariable parses an environment variable string in "KEY=VALUE" format.
// Returns the key, value, and a boolean indicating successful parsing.
// If the string is not in the correct format or key is empty, returns empty strings and false.
//
// Edge cases:
//   - "=VALUE" (empty key): returns key="", value="", ok=false (invalid)
//   - "KEY=" (empty value): returns key="KEY", value="", ok=true (valid)
//   - "KEY" (no equals): returns key="", value="", ok=false (invalid)
//   - "" (empty string): returns key="", value="", ok=false (invalid)
func ParseEnvVariable(env string) (key, value string, ok bool) {
	key, value, found := strings.Cut(env, "=")
	if !found || key == "" {
		return "", "", false
	}
	return key, value, true
}
