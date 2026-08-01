package groupmembership

import (
	"errors"
	"fmt"
	"log/slog"
	"os/user"
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
// defaults for every seam a test is not exercising: getenv reports SUDO_UID
// as unset, verifyUserExists always reports the user exists, and
// reportAdoption only counts its calls. Callers replace the getenv field with
// the value they want to resolve; the default is present so that a caller
// that forgets to get an unset SUDO_UID rather than a nil-function panic.
// The recorder is shared with the returned deps and must be inspected after
// the call.
func newPermissionCheckUIDDeps(rec *permissionCheckUIDDepsRecorder) permissionCheckUIDDeps {
	return permissionCheckUIDDeps{
		getenv: func(string) string { return "" },
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
// SudoUIDAware decision table (0161 architecture document §3.5). Rows 1-7
// (real UID 0) are covered by the "realUID 0" subtest table, and row 8 (real
// UID non-zero) by the "realUID non-zero" subtest. Each row asserts the
// resolved base UID, the error, whether the existence check was invoked, and
// whether the adoption record was emitted.
func TestResolvePermissionCheckUID_SudoUIDAware(t *testing.T) {
	t.Run("realUID 0", func(t *testing.T) {
		lookupFailedErr := errors.New("injected lookup failure")
		tests := []struct {
			name            string
			sudoUID         string
			verifyErr       error // nil means the user exists
			wantUID         int
			wantErrIs       error
			wantErrNil      bool
			wantVerifyCalls int
			wantRecorded    bool
		}{
			{name: "unset", sudoUID: "", wantUID: 0, wantErrNil: true, wantVerifyCalls: 0, wantRecorded: false},
			{name: "zero and user exists", sudoUID: "0", wantUID: 0, wantErrNil: true, wantVerifyCalls: 1, wantRecorded: false},
			{name: "zero and user missing", sudoUID: "0", verifyErr: user.UnknownUserIdError(0), wantErrIs: ErrSudoUIDUserNotFound, wantVerifyCalls: 1, wantRecorded: false},
			{name: "valid and user exists", sudoUID: "1000", wantUID: 1000, wantErrNil: true, wantVerifyCalls: 1, wantRecorded: true},
			{name: "max uint32 and user exists", sudoUID: "4294967295", wantUID: 4294967295, wantErrNil: true, wantVerifyCalls: 1, wantRecorded: true},
			{name: "valid and user missing", sudoUID: "1000", verifyErr: user.UnknownUserIdError(1000), wantErrIs: ErrSudoUIDUserNotFound, wantVerifyCalls: 1, wantRecorded: false},
			{name: "valid and lookup failed", sudoUID: "1000", verifyErr: lookupFailedErr, wantErrIs: ErrSudoUIDUserLookupFailed, wantVerifyCalls: 1, wantRecorded: false},
			{name: "zero and lookup failed", sudoUID: "0", verifyErr: lookupFailedErr, wantErrIs: ErrSudoUIDUserLookupFailed, wantVerifyCalls: 1, wantRecorded: false},
			{name: "negative", sudoUID: "-1", wantErrIs: ErrSudoUIDOutOfRange, wantVerifyCalls: 0, wantRecorded: false},
			{name: "exceeds uint32", sudoUID: "4294967296", wantErrIs: ErrSudoUIDOutOfRange, wantVerifyCalls: 0, wantRecorded: false},
			{name: "non-numeric", sudoUID: "abc", wantErrIs: strconv.ErrSyntax, wantVerifyCalls: 0, wantRecorded: false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := &permissionCheckUIDDepsRecorder{}
				deps := newPermissionCheckUIDDeps(rec)
				deps.getenv = func(string) string { return tt.sudoUID }
				if tt.verifyErr != nil {
					deps.verifyUserExists = func(int) error {
						rec.verifyUserExistsCalls++
						return tt.verifyErr
					}
				}

				uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

				if tt.wantErrNil {
					require.NoError(t, err)
					assert.Equal(t, tt.wantUID, uid)
				} else {
					require.Error(t, err)
					assert.ErrorIs(t, err, tt.wantErrIs)
					assert.Zero(t, uid)
				}
				assert.Equal(t, tt.wantVerifyCalls, rec.verifyUserExistsCalls)
				if tt.wantRecorded {
					assert.Equal(t, 1, rec.reportAdoptionCalls)
				} else {
					assert.Zero(t, rec.reportAdoptionCalls)
				}
			})
		}
	})

	t.Run("realUID non-zero always returns realUID without error", func(t *testing.T) {
		for _, sudoUID := range []string{"", "2000", "abc", "-1"} {
			t.Run(fmt.Sprintf("sudoUID=%q", sudoUID), func(t *testing.T) {
				rec := &permissionCheckUIDDepsRecorder{}
				deps := newPermissionCheckUIDDeps(rec)
				deps.getenv = func(string) string { return sudoUID }

				uid, err := resolvePermissionCheckUID(SudoUIDAware, 1000, deps)

				require.NoError(t, err)
				assert.Equal(t, 1000, uid)
				assert.Zero(t, rec.verifyUserExistsCalls)
				assert.Zero(t, rec.reportAdoptionCalls)
			})
		}
	})
}

// TestResolvePermissionCheckUID_UserNotFound verifies that when the
// existence check reports the user as absent (user.UnknownUserIdError), no
// base UID is returned and the error matches both ErrSudoUIDUserNotFound and
// the original os/user error, which is preserved through %w.
func TestResolvePermissionCheckUID_UserNotFound(t *testing.T) {
	missingErr := user.UnknownUserIdError(1000)
	deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
	deps.getenv = func(string) string { return "1000" }
	deps.verifyUserExists = func(int) error { return missingErr }

	uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

	assert.Zero(t, uid)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSudoUIDUserNotFound)
	assert.ErrorIs(t, err, missingErr)
}

// TestResolvePermissionCheckUID_UserLookupFailed verifies that when the
// existence check itself fails, no base UID is returned and the error matches
// both ErrSudoUIDUserLookupFailed and the exact error value that was passed.
func TestResolvePermissionCheckUID_UserLookupFailed(t *testing.T) {
	lookupErr := errors.New("injected lookup failure")
	deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
	deps.getenv = func(string) string { return "1000" }
	deps.verifyUserExists = func(int) error { return lookupErr }

	uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

	assert.Zero(t, uid)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSudoUIDUserLookupFailed)
	assert.ErrorIs(t, err, lookupErr)
}

