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

// resetNsswitchClassification clears the process-wide classification and the
// reporter that shares its lifetime, so that a test can observe the first
// classification of the process -- including the reporter's one emission per
// process, which an earlier test would otherwise have consumed. It clears
// them again afterwards so that a value one test planted cannot be read by
// the next.
//
// Callers must not run in parallel with each other, nor with any test that
// enumerates: the classification is process-wide, and production settles it
// once at startup and only reads it afterwards.
func resetNsswitchClassification(t *testing.T) {
	t.Helper()

	clearNsswitchClassification()
	t.Cleanup(restoreStartupNsswitchClassification)
}

// clearNsswitchClassification returns the process-wide classification and its
// reporter to the state they have before startup settles them.
func clearNsswitchClassification() {
	nsswitchVerdictValue = completenessVerdict{}
	processNSSCompletenessReporter.reported.Store(false)
}

// startupNsswitchClassification is the verdict that settling the
// classification once, the way a binary does at startup, left behind.
// Restoring from this snapshot rather than classifying again keeps a cleanup
// from reading /etc/nsswitch.conf a second time and from emitting the
// startup warning into whatever logger a later test has installed.
var startupNsswitchClassification completenessVerdict

// settleStartupNsswitchClassification settles the classification for the test
// binary the way record, verify and runner do at startup, and remembers the
// verdict it produced so that restoreStartupNsswitchClassification can put it
// back. TestMain calls it before any test runs, so the settle it performs
// here is always the first: the reporter's one-emission-per-process latch is
// therefore always left tripped, and restoreStartupNsswitchClassification can
// restore that fact as a constant rather than a second snapshot field.
func settleStartupNsswitchClassification() {
	precomputeEnumerationEnvironment()
	startupNsswitchClassification = nsswitchVerdictValue
}

// restoreStartupNsswitchClassification puts the process back into the state
// TestMain left it in. A test that cleared or planted a verdict must restore
// it, or the tests that follow would enumerate against an unstated
// classification and deny.
func restoreStartupNsswitchClassification() {
	nsswitchVerdictValue = startupNsswitchClassification
	processNSSCompletenessReporter.reported.Store(true)
}

// useNsswitchVerdict fixes the completeness verdict for this process for the
// duration of one test and clears it again afterwards, so that a test can
// drive a whole enumeration from a chosen verdict without depending on the
// host's own /etc/nsswitch.conf.
//
// Callers must not run in parallel with each other: the verdict it plants is
// process-wide.
//
// The planted verdict is never reported: precomputeEnumerationEnvironment is
// not what put it there, and it leaves an already settled verdict alone. A
// test that asserts on the startup warning must instead call
// resetNsswitchClassification and let the host's own classification settle.
//
// Only the cgo build will ever call this: the non-cgo build takes the verdict
// as an argument to enumerateFromFiles, which has to combine it with the
// malformed lines it saw. Hence the suppression below, which is permanent.
//
//nolint:unused // called only from //go:build cgo && test files
func useNsswitchVerdict(t *testing.T, v completenessVerdict) {
	t.Helper()

	resetNsswitchClassification(t)
	nsswitchVerdictValue = v
}
