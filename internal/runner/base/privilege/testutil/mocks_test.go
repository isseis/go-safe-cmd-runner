//go:build test

package privilegetestutil

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockPrivilegeManager_InWindowCalledBeforeAndAfterFn(t *testing.T) {
	m := NewMockPrivilegeManager(true)
	var phases []MockWindowPhase
	m.InWindow = func(phase MockWindowPhase) {
		phases = append(phases, phase)
	}

	fnCalled := false
	err := m.WithPrivileges(runnertypes.ElevationContext{Operation: runnertypes.OperationUserGroupExecution}, func() error {
		fnCalled = true
		// The "before" sample must already have been taken by the time fn runs.
		assert.Equal(t, []MockWindowPhase{MockWindowPhaseBeforeFn}, phases)
		return nil
	})

	require.NoError(t, err)
	assert.True(t, fnCalled)
	assert.Equal(t, []MockWindowPhase{MockWindowPhaseBeforeFn, MockWindowPhaseAfterFn}, phases)
}

func TestMockPrivilegeManager_ReentrantCallReturnsSentinel(t *testing.T) {
	m := NewMockPrivilegeManager(true)

	var innerErr error
	outerErr := m.WithPrivileges(runnertypes.ElevationContext{Operation: runnertypes.OperationUserGroupExecution}, func() error {
		innerErr = m.WithPrivileges(runnertypes.ElevationContext{Operation: runnertypes.OperationKillAfterCancel}, func() error {
			return nil
		})
		return nil
	})

	require.NoError(t, outerErr)
	assert.ErrorIs(t, innerErr, privilege.ErrReentrantPrivilegeCall)
	// The guard must clear after the outer call returns, so a later,
	// non-nested call succeeds normally.
	err := m.WithPrivileges(runnertypes.ElevationContext{Operation: runnertypes.OperationUserGroupExecution}, func() error {
		return nil
	})
	require.NoError(t, err)
}

func TestMockPrivilegeManager_FailForInjectsPerOperationError(t *testing.T) {
	errKill := errors.New("kill window failure")
	m := NewMockPrivilegeManager(true)
	m.FailFor = map[runnertypes.Operation]error{
		runnertypes.OperationKillAfterCancel: errKill,
	}

	// The operation not named in FailFor still succeeds.
	err := m.WithPrivileges(runnertypes.ElevationContext{Operation: runnertypes.OperationUserGroupExecution}, func() error {
		return nil
	})
	require.NoError(t, err)

	// The operation named in FailFor fails without running fn.
	fnCalled := false
	err = m.WithPrivileges(runnertypes.ElevationContext{Operation: runnertypes.OperationKillAfterCancel}, func() error {
		fnCalled = true
		return nil
	})
	assert.ErrorIs(t, err, errKill)
	assert.False(t, fnCalled)
}

func TestMockPrivilegeManager_ShouldFailTakesPrecedenceOverFailFor(t *testing.T) {
	errKill := errors.New("kill window failure")
	m := NewMockPrivilegeManager(true)
	m.ShouldFail = true
	m.FailFor = map[runnertypes.Operation]error{
		runnertypes.OperationKillAfterCancel: errKill,
	}

	err := m.WithPrivileges(runnertypes.ElevationContext{Operation: runnertypes.OperationKillAfterCancel}, func() error {
		return nil
	})
	assert.ErrorIs(t, err, ErrMockPrivilegeElevationFailed)
	assert.NotErrorIs(t, err, errKill)
}

// TestMockPrivilegeManager_InWindowBracketsExecFn pins that ExecFn, which
// replaces fn but still runs inside the open window, is bracketed by the same
// two InWindow samples. Without it a test setting both would collect no
// samples and pass any window assertion vacuously.
func TestMockPrivilegeManager_InWindowBracketsExecFn(t *testing.T) {
	m := NewMockPrivilegeManager(true)
	var phases []MockWindowPhase
	m.InWindow = func(phase MockWindowPhase) {
		phases = append(phases, phase)
	}
	execFnCalled := false
	m.ExecFn = func() error {
		execFnCalled = true
		assert.Equal(t, []MockWindowPhase{MockWindowPhaseBeforeFn}, phases)
		return nil
	}

	fnCalled := false
	err := m.WithPrivileges(runnertypes.ElevationContext{Operation: runnertypes.OperationUserGroupExecution}, func() error {
		fnCalled = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, execFnCalled)
	// ExecFn takes precedence over fn; that existing behaviour is unchanged.
	assert.False(t, fnCalled)
	assert.Equal(t, []MockWindowPhase{MockWindowPhaseBeforeFn, MockWindowPhaseAfterFn}, phases)
}
