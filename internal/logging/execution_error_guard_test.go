//go:build test

package logging

import (
	"regexp"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/testutil/identitymutationguard"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forbiddenCauseSubstitutionPattern matches the identifiers of the removed
// cause-substitution interface and its helpers.
var forbiddenCauseSubstitutionPattern = regexp.MustCompile(`\b(UserFriendlyError|GetUserFriendlyMessage|UserMessage|formatCause)\b`)

// TestProductionCodeHasNoUserFriendlyError pins that the cause-substitution
// interface and its helpers are gone from production. No production file may
// name UserFriendlyError, GetUserFriendlyMessage, UserMessage or formatCause,
// in code or in a comment: a stale comment would reintroduce the concept and
// invite the substitution back.
func TestProductionCodeHasNoUserFriendlyError(t *testing.T) {
	files := identitymutationguard.ProductionGoFilesInRepo(t)
	require.NotEmpty(t, files, "the repository scan returned no production files")

	for _, file := range files {
		src := identitymutationguard.ReadProductionSource(t, file)
		if match := forbiddenCauseSubstitutionPattern.FindString(src); match != "" {
			assert.Failf(t, "forbidden identifier in production",
				"%s names %q; the cause must be reported as its own Error() text", file, match)
		}
	}
}

// TestUserFriendlyErrorCheckRecognizesForms pins that the scan detects each
// forbidden name, so it cannot become a no-op that passes the repository scan.
func TestUserFriendlyErrorCheckRecognizesForms(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{name: "interface name", src: "var _ = UserFriendlyError(nil)\n", want: true},
		{name: "helper name", src: "var _ = GetUserFriendlyMessage\n", want: true},
		{name: "method name", src: "var _ = e.UserMessage()\n", want: true},
		{name: "formatter name", src: "var _ = formatCause\n", want: true},
		{name: "name in a comment", src: "// formatCause was removed\n", want: true},
		{name: "clean source", src: "func f(err error) string { return err.Error() }\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, forbiddenCauseSubstitutionPattern.MatchString(tt.src))
		})
	}
}
