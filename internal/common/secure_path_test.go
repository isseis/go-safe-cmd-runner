//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecurePathEnv_IncludesCoreutilsDir verifies that the fixed command
// resolution PATH contains the coreutils directory. This is the prerequisite
// for treating commands resolved under CoreutilsDir as residing in a safe
// directory: GenerateAllowedCommandsFromPath(SecurePathEnv) derives the allow
// pattern from this value, so dropping the coreutils directory would break the
// safe-directory decision for coreutils commands.
func TestSecurePathEnv_IncludesCoreutilsDir(t *testing.T) {
	assert.True(t, strings.Contains(SecurePathEnv, CoreutilsDir),
		"SecurePathEnv %q must contain CoreutilsDir %q", SecurePathEnv, CoreutilsDir)
}
