//go:build !cgo && test

package groupmembership

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdviseIncompleteness_NoCGO checks the properties this build's advice
// must have, mirroring the cgo test. It does not copy the sentences: that the
// move preserved them character for character is a one-off question about this
// change, answered by the exact-string search the task records, not something
// to freeze here where every later improvement would fail for no defect.
//
// What must hold, and is checked below:
//
//   - Every cause a host can produce points at building with cgo. That is the
//     one remediation this build has and the cgo build does not.
//   - A cause only an implementation error can produce says so instead.
//   - Each cause's advice names the thing that produced it, and stays silent
//     about the ones it did not: an operator sent to /etc/nsswitch.conf over a
//     malformed /etc/group line reads the wrong file.
//   - Each cause's advice differs from every other cause's.
//   - The advice reaches the message the operator sees.
func TestAdviseIncompleteness_NoCGO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		cause  incompletenessCause
		detail string
		// namesNsswitch says whether the advice should point the operator at
		// /etc/nsswitch.conf. Only the cause that comes from reading it should.
		namesNsswitch bool
		// wantRemediation are terms the remediation has to carry because they
		// name a thing rather than a way of putting it.
		wantRemediation []string
	}{
		{
			name:   "unsupported platform",
			cause:  causeUnsupportedPlatform,
			detail: "goos=darwin",
		},
		{
			name:            "nss sources",
			cause:           causeNSSSources,
			detail:          "group: files sss",
			namesNsswitch:   true,
			wantRemediation: []string{"passwd", "group"},
		},
		{
			name:   "malformed line",
			cause:  causeMalformedLine,
			detail: "1 line skipped, first at /etc/group:12",
			// Deleting the offending line is the obvious response and the
			// wrong one for a NIS compatibility entry, where the line is
			// correct and only this build cannot follow it. The remediation
			// has to say so.
			wantRemediation: []string{"NIS"},
		},
	}

	// The rows above are the causes a host condition produces; causeUnspecified
	// and an out-of-range value are covered through incompleteEnumerationError
	// by manager_test.go. The loop below enforces that between them they cover
	// every defined cause.
	covered := map[incompletenessCause]struct{}{causeUnspecified: {}}
	for _, tt := range tests {
		covered[tt.cause] = struct{}{}
	}
	for _, cause := range allIncompletenessCauses {
		_, ok := covered[cause]
		require.True(t, ok, "cause %s is classified by adviseIncompleteness but asserted nowhere", cause)
	}

	seen := make(map[incompletenessAdvice]incompletenessCause, len(allIncompletenessCauses))
	for _, cause := range allIncompletenessCauses {
		advice := adviseIncompleteness(cause)
		if other, duplicate := seen[advice]; duplicate {
			t.Errorf("causes %s and %s give identical advice; one of them is reaching the other's branch", other, cause)
		}
		seen[advice] = cause
	}

	t.Run("cause no environment can produce", func(t *testing.T) {
		t.Parallel()

		advice := adviseIncompleteness(causeUnspecified)
		assert.Equal(t, implementationDefectAdvice(advice.fact), advice)
		assert.NotContains(t, advice.remediation, "CGO_ENABLED")
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			advice := adviseIncompleteness(tt.cause)
			assert.NotEmpty(t, advice.fact)

			// This build cannot consult what libc would, so for every cause a
			// host can produce, building with cgo is the way out.
			assert.Contains(t, advice.remediation, "CGO_ENABLED=1")
			for _, want := range tt.wantRemediation {
				assert.Contains(t, advice.remediation, want)
			}

			combined := advice.fact + "\n" + advice.remediation
			if tt.namesNsswitch {
				assert.Contains(t, combined, nsswitchPath)
			} else {
				assert.NotContains(t, combined, nsswitchPath, "this cause did not come from reading that file")
			}

			// Taking the strings from the function rather than from a literal
			// means this still holds after the wording is improved, while an
			// assembly that stopped carrying the advice fails.
			err := incompleteEnumerationError(unrelatedGID, incompleteVerdict(tt.cause, tt.detail))
			require.ErrorIs(t, err, ErrGroupMemberEnumerationIncomplete)

			message := err.Error()
			assert.Contains(t, message, advice.fact)
			assert.Contains(t, message, advice.remediation)
			assert.Contains(t, message, "user_database_source="+userDatabaseSource)
			assert.Contains(t, message, "cause="+tt.cause.String())
			assert.Contains(t, message, "detail="+tt.detail)
		})
	}
}