// TestResolvePermissionCheckUID_ErrorMessageContent pins the building blocks
// of the two error messages (architecture document §4.3): the raw SUDO_UID
// string, the user_database_source= attribute with the constant's value, and
// the remediation phrase specific to each error. The SUDO_UID input "01000"
// is used because its string form differs from the parsed value (1000), so
// the assertion on "01000" also pins the rule that the raw environment
// string, not the parsed UID, appears in the message.
func TestResolvePermissionCheckUID_ErrorMessageContent(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
		deps.getenv = func(string) string { return "01000" }
		deps.verifyUserExists = func(int) error { return user.UnknownUserIdError(1000) }

		_, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "SUDO_UID")
		assert.Contains(t, err.Error(), "01000")
		assert.Contains(t, err.Error(), "user_database_source="+userDatabaseSource)
		assert.Contains(t, err.Error(), "re-run from an interactive sudo session")
	})

	t.Run("lookup failed", func(t *testing.T) {
		deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
		deps.getenv = func(string) string { return "01000" }
		deps.verifyUserExists = func(int) error { return errors.New("injected lookup failure") }

		_, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "SUDO_UID")
		assert.Contains(t, err.Error(), "01000")
		assert.Contains(t, err.Error(), "user_database_source="+userDatabaseSource)
		assert.Contains(t, err.Error(), "check the state of the user database")
	})
}

