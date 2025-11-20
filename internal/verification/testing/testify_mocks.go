// Package verificationtesting provides mock implementations for verification management testing.
package verificationtesting

import (
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
