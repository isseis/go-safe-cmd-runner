//go:build test

package groupmembership

import (
	"fmt"
	"log/slog"
)

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

// PermissionCheckUIDDeps is the public counterpart of permissionCheckUIDDeps,
// exposing the three dependency seams of resolvePermissionCheckUID to tests in
// other packages (e.g. cmd/record, cmd/verify, cmd/runner).
type PermissionCheckUIDDeps struct {
	// Getenv reads an environment variable.
	Getenv func(name string) string

	// VerifyUserExists reports whether a user with the given UID exists.
	// A nil error means the user exists.
	VerifyUserExists func(uid int) error

	// ReportAdoption records that the permission check UID was taken from
	// SUDO_UID. When nil, a default is used that emits to slog.Default().
	// The default uses a fresh reporter instance for every call, so it must
	// not be used to verify the once-per-process limit; that verification
	// belongs to the in-package tests.
	ReportAdoption func(policy PermissionCheckUIDPolicy, realUID, permissionCheckUID int)
}

// ResolvePermissionCheckUID resolves the permission check UID for the given
// policy, real UID, and dependency seams. It is an entry point for tests in
// other packages (e.g. cmd/record, cmd/verify) to exercise the unexported
// resolvePermissionCheckUID. Getenv and VerifyUserExists must be non-nil.
func ResolvePermissionCheckUID(policy PermissionCheckUIDPolicy, realUID int, deps PermissionCheckUIDDeps) (int, error) {
	if deps.Getenv == nil {
		panic("groupmembership: Getenv must not be nil")
	}
	if deps.VerifyUserExists == nil {
		panic("groupmembership: VerifyUserExists must not be nil")
	}
	reportAdoption := deps.ReportAdoption
	if reportAdoption == nil {
		reportAdoption = func(policy PermissionCheckUIDPolicy, realUID, permissionCheckUID int) {
			reporter := &sudoUIDAdoptionReporter{}
			reporter.report(slog.Default(), policy, realUID, permissionCheckUID)
		}
	}
	return resolvePermissionCheckUID(policy, realUID, permissionCheckUIDDeps{
		getenv:           deps.Getenv,
		verifyUserExists: deps.VerifyUserExists,
		reportAdoption:   reportAdoption,
	})
}