// TestResolvePermissionCheckUID_SentinelErrorsAreDistinct verifies that the
// two new sentinel errors are distinguishable from the pre-existing errors of
// the SudoUIDAware branch (ErrSudoUIDOutOfRange, strconv.ErrSyntax) and from
// each other via errors.Is.
func TestResolvePermissionCheckUID_SentinelErrorsAreDistinct(t *testing.T) {
	newDeps := func(verifyErr error) permissionCheckUIDDeps {
		deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
		deps.getenv = func(string) string { return "1000" }
		deps.verifyUserExists = func(int) error { return verifyErr }
		return deps
	}

	_, notFoundErr := resolvePermissionCheckUID(SudoUIDAware, 0, newDeps(user.UnknownUserIdError(1000)))
	_, lookupFailedErr := resolvePermissionCheckUID(SudoUIDAware, 0, newDeps(errors.New("injected lookup failure")))
	require.Error(t, notFoundErr)
	require.Error(t, lookupFailedErr)

	assert.False(t, errors.Is(notFoundErr, ErrSudoUIDOutOfRange))
	assert.False(t, errors.Is(notFoundErr, strconv.ErrSyntax))
	assert.False(t, errors.Is(notFoundErr, ErrSudoUIDUserLookupFailed))
	assert.False(t, errors.Is(lookupFailedErr, ErrSudoUIDUserNotFound))
}

// TestResolvePermissionCheckUID_ExistenceCheckNotInvoked verifies that the
// existence check is never invoked for numerically invalid SUDO_UID values
// (which return the same errors as before 0161) and when the real UID is
// non-zero (which returns the real UID).
func TestResolvePermissionCheckUID_ExistenceCheckNotInvoked(t *testing.T) {
	t.Run("numerically invalid values", func(t *testing.T) {
		tests := []struct {
			name      string
			sudoUID   string
			wantErrIs error
		}{
			{name: "non-numeric", sudoUID: "abc", wantErrIs: strconv.ErrSyntax},
			{name: "negative", sudoUID: "-1", wantErrIs: ErrSudoUIDOutOfRange},
			{name: "exceeds uint32", sudoUID: "4294967296", wantErrIs: ErrSudoUIDOutOfRange},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				rec := &permissionCheckUIDDepsRecorder{}
				deps := newPermissionCheckUIDDeps(rec)
				deps.getenv = func(string) string { return tt.sudoUID }

				uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

				assert.Zero(t, uid)
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)
				assert.Zero(t, rec.verifyUserExistsCalls)
				assert.Zero(t, rec.reportAdoptionCalls)
			})
		}
	})

	t.Run("real UID non-zero", func(t *testing.T) {
		rec := &permissionCheckUIDDepsRecorder{}
		deps := newPermissionCheckUIDDeps(rec)
		deps.getenv = func(string) string { return "1000" }

		uid, err := resolvePermissionCheckUID(SudoUIDAware, 1000, deps)

		require.NoError(t, err)
		assert.Equal(t, 1000, uid)
		assert.Zero(t, rec.verifyUserExistsCalls)
		assert.Zero(t, rec.reportAdoptionCalls)
	})
}

