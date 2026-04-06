//go:build test

// Package verificationtesting provides mock implementations for verification management testing.
package verificationtesting

import (
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/mock"
)

// MockManager is a mock implementation of verification.ManagerInterface
type MockManager struct {
	mock.Mock
}

// ResolvePath mocks the ResolvePath method
func (m *MockManager) ResolvePath(path string) (string, error) {
	args := m.Called(path)
	return args.String(0), args.Error(1)
}

// VerifyGroupFiles mocks the VerifyGroupFiles method
func (m *MockManager) VerifyGroupFiles(runtimeGroup *runnertypes.RuntimeGroup) (*verification.Result, error) {
	args := m.Called(runtimeGroup)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*verification.Result), args.Error(1)
}

// VerifyCommandDynLibDeps mocks the VerifyCommandDynLibDeps method
func (m *MockManager) VerifyCommandDynLibDeps(cmdPath string) error {
	args := m.Called(cmdPath)
	return args.Error(0)
}

// VerifyCommandShebangInterpreter mocks the VerifyCommandShebangInterpreter method
func (m *MockManager) VerifyCommandShebangInterpreter(cmdPath string, envVars map[string]string) error {
	args := m.Called(cmdPath, envVars)
	return args.Error(0)
}

// MockFileValidator is a mock implementation of filevalidator.FileValidator
type MockFileValidator struct {
	mock.Mock
}

// Verify mocks the Verify method
func (m *MockFileValidator) Verify(filePath string) error {
	args := m.Called(filePath)
	return args.Error(0)
}

// VerifyWithHash mocks the VerifyWithHash method
func (m *MockFileValidator) VerifyWithHash(filePath string) (string, error) {
	args := m.Called(filePath)
	return args.String(0), args.Error(1)
}

// VerifyWithPrivileges mocks the VerifyWithPrivileges method
func (m *MockFileValidator) VerifyWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) error {
	args := m.Called(filePath, privManager)
	return args.Error(0)
}

// VerifyAndRead mocks the VerifyAndRead method
func (m *MockFileValidator) VerifyAndRead(filePath string) ([]byte, error) {
	args := m.Called(filePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// VerifyAndReadWithPrivileges mocks the VerifyAndReadWithPrivileges method
func (m *MockFileValidator) VerifyAndReadWithPrivileges(filePath string, privManager runnertypes.PrivilegeManager) ([]byte, error) {
	args := m.Called(filePath, privManager)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// LoadRecord mocks the LoadRecord method
func (m *MockFileValidator) LoadRecord(filePath string) (*fileanalysis.Record, error) {
	args := m.Called(filePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*fileanalysis.Record), args.Error(1)
}
