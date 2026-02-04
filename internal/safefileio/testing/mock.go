//go:build test

// Package testing provides testing utilities for safefileio package.
package testing

import (
	"errors"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// ErrSafeOpenFileNotImplemented is returned by MockFileSystem when SafeOpenFile
// is not implemented by the test.
var ErrSafeOpenFileNotImplemented = errors.New("SafeOpenFile not implemented in mock")

// MockFileSystem implements safefileio.FileSystem for testing.
type MockFileSystem struct {
	// SafeOpenFileFunc allows customizing SafeOpenFile behavior
	SafeOpenFileFunc func(name string, flag int, perm os.FileMode) (safefileio.File, error)
	// GetGroupMembershipFunc allows customizing GetGroupMembership behavior
	GetGroupMembershipFunc func() *groupmembership.GroupMembership
	// RemoveFunc allows customizing Remove behavior
	RemoveFunc func(name string) error
	// AtomicMoveFileFunc allows customizing AtomicMoveFile behavior
	AtomicMoveFileFunc func(srcPath, dstPath string, requiredPerm os.FileMode) error

	// Call tracking for verification
	AtomicMoveFileCalls []struct {
		SrcPath      string
		DstPath      string
		RequiredPerm os.FileMode
	}
	RemoveCalls []string
}

// SafeOpenFile implements safefileio.FileSystem.
func (m *MockFileSystem) SafeOpenFile(name string, flag int, perm os.FileMode) (safefileio.File, error) {
	if m.SafeOpenFileFunc != nil {
		return m.SafeOpenFileFunc(name, flag, perm)
	}
	return nil, ErrSafeOpenFileNotImplemented
}

// GetGroupMembership implements safefileio.FileSystem.
func (m *MockFileSystem) GetGroupMembership() *groupmembership.GroupMembership {
	if m.GetGroupMembershipFunc != nil {
		return m.GetGroupMembershipFunc()
	}
	return groupmembership.New()
}

// Remove implements safefileio.FileSystem.
func (m *MockFileSystem) Remove(name string) error {
	m.RemoveCalls = append(m.RemoveCalls, name)
	if m.RemoveFunc != nil {
		return m.RemoveFunc(name)
	}
	return nil
}

// AtomicMoveFile implements safefileio.FileSystem.
func (m *MockFileSystem) AtomicMoveFile(srcPath, dstPath string, requiredPerm os.FileMode) error {
	m.AtomicMoveFileCalls = append(m.AtomicMoveFileCalls, struct {
		SrcPath      string
		DstPath      string
		RequiredPerm os.FileMode
	}{srcPath, dstPath, requiredPerm})
	if m.AtomicMoveFileFunc != nil {
		return m.AtomicMoveFileFunc(srcPath, dstPath, requiredPerm)
	}
	return nil
}

// NewMockFileSystem creates a new MockFileSystem with default implementations.
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{}
}
