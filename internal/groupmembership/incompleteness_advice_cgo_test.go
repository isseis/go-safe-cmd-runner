//go:build cgo && test

package groupmembership

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdviseIncompleteness_CGO fixes the wording this build offers per cause.
// The whole point of the split is that this build never tells an operator to
// rebuild with CGO_ENABLED=1 -- they are already there, so it names no way out
// of the denial -- and offers instead what does resolve it. The expected
// strings are matched whole: a substring would still match a rewritten one.
//
// Each row is checked on the advice and on the message an operator actually
// sees, so an assembly that stopped carrying the advice fails here too.
func TestAdviseIncompleteness_CGO(t *testing.T) {
	t.Parallel()

	const (
		defectRemediation = "report this as a defect in the enumeration implementation"
		chmod             = "clear the group-writable bit on the path (chmod g-w)"
	)

	// Every cause is listed, including the ones only an implementation error
	// can produce, so that the rows below can be checked against
	// allIncompletenessCauses for completeness.
	want := map[incompletenessCause]incompletenessAdvice{
		causeUnsupportedPlatform: {
			fact:        "this platform offers no way to determine how its user database is configured, so a group's member list cannot be confirmed to cover every member",
			remediation: chmod,
		},
		causeNSSSources: {
			fact:        "/etc/nsswitch.conf does not establish that every member of a group is enumerated: a source it names gives no guarantee of exhaustive enumeration (SSSD returns no directory users under enumerate = False, and no explicit members under ignore_group_members = True), a line it needs is missing or could not be read as written, or the file could not be read; the detail says which",
			remediation: chmod + ", or configure the passwd and group lines with only sources whose enumeration is exhaustive (files, systemd)",
		},
		// Only a build that scans the user database files itself can attach
		// this cause, so on this build it means what an unknown cause means:
		// report it, do not act on the host.
		causeMalformedLine: {
			fact:        "a cause only a build that scans the user database files directly can produce was reported",
			remediation: defectRemediation,
		},
		causeUnspecified: {
			fact:        "the enumeration was judged incomplete but recorded no cause",
			remediation: defectRemediation,
		},
	}

	// Ranging over the defined causes rather than over the rows means a cause
	// added later fails here instead of falling silently into the switch's
	// default, where a host condition would be reported as a defect.
	for _, cause := range allIncompletenessCauses {
		expected, ok := want[cause]
		require.True(t, ok, "cause %s has no expected advice; classify it in adviseIncompleteness and add it here", cause)

		t.Run(cause.String(), func(t *testing.T) {
			t.Parallel()

			advice := adviseIncompleteness(cause)
			assert.Equal(t, expected, advice)

			message := incompleteEnumerationError(unrelatedGID, incompleteVerdict(cause, "detail for "+cause.String())).Error()
			assert.Contains(t, message, expected.fact)
			assert.Contains(t, message, expected.remediation)
			assert.NotContains(t, message, "CGO_ENABLED", "the message must not tell a cgo build to become one")
		})
	}

	// A cause outside the defined range denies like an unrecognized one
	// rather than reaching for wording that fits some host condition.
	t.Run("cause outside the defined range", func(t *testing.T) {
		t.Parallel()

		advice := adviseIncompleteness(causeOutOfRange)
		assert.Equal(t, implementationDefectAdvice("the enumeration was judged incomplete for a cause this build does not recognize"), advice)
	})

	// The state attributes identify the host and the cause, and the sentinel
	// keeps the denial recognizable to callers.
	t.Run("message carries the state and the sentinel", func(t *testing.T) {
		t.Parallel()

		err := incompleteEnumerationError(unrelatedGID, incompleteVerdict(causeNSSSources, "passwd: sss"))
		require.ErrorIs(t, err, ErrGroupMemberEnumerationIncomplete)

		message := err.Error()
		assert.Contains(t, message, "user_database_source=nss")
		assert.Contains(t, message, "cause=nss-sources")
		assert.Contains(t, message, "detail=passwd: sss")
	})
}
