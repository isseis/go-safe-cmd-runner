//go:build test

package groupmembership

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
