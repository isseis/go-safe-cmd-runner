//go:build test || !prod

package executor

import "os"

// Common mock implementations for testing across executor package

// MockOutputWriter implements OutputWriter for testing
type MockOutputWriter struct {
	Outputs []string
}

// Write implements the OutputWriter interface for testing
func (m *MockOutputWriter) Write(_ string, data []byte) error {
	if m.Outputs == nil {
		m.Outputs = make([]string, 0)
	}
	m.Outputs = append(m.Outputs, string(data))
	return nil
}

// Close implements the OutputWriter interface for testing
func (m *MockOutputWriter) Close() error {
	return nil
}

// MockFileSystem implements FileSystem for testing
type MockFileSystem struct {
	// A map to configure which paths exist for advanced testing scenarios
	ExistingPaths map[string]bool
	// An error to return from methods, for testing error paths
	Err error
}

// CreateTempDir implements the FileSystem interface for testing
func (m *MockFileSystem) CreateTempDir(dir, prefix string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	return os.MkdirTemp(dir, prefix)
}

// RemoveAll implements the FileSystem interface for testing
func (m *MockFileSystem) RemoveAll(path string) error {
	if m.Err != nil {
		return m.Err
	}
	return os.RemoveAll(path)
}

// FileExists implements the FileSystem interface for testing
func (m *MockFileSystem) FileExists(path string) (bool, error) {
	if m.Err != nil {
		return false, m.Err
	}

	// If ExistingPaths is configured, use it
	if m.ExistingPaths != nil {
		return m.ExistingPaths[path], nil
	}

	// Otherwise, use real filesystem check
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// NewMockFileSystem creates a new MockFileSystem with default behavior
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{}
}

// NewMockFileSystemWithPaths creates a MockFileSystem with predefined paths
func NewMockFileSystemWithPaths(paths map[string]bool) *MockFileSystem {
	return &MockFileSystem{ExistingPaths: paths}
}

// NewMockOutputWriter creates a new MockOutputWriter
func NewMockOutputWriter() *MockOutputWriter {
	return &MockOutputWriter{Outputs: make([]string, 0)}
}
