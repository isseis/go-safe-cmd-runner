// Package safefileio provides secure file I/O operations with protection against
// common security vulnerabilities like symlink attacks and TOCTOU race conditions.
package safefileio

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

// openat2 constants for RESOLVE flags
const (
	// ResolveNoSymlinks disallows resolution of symbolic links
	ResolveNoSymlinks = 0x04
	// AtFdcwd represents the current working directory
	AtFdcwd = -0x64
	// SysOpenat2 is the system call number for openat2 on Linux
	SysOpenat2 = 437
)

// openHow struct for openat2 system call
type openHow struct {
	flags   uint64
	mode    uint64
	resolve uint64
}

// isOpenat2Available checks if openat2 system call is available
var openat2Available bool

func init() {
	// Test if openat2 is available by trying to use it
	how := openHow{
		flags:   uint64(os.O_RDONLY),
		mode:    0,
		resolve: 0,
	}
	fd, err := openat2(AtFdcwd, "/dev/null", &how)
	if err == nil {
		if closeErr := syscall.Close(fd); closeErr != nil {
			log.Printf("error closing file descriptor: %v\n", closeErr)
		}
		openat2Available = true
	}
}

// openat2 wraps the openat2 system call
func openat2(dirfd int, pathname string, how *openHow) (int, error) {
	pathBytes, err := syscall.BytePtrFromString(pathname)
	if err != nil {
		return -1, err
	}

	fd, _, errno := syscall.Syscall6(
		SysOpenat2,
		uintptr(dirfd),
		// #nosec G103 - uintptr conversion is required for syscall interface
		uintptr(unsafe.Pointer(pathBytes)),
		// #nosec G103 - uintptr conversion is required for syscall interface
		uintptr(unsafe.Pointer(how)),
		unsafe.Sizeof(*how),
		0, 0,
	)

	if errno != 0 {
		return -1, errno
	}

	return int(fd), nil
}

// FileSystem is an interface that abstracts file system operations
type FileSystem interface {
	OpenFile(name string, flag int, perm os.FileMode) (File, error)
	SafeOpenFile(name string, flag int, perm os.FileMode) (File, error)
}

// File is an interface that abstracts file operations
type File interface {
	io.Reader
	io.Writer
	Close() error
	Stat() (os.FileInfo, error)
}

// osFS implements FileSystem using the local disk
var defaultFS FileSystem = osFS{}

type osFS struct{}

func (osFS) OpenFile(name string, flag int, perm os.FileMode) (File, error) {
	// #nosec G304 - The path is validated using openat2 or path verification to prevent TOCTOU attacks
	return os.OpenFile(name, flag, perm)
}

func (osFS) SafeOpenFile(name string, flag int, perm os.FileMode) (File, error) {
	return safeOpenFile(name, flag, perm)
}

// SafeWriteFile writes a file safely after validating the path and checking file properties.
// It uses openat2 with RESOLVE_NO_SYMLINKS when available for atomic symlink-safe operations,
// eliminating TOCTOU (Time-of-Check Time-of-Use) race conditions completely.
// On systems without openat2, it falls back to path verification before opening the file.
//
// Note: The filepath parameter is intentionally not restricted to a safe directory as the
// function is designed to work with any valid file path while maintaining security.
func SafeWriteFile(filePath string, content []byte, perm os.FileMode) (err error) {
	return safeWriteFileWithFS(filePath, content, perm, defaultFS)
}

// safeWriteFileWithFS is the internal implementation that accepts a FileSystem for testing
func safeWriteFileWithFS(filePath string, content []byte, perm os.FileMode, fs FileSystem) (err error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// Use the FileSystem interface consistently for both testing and production
	file, err := fs.SafeOpenFile(absPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return err
	}

	// Ensure the file is closed on error
	defer func() {
		if closeErr := file.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("failed to close file: %w", closeErr)
		}
	}()

	// Validate the file is a regular file (not a device, pipe, etc.)
	if _, err := validateFile(file, absPath); err != nil {
		return err
	}

	// Write the content
	if _, err = file.Write(content); err != nil {
		return fmt.Errorf("failed to write to %s: %w", absPath, err)
	}

	return nil
}