// TestResolvePermissionCheckUID_ExistenceCheckSkippedUnderRealUIDOnly
// contrasts RealUIDOnly with SudoUIDAware under identical conditions (real
// UID 0, valid SUDO_UID) so that the absence of the existence check under
// RealUIDOnly cannot be mistaken for the real UID being non-zero.
func TestResolvePermissionCheckUID_ExistenceCheckSkippedUnderRealUIDOnly(t *testing.T) {
	t.Run("RealUIDOnly verifies neither existence nor record", func(t *testing.T) {
		rec := &permissionCheckUIDDepsRecorder{}
		deps := newPermissionCheckUIDDeps(rec)
		deps.getenv = func(string) string { return "1000" }

		uid, err := resolvePermissionCheckUID(RealUIDOnly, 0, deps)

		require.NoError(t, err)
		assert.Zero(t, uid)
		assert.Zero(t, rec.verifyUserExistsCalls)
		assert.Zero(t, rec.reportAdoptionCalls)
	})

	t.Run("SudoUIDAware verifies and records under the same conditions", func(t *testing.T) {
		rec := &permissionCheckUIDDepsRecorder{}
		deps := newPermissionCheckUIDDeps(rec)
		deps.getenv = func(string) string { return "1000" }

		uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

		require.NoError(t, err)
		assert.Equal(t, 1000, uid)
		assert.Equal(t, 1, rec.verifyUserExistsCalls)
		assert.Equal(t, 1, rec.reportAdoptionCalls)
	})
}

// TestResolvePermissionCheckUID_AdoptionRecordConditions verifies that the
// adoption record is emitted only when SUDO_UID is a valid value different
// from the real UID and the existence check succeeds.
func TestResolvePermissionCheckUID_AdoptionRecordConditions(t *testing.T) {
	tests := []struct {
		name       string
		policy     PermissionCheckUIDPolicy
		realUID    int
		sudoUID    string
		verifyErr  error
		wantRecord bool
	}{
		{name: "adopted when SUDO_UID differs from real UID", policy: SudoUIDAware, realUID: 0, sudoUID: "1000", wantRecord: true},
		{name: "not recorded when SUDO_UID is unset", policy: SudoUIDAware, realUID: 0, sudoUID: "", wantRecord: false},
		{name: "not recorded when SUDO_UID equals real UID", policy: SudoUIDAware, realUID: 0, sudoUID: "0", wantRecord: false},
		{name: "not recorded when existence check fails", policy: SudoUIDAware, realUID: 0, sudoUID: "1000", verifyErr: user.UnknownUserIdError(1000), wantRecord: false},
		{name: "not recorded under RealUIDOnly", policy: RealUIDOnly, realUID: 0, sudoUID: "1000", wantRecord: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &permissionCheckUIDDepsRecorder{}
			deps := newPermissionCheckUIDDeps(rec)
			deps.getenv = func(string) string { return tt.sudoUID }
			if tt.verifyErr != nil {
				deps.verifyUserExists = func(int) error { return tt.verifyErr }
			}

			_, err := resolvePermissionCheckUID(tt.policy, tt.realUID, deps)

			if tt.verifyErr != nil {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			if tt.wantRecord {
				assert.Equal(t, 1, rec.reportAdoptionCalls)
			} else {
				assert.Zero(t, rec.reportAdoptionCalls)
			}
		})
	}
}

// TestResolvePermissionCheckUID_ReportsAdoptionOnlyOncePerReporter verifies
// that binding one reporter instance into reportAdoption and resolving three
// times still yields the correct base UID every time and exactly one record
// (architecture document §7.1).
func TestResolvePermissionCheckUID_ReportsAdoptionOnlyOncePerReporter(t *testing.T) {
	handler := newLogCaptureHandler(nil)
	logger := slog.New(handler)
	reporter := &sudoUIDAdoptionReporter{}
	deps := permissionCheckUIDDeps{
		getenv:           func(string) string { return "1000" },
		verifyUserExists: func(int) error { return nil },
		reportAdoption: func(policy PermissionCheckUIDPolicy, realUID, permissionCheckUID int) {
			reporter.report(logger, policy, realUID, permissionCheckUID)
		},
	}

	for range 3 {
		uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)
		require.NoError(t, err)
		assert.Equal(t, 1000, uid)
	}

	assert.Len(t, handler.Records(), 1)
}

