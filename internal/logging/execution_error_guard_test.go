//go:build test

package logging

import (
	"regexp"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProductionCodeHasNoUserFriendlyError pins that the cause-substitution
// interface and its helpers are gone from production. No production file may
// name UserFriendlyError, GetUserFriendlyMessage, UserMessage or formatCause,
// in code or in a comment: a stale comment would reintroduce the concept and
// invite the substitution back.
func TestProductionCodeHasNoUserFriendlyError(t *testing.T) {
	forbidden := regexp.MustCompile(`\b(UserFriendlyError|GetUserFriendlyMessage|UserMessage|formatCause)\b`)

	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		if match := forbidden.FindString(src); match != "" {
			assert.Failf(t, "forbidden identifier in production",
				"%s names %q; the cause must be reported as its own Error() text", file, match)
		}
	}
}
