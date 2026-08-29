//go:build cgo && test

package groupmembership

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdviseIncompleteness_CGO verifies that this build never tells an
// operator to rebuild with CGO_ENABLED=1 -- they are already there, so it is
// no remediation at all -- and offers instead what does resolve the denial.
func TestAdviseIncompleteness_CGO(t *testing.T) {
	t.Parallel()

	t.Run("causes the host can produce", func(t *testing.T) {
		t.Parallel()

		for _, cause := range []incompletenessCause{causeUnsupportedPlatform, causeNSSSources} {
			t.Run(cause.String(), func(t *testing.T) {
				t.Parallel()

				advice := adviseIncompleteness(cause)
				assert.NotContains(t, advice.fact, "CGO_ENABLED")
				assert.NotContains(t, advice.remediation, "CGO_ENABLED")
				assert.Contains(t, advice.remediation, "chmod g-w")
			})
		}
	})

	t.Run("causes that mean the implementation is wrong", func(t *testing.T) {
		t.Parallel()

		// causeMalformedLine is reachable only for a build that scans the user
		// database files itself, so on this build it means the same as a cause
		// outside the defined range: report it, do not act on the host.
		for _, cause := range []incompletenessCause{causeMalformedLine, causeUnspecified, causeOutOfRange} {
			t.Run(cause.String(), func(t *testing.T) {
				t.Parallel()

				advice := adviseIncompleteness(cause)
				assert.Contains(t, advice.remediation, "defect")
			})
		}
	})

	// The denial an operator actually sees: the state attributes identify the
	// host and the cause, and the sentinel keeps the decision recognizable to
	// callers.
	t.Run("message carries the state and the sentinel", func(t *testing.T) {
		t.Parallel()

		err := incompleteEnumerationError(unrelatedGID, incompleteVerdict(causeNSSSources, "passwd: sss"))
		require.ErrorIs(t, err, ErrGroupMemberEnumerationIncomplete)

		message := err.Error()
		assert.Contains(t, message, "user_database_source=nss")
		assert.Contains(t, message, "cause=nss-sources")
		assert.Contains(t, message, "detail=passwd: sss")
		assert.NotContains(t, message, "CGO_ENABLED", "the message must not tell a cgo build to become one")
	})
}