// TestResolvePermissionCheckUID_FailsClosedOnExistenceFailure verifies that
// on either existence failure path the resolved UID is exactly 0 -- neither
// the SUDO_UID value nor the real UID -- so that no usable base UID leaks
// through. The decision-table rows already assert that an error is returned;
// this test additionally fixes that the UID itself is the zero value.
func TestResolvePermissionCheckUID_FailsClosedOnExistenceFailure(t *testing.T) {
	t.Run("user not found", func(t *testing.T) {
		deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
		deps.getenv = func(string) string { return "1000" }
		deps.verifyUserExists = func(int) error { return user.UnknownUserIdError(1000) }

		uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

		assert.Zero(t, uid)
		require.Error(t, err)
	})

	t.Run("lookup failed", func(t *testing.T) {
		deps := newPermissionCheckUIDDeps(&permissionCheckUIDDepsRecorder{})
		deps.getenv = func(string) string { return "1000" }
		deps.verifyUserExists = func(int) error { return errors.New("injected lookup failure") }

		uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

		assert.Zero(t, uid)
		require.Error(t, err)
	})
}

// TestResolvePermissionCheckUID_VerifiesEvenWhenSudoUIDIsZero verifies that
// the existence check runs even when SUDO_UID is "0" and therefore equals the
// real UID, so that a value matching the real UID cannot pass unverified.
func TestResolvePermissionCheckUID_VerifiesEvenWhenSudoUIDIsZero(t *testing.T) {
	rec := &permissionCheckUIDDepsRecorder{}
	deps := newPermissionCheckUIDDeps(rec)
	deps.getenv = func(string) string { return "0" }

	uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

	require.NoError(t, err)
	assert.Zero(t, uid)
	assert.Equal(t, 1, rec.verifyUserExistsCalls)
	assert.Zero(t, rec.reportAdoptionCalls)
}

// TestResolvePermissionCheckUID_RecordFailureDoesNotChangeVerdict verifies
// that the adoption record is observational only: even when the logger's
// handler fails on every record, the base UID is still resolved correctly and
// no error is returned (architecture document §1.2).
func TestResolvePermissionCheckUID_RecordFailureDoesNotChangeVerdict(t *testing.T) {
	handler := newLogCaptureHandler(errors.New("injected handler failure"))
	logger := slog.New(handler)
	reporter := &sudoUIDAdoptionReporter{}
	deps := permissionCheckUIDDeps{
		getenv:           func(string) string { return "1000" },
		verifyUserExists: func(int) error { return nil },
		reportAdoption: func(policy PermissionCheckUIDPolicy, realUID, permissionCheckUID int) {
			reporter.report(logger, policy, realUID, permissionCheckUID)
		},
	}

	uid, err := resolvePermissionCheckUID(SudoUIDAware, 0, deps)

	require.NoError(t, err)
	assert.Equal(t, 1000, uid)
}

// TestResolvePermissionCheckUID_PanicsOnNilDeps verifies that the exported
// test wrapper fails fast with a panic when either of the two mandatory
// dependency seams is nil, so that callers cannot silently fall back to a
// nil-getenv or nil-verification resolution.
func TestResolvePermissionCheckUID_PanicsOnNilDeps(t *testing.T) {
	t.Run("nil Getenv", func(t *testing.T) {
		assert.PanicsWithValue(t, "groupmembership: Getenv must not be nil", func() {
			_, _ = ResolvePermissionCheckUID(SudoUIDAware, 0, PermissionCheckUIDDeps{
				VerifyUserExists: func(int) error { return nil },
			})
		})
	})

	t.Run("nil VerifyUserExists", func(t *testing.T) {
		assert.PanicsWithValue(t, "groupmembership: VerifyUserExists must not be nil", func() {
			_, _ = ResolvePermissionCheckUID(SudoUIDAware, 0, PermissionCheckUIDDeps{
				Getenv: func(string) string { return "1000" },
			})
		})
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
		assert.Equal(t, 1, calls)
		for _, name := range names {
			assert.Equal(t, "SUDO_UID", name)
		}
	})
}
