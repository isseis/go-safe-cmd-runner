// Package common provides shared interfaces and utilities used across the runner packages.
//
//nolint:revive // var-naming: package name "common" is intentional for shared internal utilities
package common

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Error definitions for static error handling
var (
	ErrEmptyPath = errors.New("path cannot be empty")
)

// FileSystem defines the interface for file system operations
// This interface allows for easy mocking in tests and provides a consistent API
// for file operations across all packages.
type FileSystem interface {
	// CreateTempDir creates a temporary directory with the given prefix
	CreateTempDir(dir string, prefix string) (string, error)

	// TempDir returns the default directory for temporary files
	TempDir() string

	// RemoveAll removes a directory and all its contents
	RemoveAll(path string) error

	// Remove removes a single file or empty directory
	Remove(path string) error

	// Lstat returns file information
	Lstat(path string) (fs.FileInfo, error)

	// FileExists checks if a file or directory exists
	FileExists(path string) (bool, error)

	// IsDir checks if the path is a directory
	IsDir(path string) (bool, error)

	// CreateTemp creates a temporary file with the given prefix in the specified directory
	CreateTemp(dir string, pattern string) (*os.File, error)

	// MkdirAll creates a directory and all necessary parents with the specified permissions
	MkdirAll(path string, perm os.FileMode) error

	// EvalSymlinks returns the path name after the evaluation of any symbolic links.
	EvalSymlinks(path string) (string, error)
}

// DefaultFileSystem implements FileSystem using standard os package functions
type DefaultFileSystem struct{}

// NewDefaultFileSystem creates a new DefaultFileSystem
func NewDefaultFileSystem() *DefaultFileSystem {
	return &DefaultFileSystem{}
}

// CreateTempDir creates a temporary directory with the given prefix
func (fs *DefaultFileSystem) CreateTempDir(dir string, prefix string) (string, error) {
	return os.MkdirTemp(dir, prefix)
}

// TempDir returns the default directory for temporary files
func (fs *DefaultFileSystem) TempDir() string {
	return os.TempDir()
}

// RemoveAll removes a directory and all its contents
func (fs *DefaultFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Remove removes a single file or empty directory
func (fs *DefaultFileSystem) Remove(path string) error {
	return os.Remove(path)
}

// Lstat returns file information
func (fs *DefaultFileSystem) Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

// FileExists checks if a file or directory exists
func (fs *DefaultFileSystem) FileExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// IsDir checks if the path is a directory, following symlinks.
// Use os.Stat so that symlinks to directories (e.g. /tmp -> /private/tmp on
// macOS) are correctly reported as directories.
func (fs *DefaultFileSystem) IsDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}

// CreateTemp creates a temporary file with the given prefix in the specified directory
func (fs *DefaultFileSystem) CreateTemp(dir string, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

// MkdirAll creates a directory and all necessary parents with the specified permissions
func (fs *DefaultFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// EvalSymlinks returns the path name after the evaluation of any symbolic links.
func (fs *DefaultFileSystem) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}

// resolveMode indicates how ResolvedPath was constructed.
type resolveMode int

const (
	// resolveModeFull is set by NewResolvedPath (EvalSymlinks on the full path).
	// It is iota+1 so that the zero value (uninitialized ResolvedPath{}) is never
	// treated as a valid parent-only path, ensuring boundary assertions reject it.
	resolveModeFull resolveMode = iota + 1
	// resolveModeParentOnly is set by NewResolvedPathParentOnly.
	resolveModeParentOnly
)

// ResolvedPath represents a file path that has been resolved to an absolute path
// via one of two constructors:
//   - NewResolvedPath: evaluates all symbolic links including the leaf component.
//   - NewResolvedPathParentOnly: evaluates symbolic links in the parent directory only;
//     the leaf component is left unresolved, so a symlink at that position is preserved
//     and can still be detected by callers (e.g. via openat2(RESOLVE_NO_SYMLINKS)).
//
// Use IsParentOnly to distinguish the two modes. Security-boundary write functions
// require IsParentOnly() == true to preserve leaf-symlink detection.
type ResolvedPath struct {
	path string
	mode resolveMode
}

// NewResolvedPathParentOnly creates a ResolvedPath by resolving only the parent directory
// via EvalSymlinks and re-joining the file name unchanged.
// The file itself need not exist; only the parent directory must exist.
// Because the leaf is not dereferenced, callers such as SafeReadFile can still detect
// and reject a symlink at the leaf position via openat2(RESOLVE_NO_SYMLINKS).
// Returns ErrEmptyPath if the path is empty, or any error from Abs/EvalSymlinks on the parent.
func NewResolvedPathParentOnly(path string) (ResolvedPath, error) {
	if path == "" {
		return ResolvedPath{}, ErrEmptyPath
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ResolvedPath{}, err
	}
	parentDir := filepath.Dir(absPath)
	resolvedParent, err := filepath.EvalSymlinks(parentDir)
	if err != nil {
		return ResolvedPath{}, err
	}
	return ResolvedPath{path: filepath.Join(resolvedParent, filepath.Base(absPath)), mode: resolveModeParentOnly}, nil
}

// NewResolvedPath creates a ResolvedPath for an existing file or directory.
// It resolves the path to an absolute path and evaluates all symbolic links.
// Returns ErrEmptyPath if the path is empty, or any error from Abs/EvalSymlinks.
func NewResolvedPath(path string) (ResolvedPath, error) {
	if path == "" {
		return ResolvedPath{}, ErrEmptyPath
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ResolvedPath{}, err
	}
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return ResolvedPath{}, err
	}
	return ResolvedPath{path: resolvedPath, mode: resolveModeFull}, nil
}

// String returns the resolved path as a string.
func (p ResolvedPath) String() string {
	return p.path
}

// IsParentOnly returns true if this ResolvedPath was created with NewResolvedPathParentOnly.
// Security-boundary write functions (SafeWriteFileOverwrite)
// require IsParentOnly() == true to preserve leaf-symlink detection via openat2(RESOLVE_NO_SYMLINKS).
func (p ResolvedPath) IsParentOnly() bool {
	return p.mode == resolveModeParentOnly
}

// ContainsPathTraversalSegment checks if a path contains ".." as a distinct path segment
// This avoids false positives for legitimate filenames that contain ".." (e.g., "archive..zip")
func ContainsPathTraversalSegment(path string) bool {
	// Split the path into segments and check if any segment is ".."
	segments := strings.Split(path, string(filepath.Separator))
	return slices.Contains(segments, "..")
}
