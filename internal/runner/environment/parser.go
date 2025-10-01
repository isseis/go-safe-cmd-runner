package environment

import "github.com/isseis/go-safe-cmd-runner/internal/common"

// ParseEnvVariable parses an environment variable string in "KEY=VALUE" format.
// Returns the key, value, and a boolean indicating successful parsing.
// If the string is not in the correct format, returns empty strings and false.
func ParseEnvVariable(env string) (key, value string, ok bool) {
	return common.ParseEnvVariable(env)
}
