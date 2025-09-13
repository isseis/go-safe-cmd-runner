// Package testing provides mock implementations for executor interfaces used in testing.
package testing

import (
	"context"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/mock"
)

// MockExecutor provides a mock implementation of executor.CommandExecutor for testing.
// This mock includes safe nil handling to prevent panics during error case testing.
type MockExecutor struct {
	mock.Mock
}

// Execute implements executor.CommandExecutor.Execute with safe nil handling.
// If the mock returns nil as the result, it safely returns nil without panicking.
func (m *MockExecutor) Execute(ctx context.Context, cmd runnertypes.Command, env map[string]string) (*executor.Result, error) {
	args := m.Called(ctx, cmd, env)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*executor.Result), args.Error(1)
}

// Validate implements executor.CommandExecutor.Validate.
func (m *MockExecutor) Validate(cmd runnertypes.Command) error {
	args := m.Called(cmd)
	return args.Error(0)
}

// NewMockExecutor creates a new MockExecutor instance.
func NewMockExecutor() *MockExecutor {
	return &MockExecutor{}
}
