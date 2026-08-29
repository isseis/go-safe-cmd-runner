//go:build cgo && test

package groupmembership

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdviseIncompleteness_CGO checks the properties this build's advice must
// have, not the sentences it happens to use. This wording is meant to be
// improved on, so copying it into expectations would turn every improvement
// into a test failure with no defect behind it. The non-cgo wording is a
// different matter -- it is deliberately frozen, and its test pins it
// literally for that reason.
//
// What must hold, and is checked below:
//
//   - This build never tells an operator to rebuild with CGO_ENABLED=1. They
//     are already there, so it would name no way out of the denial.
//   - A cause an operator can act on names the action; a cause only an
//     implementation error can produce says so instead.
//   - Each cause has its own advice, so a branch that returned another
//     cause's advice is caught even though no sentence is pinned.
//   - The advice reaches the message the operator sees.
func TestAdviseIncompleteness_CGO(t *testing.T) {
	t.Parallel()

	// Causes a host condition can produce. The rest are reachable only
	// through an implementation error, including causeMalformedLine: only a
	// build that scans the user database files itself can attach it.
	actionable := map[incompletenessCause]struct{}{
		causeUnsupportedPlatform: {},
		causeNSSSources:          {},
	}

	// Ranged over rather than listed, so a cause added later is classified
	// here instead of falling into the switch's default.
	seen := make(map[incompletenessAdvice]incompletenessCause, len(allIncompletenessCauses))
	for _, cause := range allIncompletenessCauses {
		advice := adviseIncompleteness(cause)

		if other, duplicate := seen[advice]; duplicate {
			t.Errorf("causes %s and %s give identical advice; one of them is reaching the other's branch", other, cause)
		}
		seen[advice] = cause

		t.Run(cause.String(), func(t *testing.T) {
			t.Parallel()

			assert.NotEmpty(t, advice.fact)
			assert.NotEmpty(t, advice.remediation)
			assert.NotContains(t, advice.fact, "CGO_ENABLED")
			assert.NotContains(t, advice.remediation, "CGO_ENABLED")

			if _, ok := actionable[cause]; ok {
				// chmod g-w is the action itself, not a turn of phrase: the
				// denial is reached through the group-writable bit, and
				// clearing it is what an operator can always do.
				assert.Contains(t, advice.remediation, "chmod g-w")
			} else {
				assert.Equal(t, implementationDefectAdvice(advice.fact), advice,
					"a cause no environment can produce must carry the shared defect remediation")
			}

			// Taking the strings from the function rather than from a literal
			// means this still holds after the wording is improved, while an
			// assembly that stopped carrying the advice fails.
			message := incompleteEnumerationError(unrelatedGID, incompleteVerdict(cause, "detail for "+cause.String())).Error()
			assert.Contains(t, message, advice.fact)
			assert.Contains(t, message, advice.remediation)
			assert.NotContains(t, message, "CGO_ENABLED")
		})
	}

	// A cause outside the defined range denies like an unrecognized one rather
	// than reaching for wording that fits some host condition.
	t.Run("cause outside the defined range", func(t *testing.T) {
		t.Parallel()

		advice := adviseIncompleteness(causeOutOfRange)
		assert.Equal(t, implementationDefectAdvice(advice.fact), advice)
		assert.NotContains(t, advice.remediation, "chmod g-w")
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
		// The fact names the file whose configuration produced this cause, so
		// the operator knows where to look before reading the detail.
		assert.Contains(t, message, nsswitchPath)
		assert.True(t, strings.HasSuffix(message, ErrGroupMemberEnumerationIncomplete.Error()))
	})
}
