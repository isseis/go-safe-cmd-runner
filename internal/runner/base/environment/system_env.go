// Package environment enumerates the system environment and decides which variable
// names must never reach a child process (see denylist.go). It applies no allowlist:
// the effective allowlist checks live in the config layer (ProcessEnvImport) and the
// executor layer (BuildProcessEnvironment).
package environment

import (
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
)

// ParseSystemEnvironment returns every entry of os.Environ() that parses as key=value.
// No access control is applied here; see the package comment for where the allowlist
// and denylist checks live.
func ParseSystemEnvironment() map[string]string {
	result := make(map[string]string)

	for _, env := range os.Environ() {
		variable, value, ok := common.ParseKeyValue(env)
		if !ok {
			continue
		}

		result[variable] = value
	}

	return result
}
