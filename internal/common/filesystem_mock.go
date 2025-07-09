package common

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// DefaultDirPerm represents default directory permissions (rwxr-xr-x)
	DefaultDirPerm = 0o755
)

// MockFileSystem implements FileSystem for testing
type MockFileSystem struct {
	files map[string]*MockFileInfo
	dirs  map[string]bool
	// Counter for creating unique temp directories
	tempDirCounter int
}

// ErrDirectoryNotEmpty is returned when trying to remove a non-empty directory
var ErrDirectoryNotEmpty = errors.New("directory not empty")

// MockFileInfo implements fs.FileInfo for testing
type MockFileInfo struct {
	name    string
	size    int64
	mode    os.FileMode
	modTime time.Time
	isDir   bool
}

// Name returns the base name of the file
func (m *MockFileInfo) Name() string { return m.name }

// Size returns the length in bytes
func (m *MockFileInfo) Size() int64 { return m.size }

// Mode returns the file mode bits
func (m *MockFileInfo) Mode() os.FileMode { return m.mode }

// ModTime returns the modification time
func (m *MockFileInfo) ModTime() time.Time { return m.modTime }

// IsDir reports whether m describes a directory
func (m *MockFileInfo) IsDir() bool { return m.isDir }

// Sys returns the underlying data source (always nil for mock)
func (m *MockFileInfo) Sys() any { return nil }

// NewMockFileSystem creates a new MockFileSystem
func NewMockFileSystem() *MockFileSystem {
	return &MockFileSystem{
		files: make(map[string]*MockFileInfo),
		dirs:  make(map[string]bool),
	}
}

// CreateTempDir creates a mock temporary directory
func (m *MockFileSystem) CreateTempDir(prefix string) (string, error) {
	m.tempDirCounter++
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("%s%d", prefix, m.tempDirCounter))
	m.dirs[tempDir] = true
	m.files[tempDir] = &MockFileInfo{
		name:    filepath.Base(tempDir),
		mode:    DefaultDirPerm,
		modTime: time.Now(),
		isDir:   true,
	}
	return tempDir, nil
}

// MkdirAll creates directories and all parent directories in the mock filesystem
func (m *MockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	// Normalize path
	path = filepath.Clean(path)

	// Create all parent directories
	parts := strings.Split(path, string(filepath.Separator))
	currentPath := ""

	for i, part := range parts {
		switch {
		case part == "" && i == 0:
			// Root directory on Unix
			currentPath = "/"
		case part == "":
			continue
		case currentPath == "/":
			currentPath = "/" + part
		case currentPath == "":
			currentPath = part
		default:
			currentPath = filepath.Join(currentPath, part)
		}

		// Create directory if it doesn't exist
		if _, exists := m.dirs[currentPath]; !exists {
			m.dirs[currentPath] = true
			m.files[currentPath] = &MockFileInfo{
				name:    filepath.Base(currentPath),
				mode:    perm,
				modTime: time.Now(),
				isDir:   true,
			}
		}
	}

	return nil
}

// RemoveAll removes a directory and all its contents from the mock filesystem
func (m *MockFileSystem) RemoveAll(path string) error {
	path = filepath.Clean(path)

	// Remove the path itself if it exists
	delete(m.dirs, path)
	delete(m.files, path)

	// Remove all subdirectories and files
	for filePath := range m.files {
		if strings.HasPrefix(filePath, path+"/") {
			delete(m.files, filePath)
			delete(m.dirs, filePath)
		}
	}

	return nil
}

// Remove removes a single file or empty directory from the mock filesystem
func (m *MockFileSystem) Remove(path string) error {
	path = filepath.Clean(path)

	if _, exists := m.files[path]; !exists {
		return os.ErrNotExist
	}

	// For directories, check if empty
	for filePath := range m.files {
		if filePath != path && strings.HasPrefix(filePath, path+string(filepath.Separator)) {
			return ErrDirectoryNotEmpty
		}
	}

	delete(m.files, path)
	delete(m.dirs, path)

	return nil
}

// Stat returns file information for the given path
func (m *MockFileSystem) Stat(path string) (fs.FileInfo, error) {
	path = filepath.Clean(path)

	info, exists := m.files[path]
	if !exists {
		return nil, os.ErrNotExist
	}

	return info, nil
}

// FileExists checks if a file or directory exists in the mock filesystem
func (m *MockFileSystem) FileExists(path string) (bool, error) {
	path = filepath.Clean(path)
	_, exists := m.files[path]
	return exists, nil
}

// IsDir checks if the path is a directory in the mock filesystem
func (m *MockFileSystem) IsDir(path string) (bool, error) {
	path = filepath.Clean(path)

	info, exists := m.files[path]
	if !exists {
		return false, os.ErrNotExist
	}

	return info.IsDir(), nil
}

// GetFiles returns all files in the mock filesystem (for testing)
func (m *MockFileSystem) GetFiles() []string {
	var files []string
	for path := range m.files {
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

// GetDirs returns all directories in the mock filesystem (for testing)
func (m *MockFileSystem) GetDirs() []string {
	var dirs []string
	for path := range m.dirs {
		dirs = append(dirs, path)
	}
	sort.Strings(dirs)
	return dirs
}

// AddFile adds a file to the mock filesystem (for testing)
func (m *MockFileSystem) AddFile(path string, mode os.FileMode, content []byte) error {
	path = filepath.Clean(path)

	// Create parent directories if they don't exist
	dir := filepath.Dir(path)
	if dir != "." && dir != "/" {
		if err := m.MkdirAll(dir, DefaultDirPerm); err != nil {
			return err
		}
	}

	m.files[path] = &MockFileInfo{
		name:    filepath.Base(path),
		size:    int64(len(content)),
		mode:    mode,
		modTime: time.Now(),
		isDir:   false,
	}
	return nil
}

// AddDir adds a directory to the mock filesystem (for testing)
func (m *MockFileSystem) AddDir(path string, mode os.FileMode) {
	path = filepath.Clean(path)

	m.dirs[path] = true
	m.files[path] = &MockFileInfo{
		name:    filepath.Base(path),
		mode:    mode,
		modTime: time.Now(),
		isDir:   true,
	}
}
