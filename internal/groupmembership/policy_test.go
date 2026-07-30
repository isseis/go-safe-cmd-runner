package groupmembership

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPermissionCheckUIDPolicy_String verifies the string representation of
// each policy value and that out-of-range values are reported distinctly.
func TestPermissionCheckUIDPolicy_String(t *testing.T) {
	assert.Equal(t, "unset", PolicyUnset.String())
	assert.Equal(t, "real-uid-only", RealUIDOnly.String())
	assert.Equal(t, "sudo-uid-aware", SudoUIDAware.String())

	names := []string{PolicyUnset.String(), RealUIDOnly.String(), SudoUIDAware.String()}
	assert.Len(t, names, 3)
	assert.Equal(t, 3, len(map[string]struct{}{names[0]: {}, names[1]: {}, names[2]: {}}))

	assert.Equal(t, "unknown(99)", PermissionCheckUIDPolicy(99).String())
}

// TestSetProcessPermissionCheckUIDPolicy verifies each of the four outcomes
// of SetProcessPermissionCheckUIDPolicy documented in the architecture
// document (§6.2), as independently runnable subtests.
func TestSetProcessPermissionCheckUIDPolicy(t *testing.T) {
	t.Run("unset to RealUIDOnly succeeds", func(t *testing.T) {
		t.Cleanup(SwapProcessPermissionCheckUIDPolicy(PolicyUnset))

		err := SetProcessPermissionCheckUIDPolicy(RealUIDOnly)

		require.NoError(t, err)
		assert.Equal(t, RealUIDOnly, ProcessPermissionCheckUIDPolicy())
	})

	t.Run("re-setting the same value is a no-op", func(t *testing.T) {
		t.Cleanup(SwapProcessPermissionCheckUIDPolicy(RealUIDOnly))

		err := SetProcessPermissionCheckUIDPolicy(RealUIDOnly)

		require.NoError(t, err)
		assert.Equal(t, RealUIDOnly, ProcessPermissionCheckUIDPolicy())
	})

	t.Run("setting a different value conflicts", func(t *testing.T) {
		t.Cleanup(SwapProcessPermissionCheckUIDPolicy(RealUIDOnly))

		err := SetProcessPermissionCheckUIDPolicy(SudoUIDAware)

		require.Error(t, err)
		assert.ErrorIs(t, err, ErrPermissionCheckUIDPolicyConflict)
		assert.Equal(t, RealUIDOnly, ProcessPermissionCheckUIDPolicy())
	})

	t.Run("PolicyUnset and out-of-range values are invalid", func(t *testing.T) {
		t.Cleanup(SwapProcessPermissionCheckUIDPolicy(PolicyUnset))

		err := SetProcessPermissionCheckUIDPolicy(PolicyUnset)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPermissionCheckUIDPolicy)
		assert.Equal(t, PolicyUnset, ProcessPermissionCheckUIDPolicy())

		err = SetProcessPermissionCheckUIDPolicy(PermissionCheckUIDPolicy(99))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrInvalidPermissionCheckUIDPolicy)
		assert.Equal(t, PolicyUnset, ProcessPermissionCheckUIDPolicy())
	})
}

// TestSetProcessPermissionCheckUIDPolicy_Concurrent exercises the CAS retry
// path under concurrent SetProcessPermissionCheckUIDPolicy and
// ProcessPermissionCheckUIDPolicy calls, verifying there is no data race and
// that once a non-PolicyUnset value is observed it never changes.
func TestSetProcessPermissionCheckUIDPolicy_Concurrent(t *testing.T) {
	t.Cleanup(SwapProcessPermissionCheckUIDPolicy(PolicyUnset))

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make([]error, goroutines)

	wg.Add(goroutines + 1)
	for i := range goroutines {
		go func(i int) {
			defer wg.Done()
			p := RealUIDOnly
			if i%2 == 1 {
				p = SudoUIDAware
			}
			errs[i] = SetProcessPermissionCheckUIDPolicy(p)
		}(i)
	}

	observed := PolicyUnset
	go func() {
		defer wg.Done()
		for range 1000 {
			p := ProcessPermissionCheckUIDPolicy()
			if p != PolicyUnset {
				if observed == PolicyUnset {
					observed = p
				} else {
					assert.Equal(t, observed, p)
				}
			}
		}
	}()

	wg.Wait()

	final := ProcessPermissionCheckUIDPolicy()
	assert.Contains(t, []PermissionCheckUIDPolicy{RealUIDOnly, SudoUIDAware}, final)

	for _, err := range errs {
		if err != nil {
			assert.ErrorIs(t, err, ErrPermissionCheckUIDPolicyConflict)
		}
	}
}

// TestEffectivePermissionCheckUIDPolicy_Precedence verifies that an
// instance-level policy takes precedence over the process-wide default
// policy, and that the process-wide default is used otherwise.
func TestEffectivePermissionCheckUIDPolicy_Precedence(t *testing.T) {
	t.Cleanup(SwapProcessPermissionCheckUIDPolicy(SudoUIDAware))

	withInstancePolicy := New(WithPermissionCheckUIDPolicy(RealUIDOnly))
	assert.Equal(t, RealUIDOnly, withInstancePolicy.effectivePermissionCheckUIDPolicy())

	withoutInstancePolicy := New()
	assert.Equal(t, SudoUIDAware, withoutInstancePolicy.effectivePermissionCheckUIDPolicy())
}

// TestEffectivePermissionCheckUIDPolicy_FinalDefault verifies that
// RealUIDOnly is applied when neither the instance nor the process has a
// policy set, and that SudoUIDAware is never selected without declaration.
func TestEffectivePermissionCheckUIDPolicy_FinalDefault(t *testing.T) {
	t.Cleanup(SwapProcessPermissionCheckUIDPolicy(PolicyUnset))

	gm := New()

	assert.Equal(t, RealUIDOnly, gm.effectivePermissionCheckUIDPolicy())
}
