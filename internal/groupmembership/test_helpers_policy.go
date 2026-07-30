//go:build test

package groupmembership

import "fmt"

// WithPermissionCheckUIDPolicy sets the permission check UID policy for this
// GroupMembership instance only. When specified, it takes precedence over
// the process-wide default policy.
//
// Production code declares the binary-wide policy via
// SetProcessPermissionCheckUIDPolicy instead (see §5.5 of the architecture
// document); this option exists for tests only.
//
// p must be PolicyUnset, RealUIDOnly, or SudoUIDAware; any other value
// (e.g. an invalid cast such as PermissionCheckUIDPolicy(99)) panics, since
// effectivePermissionCheckUIDPolicy trusts gm.policy to hold only one of
// these values.
func WithPermissionCheckUIDPolicy(p PermissionCheckUIDPolicy) Option {
	if p != PolicyUnset && p != RealUIDOnly && p != SudoUIDAware {
		panic(fmt.Sprintf("groupmembership: invalid permission check UID policy: %s", p))
	}
	return func(gm *GroupMembership) {
		gm.policy = p
	}
}

// SwapProcessPermissionCheckUIDPolicy overwrites the process-wide default
// permission check UID policy with p, bypassing the validation performed by
// SetProcessPermissionCheckUIDPolicy, and returns a function that restores
// the value that was in effect before the call.
//
// Callers should use it as t.Cleanup(SwapProcessPermissionCheckUIDPolicy(...)).
// Tests that call this function must not call t.Parallel(), since it
// mutates state shared across the whole process.
func SwapProcessPermissionCheckUIDPolicy(p PermissionCheckUIDPolicy) (restore func()) {
	previous := PermissionCheckUIDPolicy(processPermissionCheckUIDPolicy.Swap(int32(p)))
	return func() {
		processPermissionCheckUIDPolicy.Store(int32(previous))
	}
}

// EffectivePermissionCheckUIDPolicy returns the effective permission check
// UID policy for gm. It is an entry point for tests in other packages to
// inspect the effective policy.
func (gm *GroupMembership) EffectivePermissionCheckUIDPolicy() PermissionCheckUIDPolicy {
	return gm.effectivePermissionCheckUIDPolicy()
}

// ResolvePermissionCheckUID resolves the permission check UID for the given
// policy, real UID, and environment variable getter. It is an entry point
// for tests in other packages (e.g. cmd/record, cmd/verify) to exercise the
// unexported resolvePermissionCheckUID.
func ResolvePermissionCheckUID(policy PermissionCheckUIDPolicy, realUID int, getenv func(string) string) (int, error) {
	return resolvePermissionCheckUID(policy, realUID, getenv)
}
