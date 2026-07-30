//go:build test

package groupmembership

// WithPermissionCheckUIDPolicy sets the permission check UID policy for this
// GroupMembership instance only. When specified, it takes precedence over
// the process-wide default policy.
//
// Production code declares the binary-wide policy via
// SetProcessPermissionCheckUIDPolicy instead (see §5.5 of the architecture
// document); this option exists for tests only.
func WithPermissionCheckUIDPolicy(p PermissionCheckUIDPolicy) Option {
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
