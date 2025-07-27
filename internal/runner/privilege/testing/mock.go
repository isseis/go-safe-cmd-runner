// Package testing provides shared test utilities for privilege management.
package testing

import (
	"context"
	"errors"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
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

// MockPrivilegeManager provides a mock implementation of privilege.Manager for testing
type MockPrivilegeManager struct {
	Supported      bool
	ElevationCalls []string
	ShouldFail     bool
	ExecFn         func() error // カスタム実行関数（テスト用）
}

// WithPrivileges executes the given function with privilege elevation
func (m *MockPrivilegeManager) WithPrivileges(_ context.Context, elevationCtx privilege.ElevationContext, fn func() error) error {
	m.ElevationCalls = append(m.ElevationCalls, string(elevationCtx.Operation))
	if m.ShouldFail {
		return ErrMockPrivilegeElevationFailed
	}
	// カスタム実行関数があれば、それを優先して実行
	if m.ExecFn != nil {
		return m.ExecFn()
	}
	return fn()
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
		return privilege.ErrPrivilegedExecutionNotAvailable
	}
	return nil
}

// GetHealthStatus returns the health status of the privilege manager
func (m *MockPrivilegeManager) GetHealthStatus(_ context.Context) privilege.HealthStatus {
	return privilege.HealthStatus{
		IsSupported:      m.Supported,
		SetuidConfigured: m.Supported,
		OriginalUID:      MockUID,
		CurrentUID:       MockUID,
		EffectiveUID:     MockUID,
		CanElevate:       m.Supported,
	}
}

// GetMetrics returns metrics for the privilege manager
func (m *MockPrivilegeManager) GetMetrics() privilege.Metrics {
	return privilege.Metrics{}
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

// NewMockPrivilegeManagerWithExecFn creates a new MockPrivilegeManager with a custom execution function
func NewMockPrivilegeManagerWithExecFn(supported bool, execFn func() error) *MockPrivilegeManager {
	return &MockPrivilegeManager{
		Supported: supported,
		ExecFn:    execFn,
	}
}
