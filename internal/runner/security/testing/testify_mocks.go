// Package securitytesting provides mock implementations for security validation testing.
package securitytesting

import (
	"github.com/stretchr/testify/mock"
)

// MockValidator is a mock implementation of ValidatorInterface for testing
type MockValidator struct {
	mock.Mock
}

// ValidateAllEnvironmentVars mocks the ValidateAllEnvironmentVars method
func (m *MockValidator) ValidateAllEnvironmentVars(envVars map[string]string) error {
	args := m.Called(envVars)
	return args.Error(0)
}

// ValidateEnvironmentValue mocks the ValidateEnvironmentValue method
func (m *MockValidator) ValidateEnvironmentValue(key, value string) error {
	args := m.Called(key, value)
	return args.Error(0)
}

// ValidateCommand mocks the ValidateCommand method
func (m *MockValidator) ValidateCommand(command string) error {
	args := m.Called(command)
	return args.Error(0)
}

// ValidateCommandAllowed mocks the ValidateCommandAllowed method
func (m *MockValidator) ValidateCommandAllowed(cmdPath string, groupCmdAllowed map[string]struct{}) error {
	args := m.Called(cmdPath, groupCmdAllowed)
	return args.Error(0)
}

// SanitizeOutputForLogging mocks the SanitizeOutputForLogging method
func (m *MockValidator) SanitizeOutputForLogging(output string) string {
	args := m.Called(output)
	return args.String(0)
}
