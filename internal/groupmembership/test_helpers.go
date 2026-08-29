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
