package groupmembership

import (
	"fmt"
	"strconv"
	"strings"
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
	require.Len(t, names, 3)
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

// permissionCheckUIDDepsRecorder counts invocations of the two dependency
// seams that the SudoUIDAware branch may exercise, so tests can assert on
// how often each is called.
type permissionCheckUIDDepsRecorder struct {
	verifyUserExistsCalls int
	reportAdoptionCalls   int
}

// newPermissionCheckUIDDeps returns a permissionCheckUIDDeps with safe
// defaults for the seams a test is not exercising: verifyUserExists always
// reports the user exists, and reportAdoption only counts its calls. Callers
// replace the getenv field with the value they want to resolve. The recorder
// is shared with the returned deps and must be inspected after the call.
func newPermissionCheckUIDDeps(rec *permissionCheckUIDDepsRecorder) permissionCheckUIDDeps {
	return permissionCheckUIDDeps{
		verifyUserExists: func(int) error {
			rec.verifyUserExistsCalls++
			return nil
		},
		reportAdoption: func(PermissionCheckUIDPolicy, int, int) {
			rec.reportAdoptionCalls++
		},
	}
}

// TestResolvePermissionCheckUID_RealUIDOnly verifies that under RealUIDOnly,
// the real UID is always returned unchanged, regardless of the value of
// SUDO_UID, and that neither the existence check nor the adoption record is
// invoked for any combination.
func TestResolvePermissionCheckUID_RealUIDOnly(t *testing.T) {
	sudoUIDValues := []string{
		"", "0", "1000", "4294967295", "abc", "-1", "4294967296", strings.Repeat("9", 300),
	}

	for _, realUID := range []int{0, 1000} {
		for _, sudoUID := range sudoUIDValues {
			t.Run(fmt.Sprintf("realUID=%d/sudoUID=%q", realUID, sudoUID), func(t *testing.T) {
				rec := &permissionCheckUIDDepsRecorder{}
				deps := newPermissionCheckUIDDeps(rec)
				deps.getenv = func(string) string { return sudoUID }

				uid, err := resolvePermissionCheckUID(RealUIDOnly, realUID, deps)

				require.NoError(t, err)
				assert.Equal(t, realUID, uid)
				assert.Zero(t, rec.verifyUserExistsCalls)
				assert.Zero(t, rec.reportAdoptionCalls)
			})
		}
	}
}

// TestResolvePermissionCheckUID_SudoUIDAware verifies every row of the
// SudoUIDAware decision table (0161 architecture document §3.5). The expected
// values are fixed here as a table rather than referencing
// resolvePermissionCheckUID's prior behavior, since that function may not
// survive future refactors.
func TestResolvePermissionCheckUID_SudoUIDAware(t *testing.T) {
	t.Run("realUID 0", func(t *testing.T) {
		tests := []struct {
			name       string
			sudoUID    string
			wantUID    int
			wantErrIs  error
			wantErrNil bool
		}{
			{name: "unset", sudoUID: "", wantUID: 0, wantErrNil: true},
			{name: "zero", sudoUID: "0", wantUID: 0, wantErrNil: true},
			{name: "valid", sudoUID: "1000", wantUID: 1000, wantErrNil: true},
			{name: "max uint32", sudoUID: "4294967295", wantUID: 4294967295, wantErrNil: true},
			{name: "negative", sudoUID: "-1", wantErrIs: ErrSudoUIDOutOfRange},
			{name: "exceeds uint32", sudoUID: "4294967296", wantErrIs: ErrSudoUIDOutOfRange},
			{name: "non-numeric", sudoUID: "abc", wantErrIs: strconv.ErrSyntax},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
				deps.getenv = func(string) string { return tt.sudoUID }

				uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

				if tt.wantErrNil {
					require.NoError(t, err)
					assert.Equal(t, tt.wantUID, uid)
					return
				}
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)
			})
		}
	})

	t.Run("realUID non-zero always returns realUID without error", func(t *testing.T) {
		for _, sudoUID := range []string{"", "2000", "abc", "-1"} {
			t.Run(fmt.Sprintf("sudoUID=%q", sudoUID), func(t *testing.T) {
				deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
				deps.getenv = func(string) string { return sudoUID }

				uid, err := resolvePermissionCheckUID(SudoUIDAware, 1000, deps)

				require.NoError(t, err)
				assert.Equal(t, 1000, uid)
			})
		}
	})
}

// TestResolvePermissionCheckUID_EnvAccess verifies that SUDO_UID is read only
// under SudoUIDAware, contrasted against RealUIDOnly under the same
// conditions (realUID 0, valid SUDO_UID) to rule out that the absence of a
// read is merely because realUID was non-zero (0161 architecture document
// §7.1).
func TestResolvePermissionCheckUID_EnvAccess(t *testing.T) {
	t.Run("RealUIDOnly never reads SUDO_UID", func(t *testing.T) {
		var calls int
		var names []string
		getenv := func(name string) string {
			calls++
			names = append(names, name)
			return "1000"
		}

		deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
		deps.getenv = getenv

		_, err := resolvePermissionCheckUID(RealUIDOnly, 0, deps)

		require.NoError(t, err)
		assert.Zero(t, calls)
		assert.Empty(t, names)
	})

	t.Run("SudoUIDAware reads SUDO_UID", func(t *testing.T) {
		var calls int
		var names []string
		getenv := func(name string) string {
			calls++
			names = append(names, name)
			return "1000"
		}

		deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
		deps.getenv = getenv

		_, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

		require.NoError(t, err)
		assert.GreaterOrEqual(t, calls, 1)
		for _, name := range names {
			assert.Equal(t, "SUDO_UID", name)
		}
	})
}
