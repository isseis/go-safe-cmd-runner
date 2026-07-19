//go:build test

package groupmembership

// newWithEnumerator creates a GroupMembership whose enumeration function is replaced,
// for tests that need to inject enumeration successes or failures deterministically.
func newWithEnumerator(fn func(gid uint32) ([]string, error)) *GroupMembership {
	gm := New()
	gm.enumerateGroupMembers = fn
	return gm
}
