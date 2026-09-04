//go:build test

// Package privilegetestutil provides shared test utilities for privilege management.
package privilegetestutil

import (
	"context"
	"errors"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
)

// Test constants
const (
	// MockUID is the mock user ID used for testing
	MockUID = 1000
)

// Test error definitions
var (
	ErrMockPrivilegeElevationFailed = errors.New("mock privilege elevation failure")
)

// MockWindowPhase identifies when InWindow was called relative to fn. The
// zero value, MockWindowPhaseUnset, marks a call InWindow should never
// actually observe; it exists so a forgotten case in a switch over this type
// fails loudly instead of matching a phase silently.
type MockWindowPhase int

const (
	// MockWindowPhaseUnset is the zero value; never passed to InWindow.
	MockWindowPhaseUnset MockWindowPhase = iota
	// MockWindowPhaseBeforeFn is passed immediately before fn (or ExecFn) runs,
	// with the window already open.
	MockWindowPhaseBeforeFn
	// MockWindowPhaseAfterFn is passed immediately after fn (or ExecFn)
	// returns, with the window still open. Needed because anything fn itself
	// starts -- e.g. a regression that reintroduces an os/exec copy goroutine
	// -- exists only after fn returns, not before it is called.
	MockWindowPhaseAfterFn
)

// MockPrivilegeManager provides a mock implementation of runnertypes.PrivilegeManager for testing
type MockPrivilegeManager struct {
	Supported      bool
	ElevationCalls []string
	ShouldFail     bool
	ExecFn         func() error // Custom execution function (for testing)

	// FailFor injects a failure for one specific operation, leaving every
	// other operation to succeed. ShouldFail cannot express "the kill window
	// fails but the start window succeeds"; FailFor can.
	FailFor map[runnertypes.Operation]error

	// InWindow, when set, is called twice while the window is open: once
	// right before fn runs and once right after it returns (see
	// MockWindowPhase). It observes the state of the window at that instant
	// (goroutines, child process liveness, flags) -- not what fn itself calls,
	// which is a job for static analysis, not this mock.
	InWindow func(phase MockWindowPhase)

	// inWindow mirrors UnixPrivilegeManager's non-reentrant guard: a call to
	// WithPrivileges from within an already-open window returns
	// privilege.ErrReentrantPrivilegeCall instead of opening a second one.
	inWindow bool
}

// WithPrivileges executes the given function with privilege elevation
func (m *MockPrivilegeManager) WithPrivileges(elevationCtx runnertypes.ElevationContext, fn func() error) error {
	if m.inWindow {
		return privilege.ErrReentrantPrivilegeCall
	}
	m.inWindow = true
	defer func() { m.inWindow = false }()

	// Record different types of operations differently for test verification
	switch elevationCtx.Operation {
	case runnertypes.OperationUserGroupExecution:
		m.ElevationCalls = append(m.ElevationCalls, "user_group_change:"+elevationCtx.RunAsUser+":"+elevationCtx.RunAsGroup)
	default:
		m.ElevationCalls = append(m.ElevationCalls, string(elevationCtx.Operation))
	}

	if m.ShouldFail {
		return ErrMockPrivilegeElevationFailed
	}
	if err, ok := m.FailFor[elevationCtx.Operation]; ok {
		return err
	}

	if m.InWindow != nil {
		m.InWindow(MockWindowPhaseBeforeFn)
	}
	var err error
	// If a custom execution function exists, prioritize and execute it
	if m.ExecFn != nil {
		err = m.ExecFn()
	} else {
		err = fn()
	}
	if m.InWindow != nil {
		m.InWindow(MockWindowPhaseAfterFn)
	}
	return err
}

// IsPrivilegedExecutionSupported returns whether privileged execution is supported
func (m *MockPrivilegeManager) IsPrivilegedExecutionSupported() bool {
	return m.Supported
}

// GetCurrentUID returns the current user ID
func (m *MockPrivilegeManager) GetCurrentUID() int {
	return MockUID
}

// GetOriginalUID returns the original user ID
func (m *MockPrivilegeManager) GetOriginalUID() int {
	return MockUID
}

// HealthCheck performs a health check on the privilege manager
func (m *MockPrivilegeManager) HealthCheck(_ context.Context) error {
	if !m.Supported {
		return runnertypes.ErrPrivilegedExecutionNotAvailable
	}
	return nil
}

// NewMockPrivilegeManager creates a new MockPrivilegeManager with the given support status
func NewMockPrivilegeManager(supported bool) *MockPrivilegeManager {
	return &MockPrivilegeManager{
		Supported: supported,
	}
}

// NewFailingMockPrivilegeManager creates a new MockPrivilegeManager that will fail privilege elevation
func NewFailingMockPrivilegeManager(supported bool) *MockPrivilegeManager {
	return &MockPrivilegeManager{
		Supported:  supported,
		ShouldFail: true,
	}
}
