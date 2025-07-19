package environment

import "strings"

// ParseEnvVariable parses an environment variable string in "KEY=VALUE" format.
// Returns the key, value, and a boolean indicating successful parsing.
// If the string is not in the correct format, returns empty strings and false.
func ParseEnvVariable(env string) (key, value string, ok bool) {
	parts := strings.SplitN(env, "=", envSeparatorParts)
	if len(parts) != envSeparatorParts {
		return "", "", false
	}
	return parts[0], parts[1], true
}
