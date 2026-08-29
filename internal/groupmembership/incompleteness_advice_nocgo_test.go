//go:build !cgo && test

package groupmembership

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdviseIncompleteness_NoCGO fixes the wording this build offers for the
// causes an environment can actually produce. The strings are the ones 0168
// settled on, so an edit that reworded them would be caught here. Each case
// is checked both on the advice itself and on the message the denial carries,
// so a change that stopped routing the advice into the message would fail too.
func TestAdviseIncompleteness_NoCGO(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		cause           incompletenessCause
		detail          string
		wantContains    []string
		wantNotContains []string
	}{
		{
			name:   "unsupported platform",
			cause:  causeUnsupportedPlatform,
			detail: "goos=darwin",
			wantContains: []string{
				"this build cannot enumerate all members of a group on this platform",
				"rebuild with CGO_ENABLED=1 so that group members are resolved through the platform's own user database via libc",
			},
			wantNotContains: []string{"/etc/nsswitch.conf"},
		},
		{
			name:   "nss sources",
			cause:  causeNSSSources,
			detail: "group: files sss",
			wantContains: []string{
				"/etc/nsswitch.conf names a user database source this build cannot consult, or could not be read",
				"check the passwd and group lines of /etc/nsswitch.conf, then rebuild with CGO_ENABLED=1 so that the configured sources are consulted",
			},
		},
		{
			name:   "malformed line",
			cause:  causeMalformedLine,
			detail: "1 line skipped, first at /etc/group:12",
			wantContains: []string{
				"a line of the user database files could not be parsed and was skipped, so the members listed there are unknown",
				// Deleting the offending line is the obvious response and the
				// wrong one for a NIS compatibility entry, where the line is
				// correct and only this build cannot follow it.
				"check the reported line: correct it if its format is wrong, or, if it is a NIS compatibility entry (a line starting with + or -), rebuild with CGO_ENABLED=1",
			},
			wantNotContains: []string{"/etc/nsswitch.conf"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			advice := adviseIncompleteness(tt.cause)
			combined := advice.fact + "\n" + advice.remediation

			err := incompleteEnumerationError(unrelatedGID, incompleteVerdict(tt.cause, tt.detail))
			require.ErrorIs(t, err, ErrGroupMemberEnumerationIncomplete)
			message := err.Error()

			for _, want := range tt.wantContains {
				assert.Contains(t, combined, want)
				assert.Contains(t, message, want)
			}
			for _, notWant := range tt.wantNotContains {
				assert.NotContains(t, combined, notWant)
				assert.NotContains(t, message, notWant)
			}
			assert.Contains(t, message, "user_database_source="+userDatabaseSource)
			assert.Contains(t, message, "detail="+tt.detail)
		})
	}
}
