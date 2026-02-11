//go:build test

// Package testutil provides shared test utilities and mock implementations
// for the runner package.
package testutil

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
func (m *MockResourceManager) ExecuteCommand(ctx context.Context, cmd *runnertypes.RuntimeCommand, group *runnertypes.GroupSpec, env map[string]string) (resource.CommandToken, *resource.ExecutionResult, error) {
	args := m.Called(ctx, cmd, group, env)

	// Expected format: .Return(token, result, error)
	// All three values must be provided, even if token is "" or result is nil
	const (
		tokenIndex  = 0
		resultIndex = 1
		errorIndex  = 2
	)

	// Extract token (may be empty string)
	var token resource.CommandToken
	if args.Get(tokenIndex) != nil {
		token = args.Get(tokenIndex).(resource.CommandToken)
	}

	// Extract result (may be nil) and error
	var result *resource.ExecutionResult
	if args.Get(resultIndex) != nil {
		result = args.Get(resultIndex).(*resource.ExecutionResult)
	}

	return token, result, args.Error(errorIndex)
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

// RecordGroupAnalysis records group analysis in dry-run mode
func (m *MockResourceManager) RecordGroupAnalysis(groupName string, debugInfo *resource.DebugInfo) error {
	args := m.Called(groupName, debugInfo)
	return args.Error(0)
}

// UpdateCommandDebugInfo updates a command's debug info using its token in dry-run mode
func (m *MockResourceManager) UpdateCommandDebugInfo(token resource.CommandToken, debugInfo *resource.DebugInfo) error {
	args := m.Called(token, debugInfo)
	return args.Error(0)
}
