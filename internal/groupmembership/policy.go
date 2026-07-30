package groupmembership

import (
	"errors"
	"fmt"
	"sync/atomic"
)

// PermissionCheckUIDPolicy determines how the base UID used by the
// read-safety check (CanCurrentUserSafelyReadFile) is decided.
type PermissionCheckUIDPolicy int32

const (
	// PolicyUnset is the zero value and means that no policy has been
	// specified at this level. It is never selected as the effective
	// policy; it defers resolution to the next level (see §3.4 of the
	// architecture document).
	PolicyUnset PermissionCheckUIDPolicy = iota

	// RealUIDOnly always uses the process's real UID as the base UID.
	// SUDO_UID is never read under this policy.
	RealUIDOnly

	// SudoUIDAware uses the value of the SUDO_UID environment variable as
	// the base UID when the real UID is 0.
	//
	// SUDO_UID is only checked for numeric validity; it is not verified to
	// correspond to a real user. This policy therefore accepts that anyone
	// able to start the binary as root can specify the base UID at will.
	// It is only selected when explicitly declared.
	SudoUIDAware
)

// finalDefaultPermissionCheckUIDPolicy is the policy applied when neither
// the instance nor the process has a policy set.
const finalDefaultPermissionCheckUIDPolicy = RealUIDOnly

// String returns the policy name, used in error messages and log output.
func (p PermissionCheckUIDPolicy) String() string {
	switch p {
	case PolicyUnset:
		return "unset"
	case RealUIDOnly:
		return "real-uid-only"
	case SudoUIDAware:
		return "sudo-uid-aware"
	default:
		return fmt.Sprintf("unknown(%d)", int32(p))
	}
}

// ErrPermissionCheckUIDPolicyConflict is returned when attempting to set the
// process-wide permission check UID policy to a value different from the one
// already set.
var ErrPermissionCheckUIDPolicyConflict = errors.New("process-wide permission check UID policy is already set to a different value")

// ErrInvalidPermissionCheckUIDPolicy is returned when a value that cannot be
// set as the process-wide permission check UID policy (PolicyUnset, or a
// value that is neither RealUIDOnly nor SudoUIDAware) is passed.
var ErrInvalidPermissionCheckUIDPolicy = errors.New("invalid permission check UID policy")

// Option configures a GroupMembership instance at construction time.
type Option func(*GroupMembership)

// processPermissionCheckUIDPolicy holds the process-wide default permission
// check UID policy. Its zero value equals PolicyUnset.
var processPermissionCheckUIDPolicy atomic.Int32

// SetProcessPermissionCheckUIDPolicy sets the process-wide permission check
// UID policy. Each binary's main package calls this exactly once from init.
//
// If the same value is already set, this is a no-op and returns nil. If a
// different value is already set, it returns ErrPermissionCheckUIDPolicyConflict
// and leaves the stored value unchanged. If p is PolicyUnset or is neither
// RealUIDOnly nor SudoUIDAware (e.g. an invalid cast such as
// PermissionCheckUIDPolicy(99)), it returns ErrInvalidPermissionCheckUIDPolicy.
func SetProcessPermissionCheckUIDPolicy(p PermissionCheckUIDPolicy) error {
	if p != RealUIDOnly && p != SudoUIDAware {
		return fmt.Errorf("%w: %s", ErrInvalidPermissionCheckUIDPolicy, p)
	}

	for {
		current := PermissionCheckUIDPolicy(processPermissionCheckUIDPolicy.Load())
		if current == p {
			return nil
		}
		if current != PolicyUnset {
			return fmt.Errorf("%w: current=%s, requested=%s", ErrPermissionCheckUIDPolicyConflict, current, p)
		}
		if processPermissionCheckUIDPolicy.CompareAndSwap(int32(PolicyUnset), int32(p)) {
			return nil
		}
		// Another goroutine changed the value concurrently; re-evaluate.
	}
}

// ProcessPermissionCheckUIDPolicy returns the current process-wide default
// permission check UID policy. It returns PolicyUnset if unset.
func ProcessPermissionCheckUIDPolicy() PermissionCheckUIDPolicy {
	return PermissionCheckUIDPolicy(processPermissionCheckUIDPolicy.Load())
}

// effectivePermissionCheckUIDPolicy resolves the effective policy for this
// GroupMembership instance, following the precedence order: instance policy,
// then process-wide default policy, then the final default policy.
//
// This assumes that every level holds only RealUIDOnly, SudoUIDAware, or
// PolicyUnset, so no panic or default case for unexpected values is added
// here. For the process-wide value this is enforced by
// SetProcessPermissionCheckUIDPolicy; the instance value is only ever set by
// the test-only WithPermissionCheckUIDPolicy option, which is trusted not to
// pass an invalid value.
func (gm *GroupMembership) effectivePermissionCheckUIDPolicy() PermissionCheckUIDPolicy {
	if gm.policy != PolicyUnset {
		return gm.policy
	}
	if p := ProcessPermissionCheckUIDPolicy(); p != PolicyUnset {
		return p
	}
	return finalDefaultPermissionCheckUIDPolicy
}
