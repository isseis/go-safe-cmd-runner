// Package testing provides shared test utilities and mock implementations
// for the runner package.
package testing

import (
	"context"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/mock"
)

// MockResourceManager is a mock implementation of ResourceManager
type MockResourceManager struct {
	mock.Mock
}

// SetMode sets the execution mode for the resource manager
func (m *MockResourceManager) SetMode(mode resource.ExecutionMode, opts *resource.DryRunOptions) {
	m.Called(mode, opts)
}

// GetMode returns the current execution mode
func (m *MockResourceManager) GetMode() resource.ExecutionMode {
	args := m.Called()
	return args.Get(0).(resource.ExecutionMode)
}

// ExecuteCommand executes a command with the given context, command spec, group spec, and environment
func (m *MockResourceManager) ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (*resource.ExecutionResult, error) {
	args := m.Called(ctx, cmd, group, env)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*resource.ExecutionResult), args.Error(1)
}

// ValidateOutputPath validates that the output path is within the working directory
func (m *MockResourceManager) ValidateOutputPath(outputPath, workDir string) error {
	args := m.Called(outputPath, workDir)
	return args.Error(0)
}

// CreateTempDir creates a temporary directory for the given group name
func (m *MockResourceManager) CreateTempDir(groupName string) (string, error) {
	args := m.Called(groupName)
	return args.String(0), args.Error(1)
}

// CleanupTempDir removes a temporary directory
func (m *MockResourceManager) CleanupTempDir(tempDirPath string) error {
	args := m.Called(tempDirPath)
	return args.Error(0)
}

// CleanupAllTempDirs removes all temporary directories created by this manager
func (m *MockResourceManager) CleanupAllTempDirs() error {
	args := m.Called()
	return args.Error(0)
}

// WithPrivileges executes a function with elevated privileges
func (m *MockResourceManager) WithPrivileges(ctx context.Context, fn func() error) error {
	args := m.Called(ctx, fn)
	return args.Error(0)
}

// SendNotification sends a notification message with optional details
func (m *MockResourceManager) SendNotification(message string, details map[string]any) error {
	args := m.Called(message, details)
	return args.Error(0)
}

// GetDryRunResults returns the dry-run results, or nil for normal execution mode
func (m *MockResourceManager) GetDryRunResults() *resource.DryRunResult {
	args := m.Called()
	if args.Get(0) == nil {
		return nil
	}
	return args.Get(0).(*resource.DryRunResult)
}
