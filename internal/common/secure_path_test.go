//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSecurePathEnv_IncludesCoreutilsDir verifies that the fixed command
// resolution PATH contains the coreutils directory. This is the prerequisite
// for treating commands resolved under CoreutilsDir as residing in a safe
// directory: GenerateAllowedCommandsFromPath(SecurePathEnv) derives the allow
// pattern from this value, so dropping the coreutils directory would break the
// safe-directory decision for coreutils commands.
//
// The PATH list is split into entries and matched for exact element membership
// (rather than a substring match) so a sibling directory that merely has
// CoreutilsDir as a prefix (e.g. ".../coreutils-extra") cannot satisfy it.
func TestSecurePathEnv_IncludesCoreutilsDir(t *testing.T) {
	paths := filepath.SplitList(SecurePathEnv)
	assert.Contains(t, paths, CoreutilsDir,
		"SecurePathEnv %q must contain CoreutilsDir %q", SecurePathEnv, CoreutilsDir)
}
