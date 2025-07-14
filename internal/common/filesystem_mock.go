package common

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	// DefaultDirPerm represents default directory permissions (rwxr-xr-x)
	DefaultDirPerm = 0o755

	// SymlinkPerm represents default symlink permissions (rwxrwxrwx)
	// In real system, permission of symlink is never used, but permission of
	// target file/directory is used for permission check on system calls.
	SymlinkPerm = 0o777
)

// MockFileSystem implements FileSystem for testing
type MockFileSystem struct {
	files map[string]*MockFileInfo
	dirs  map[string]bool
	// Counter for creating unique temp directories
	tempDirCounter int
	// Symlinks maps symlink path to target path
	symlinks map[string]string
}

// ErrDirectoryNotEmpty is returned when trying to remove a non-empty directory
var ErrDirectoryNotEmpty = errors.New("directory not empty")

// MockFileInfo implements fs.FileInfo for testing
type MockFileInfo struct {
	name      string
	size      int64
	mode      os.FileMode
	modTime   time.Time
	isDir     bool
	isSymlink bool
	uid       uint32
	gid       uint32
}

// Name returns the base name of the file
func (m *MockFileInfo) Name() string { return m.name }

// Size returns the length in bytes
func (m *MockFileInfo) Size() int64 { return m.size }

// Mode returns the file mode bits
func (m *MockFileInfo) Mode() os.FileMode {
	if m.isSymlink {
		return m.mode | os.ModeSymlink
	}
	return m.mode
}

// ModTime returns the modification time
func (m *MockFileInfo) ModTime() time.Time { return m.modTime }

// IsDir reports whether m describes a directory
func (m *MockFileInfo) IsDir() bool { return m.isDir }

// Sys returns the underlying data source (syscall.Stat_t for mock)
func (m *MockFileInfo) Sys() any {
	return &syscall.Stat_t{
		Uid: m.uid,
		Gid: m.gid,
	}
}

// NewMockFileSystem creates a new MockFileSystem
func NewMockFileSystem() *MockFileSystem {
	fs := &MockFileSystem{
		files:    make(map[string]*MockFileInfo),
		dirs:     make(map[string]bool),
		symlinks: make(map[string]string),
	}

	// Add root directory by default (owned by root with secure permissions)
	fs.AddDirWithOwner("/", 0o755, 0, 0)

	return fs
}

// CreateTempDir creates a mock temporary directory
func (m *MockFileSystem) CreateTempDir(prefix string) (string, error) {
	m.tempDirCounter++
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("%s%d", prefix, m.tempDirCounter))
	m.dirs[tempDir] = true
	m.files[tempDir] = &MockFileInfo{
		name:      filepath.Base(tempDir),
		mode:      DefaultDirPerm,
		modTime:   time.Now(),
		isDir:     true,
		isSymlink: false,
		uid:       0,
		gid:       0,
	}
	return tempDir, nil
}

// TempDir returns the default directory for temporary files
func (m *MockFileSystem) TempDir() string {
	return "/tmp"
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
				name:      filepath.Base(currentPath),
				mode:      perm,
				modTime:   time.Now(),
				isDir:     true,
				isSymlink: false,
				uid:       0,
				gid:       0,
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
	sep := string(filepath.Separator)
	prefix := path + sep
	for filePath := range m.files {
		if strings.HasPrefix(filePath, prefix) {
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

// Lstat returns file information for the given path
func (m *MockFileSystem) Lstat(path string) (fs.FileInfo, error) {
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
		name:      filepath.Base(path),
		size:      int64(len(content)),
		mode:      mode,
		modTime:   time.Now(),
		isDir:     false,
		isSymlink: false,
		uid:       0,
		gid:       0,
	}
	return nil
}

// AddDir adds a directory to the mock filesystem (for testing)
func (m *MockFileSystem) AddDir(path string, mode os.FileMode) {
	m.AddDirWithOwner(path, mode, 0, 0)
}

// AddDirWithOwner adds a directory with specified owner to the mock filesystem (for testing)
func (m *MockFileSystem) AddDirWithOwner(path string, mode os.FileMode, uid, gid uint32) {
	path = filepath.Clean(path)

	m.dirs[path] = true
	m.files[path] = &MockFileInfo{
		name:      filepath.Base(path),
		mode:      mode | os.ModeDir, // Add directory flag to mode
		modTime:   time.Now(),
		isDir:     true,
		isSymlink: false,
		uid:       uid,
		gid:       gid,
	}
}

// AddSymlink adds a symbolic link to the mock filesystem (for testing)
func (m *MockFileSystem) AddSymlink(linkPath, targetPath string) {
	linkPath = filepath.Clean(linkPath)
	targetPath = filepath.Clean(targetPath)

	m.symlinks[linkPath] = targetPath
	m.files[linkPath] = &MockFileInfo{
		name:      filepath.Base(linkPath),
		mode:      SymlinkPerm,
		modTime:   time.Now(),
		isDir:     false,
		isSymlink: true,
		uid:       0,
		gid:       0,
	}
}
