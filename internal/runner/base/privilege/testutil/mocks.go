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

// MockWindowPhase identifies where inside an open privilege window InWindow
// was called. The zero value, MockWindowPhaseUnset, marks a call site that
// forgot to pass one; WithPrivileges never passes it.
type MockWindowPhase int

// MockWindowPhase values. See MockPrivilegeManager.InWindow for what
// "before" and "after" observe.
const (
	MockWindowPhaseUnset MockWindowPhase = iota
	MockWindowPhaseBeforeFn
	MockWindowPhaseAfterFn
)

// MockPrivilegeManager provides a mock implementation of runnertypes.PrivilegeManager for testing
type MockPrivilegeManager struct {
	Supported      bool
	ElevationCalls []string
	ShouldFail     bool
	ExecFn         func() error // Custom execution function (for testing)

	// InWindow, if set, is called twice while the window is open: once
	// immediately before fn runs (MockWindowPhaseBeforeFn) and once
	// immediately after fn returns (MockWindowPhaseAfterFn). It exists to let
	// a test sample process-wide state (the goroutine set, a child's
	// liveness) at the instant the window is open; it cannot observe what fn
	// itself called (that is the static guard's job). The "after" call
	// matters on its own: something started inside fn (e.g. a regression that
	// reintroduces an os/exec copy goroutine, which Start creates during
	// fn's own call) would be invisible to a "before"-only sample. When
	// ExecFn replaces fn, the two samples bracket ExecFn instead; they are
	// skipped only when the window fails before running anything at all
	// (ShouldFail or FailFor).
	InWindow func(phase MockWindowPhase)

	// FailFor injects a failure for one specific operation. Checked only when
	// ShouldFail is false: ShouldFail fails every operation indiscriminately
	// and takes precedence, so a test wanting "the start window succeeds but
	// the kill window fails" must leave ShouldFail unset and populate FailFor
	// alone.
	FailFor map[runnertypes.Operation]error

	// inWindow mirrors UnixPrivilegeManager's unsynchronized reentrancy
	// guard: a nested WithPrivileges call on this same mock, made from within
	// fn, returns privilege.ErrReentrantPrivilegeCall instead of running fn a
	// second time. It is the same sentinel the real implementation returns
	// (not a mock-only one), so a test asserting on it is asserting something
	// production code can actually produce.
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
	// If a custom execution function exists, prioritize and execute it in
	// place of fn. It still runs inside the open window, so the InWindow
	// samples bracket it just as they bracket fn -- skipping them here would
	// let a test that sets both ExecFn and InWindow collect no samples at all
	// and pass its window assertions vacuously.
	run := fn
	if m.ExecFn != nil {
		run = m.ExecFn
	}
	if m.InWindow != nil {
		m.InWindow(MockWindowPhaseBeforeFn)
	}
	err := run()
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