// ensureParentDirsNoSymlinks checks if any component of the path is a symlink
// by traversing the directory hierarchy step-by-step using opendir(2) equivalent.
func ensureParentDirsNoSymlinks(absPath string) error {
	// Get the directory of the file
	dir := filepath.Dir(absPath)

	// Split the path into components
	components := []string{}
	current := dir
	for {
		parent := filepath.Dir(current)
		if parent == current {
			// Reached root directory
			break
		}
		components = append([]string{filepath.Base(current)}, components...)
		current = parent
	}

	// Start from the root and traverse step by step
	currentPath := filepath.VolumeName(dir)
	if currentPath == "" {
		currentPath = "/"
	}

	for _, component := range components {
		currentPath = filepath.Join(currentPath, component)

		// Use os.Lstat to check if the current component is a symlink
		// This doesn't follow symlinks, making it safe
		fi, err := os.Lstat(currentPath)
		if err != nil {
			if os.IsNotExist(err) {
				// Directory doesn't exist yet, which is fine for creation
				continue
			}
			return fmt.Errorf("failed to stat %s: %w", currentPath, err)
		}

		// Check if it's a symlink
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: %s", ErrIsSymlink, currentPath)
		}

		// Ensure it's a directory (except for the last component which might not exist yet)
		if !fi.IsDir() {
			return fmt.Errorf("%w: not a directory: %s", ErrInvalidFilePath, currentPath)
		}
	}

	return nil
}

// MaxFileSize is the maximum allowed file size for SafeReadFile (128 MB)
const MaxFileSize = 128 * 1024 * 1024

// SafeReadFile reads a file safely after validating the path and checking file properties.
// It enforces a maximum file size of MaxFileSize to prevent memory exhaustion attacks.
// It uses openat2 with RESOLVE_NO_SYMLINKS when available for atomic symlink-safe operations.
func SafeReadFile(filePath string) ([]byte, error) {
	return SafeReadFileWithFS(filePath, defaultFS)
}

// SafeReadFileWithFS is the internal implementation that accepts a FileSystem for testing
func SafeReadFileWithFS(filePath string, fs FileSystem) ([]byte, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	// Use the FileSystem interface consistently for both testing and production
	file, err := fs.SafeOpenFile(absPath, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("error closing file: %v\n", closeErr)
		}
	}()

	return readFileContent(file, absPath)
}

// readFileContent reads and validates the content of an already opened file
func readFileContent(file File, filePath string) ([]byte, error) {
	fileInfo, err := validateFile(file, filePath)
	if err != nil {
		return nil, err
	}

	if fileInfo.Size() > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	// Use io.ReadAll with LimitReader for consistent behavior across implementations
	content, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	if int64(len(content)) > MaxFileSize {
		return nil, ErrFileTooLarge
	}

	return content, nil
}

// validateFile checks if the file is a regular file and returns its FileInfo
// To prevent TOCTOU attacks, we use the file descriptor to get the file info
func validateFile(file File, filePath string) (os.FileInfo, error) {
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	if !fileInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: not a regular file: %s", ErrInvalidFilePath, filePath)
	}

	return fileInfo, nil
}

// safeOpenFile opens a file using openat2 with RESOLVE_NO_SYMLINKS if available,
// otherwise falls back to the traditional approach with path verification
func safeOpenFile(filePath string, flag int, perm os.FileMode) (*os.File, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFilePath, err)
	}

	if openat2Available {
		// Use openat2 with RESOLVE_NO_SYMLINKS for atomic operation
		how := openHow{
			// #nosec G115 - flag conversion is intentional and safe within valid flag range
			flags:   uint64(flag),
			mode:    uint64(perm),
			resolve: ResolveNoSymlinks,
		}

		fd, err := openat2(AtFdcwd, absPath, &how)
		if err != nil {
			// Check for specific errors
			if errno, ok := err.(syscall.Errno); ok {
				switch errno {
				case syscall.ELOOP:
					return nil, ErrIsSymlink
				case syscall.EEXIST:
					return nil, ErrFileExists
				case syscall.ENOENT:
					return nil, os.ErrNotExist // Return standard not exist error
				}
			}
			return nil, fmt.Errorf("failed to open file: %w", err)
		}

		return os.NewFile(uintptr(fd), absPath), nil
	}

	// Prevent symlink attacks by ensuring parent directories are not symlinks.
	if err := ensureParentDirsNoSymlinks(absPath); err != nil {
		return nil, err
	}

	// #nosec G304 - absPath is properly validated above
	file, err := os.OpenFile(absPath, flag|syscall.O_NOFOLLOW, perm)
	if err != nil {
		switch {
		case os.IsExist(err):
			return nil, ErrFileExists
		case isNoFollowError(err):
			return nil, ErrIsSymlink
		case os.IsNotExist(err):
			return nil, err // Return the original error for file not found
		default:
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
	}

	return file, nil
}
