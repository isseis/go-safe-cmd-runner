//go:build test

package groupmembership

import "testing"

// newWithEnumerator creates a GroupMembership whose enumeration function is replaced,
// for tests that need to inject enumeration successes or failures deterministically.
func newWithEnumerator(fn func(gid uint32) (groupEnumeration, error)) *GroupMembership {
	gm := New()
	gm.enumerateGroupMembers = fn
	return gm
}

// newWithFixedEnumeration creates a GroupMembership whose enumeration always
// succeeds with the given members and completeness verdict, for tests that
// vary the verdict while holding the member set fixed.
func newWithFixedEnumeration(members []string, verdict completenessVerdict) *GroupMembership {
	return newWithEnumerator(func(uint32) (groupEnumeration, error) {
		return groupEnumeration{members: members, verdict: verdict}, nil
	})
}

// resetNsswitchClassification clears the process-wide classification latch
// and the reporter that shares its lifetime, so that a test can observe the
// first classification of the process -- including the reporter's one
// emission per process, which an earlier test would otherwise have consumed.
// It clears them again afterwards so that a value one test planted cannot be
// read by the next.
//
// Callers must not run in parallel with each other: the latch is
// process-wide, and clearing it mid-run would let another test observe a
// classification that is being settled a second time.
func resetNsswitchClassification(t *testing.T) {
	t.Helper()

	reset := func() {
		nsswitchVerdictMu.Lock()
		defer nsswitchVerdictMu.Unlock()
		nsswitchVerdictResolved = false
		nsswitchVerdictValue = completenessVerdict{}
		processNSSCompletenessReporter.reported.Store(false)
	}

	reset()
	t.Cleanup(reset)
}

// useNsswitchVerdict fixes the completeness verdict for this process for the
// duration of one test and clears it again afterwards, so that a test can
// drive a whole enumeration from a chosen verdict without depending on the
// host's own /etc/nsswitch.conf.
//
// Callers must not run in parallel with each other: the verdict it plants is
// process-wide.
//
// Its callers are the cgo build's tests, which is why the linter's
// CGO_ENABLED=0 run sees no caller. The helper stays here rather than in a
// cgo-tagged file so that both builds' tests fix the verdict the same way.
//
//nolint:unused // only the cgo build's tests call this today
func useNsswitchVerdict(t *testing.T, v completenessVerdict) {
	t.Helper()

	resetNsswitchClassification(t)

	nsswitchVerdictMu.Lock()
	defer nsswitchVerdictMu.Unlock()
	nsswitchVerdictValue = v
	nsswitchVerdictResolved = true
}
