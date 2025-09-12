package hashdir

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

// ErrDefaultHashDirectoryNotAbsolute is returned when DefaultHashDirectory is not an absolute path
var ErrDefaultHashDirectoryNotAbsolute = fmt.Errorf("default hash directory must be absolute path")

// GetHashDir determines the hash directory based on command line args and default value
func GetHashDir(hashDirectory *string, defaultHashDirectory string) string {
	// Command line arguments take precedence over environment variables
	if hashDirectory != nil && *hashDirectory != "" {
		return *hashDirectory
	}
	// Set default hash directory if none specified
	return defaultHashDirectory
}

// ValidateSecurely validates hash directory with security checks
func ValidateSecurely(path string) (string, error) {
	// Check if path is absolute
	if !filepath.IsAbs(path) {
		return "", &HashDirectoryError{
			Type: HashDirectoryErrorTypeRelativePath,
			Path: path,
		}
	}

	// Clean the absolute path
	cleanPath := filepath.Clean(path)

	// Use safefileio pattern: recursively validate all parent directories for symlink attacks
	// This approach mirrors ensureParentDirsNoSymlinks but includes the target directory itself
	if err := validatePathComponentsSecurely(cleanPath); err != nil {
		// Convert safefileio errors to HashDirectoryError
		if errors.Is(err, safefileio.ErrIsSymlink) {
			return "", &HashDirectoryError{
				Type:  HashDirectoryErrorTypeSymlinkAttack,
				Path:  cleanPath,
				Cause: err,
			}
		}
		if errors.Is(err, safefileio.ErrInvalidFilePath) {
			return "", &HashDirectoryError{
				Type:  HashDirectoryErrorTypeNotDirectory,
				Path:  cleanPath,
				Cause: err,
			}
		}
		// Check for NotExist errors in the wrapped error
		var pathErr *os.PathError
		if errors.As(err, &pathErr) && os.IsNotExist(pathErr.Err) {
			return "", &HashDirectoryError{
				Type:  HashDirectoryErrorTypeNotFound,
				Path:  cleanPath,
				Cause: err,
			}
		}
		return "", &HashDirectoryError{
			Type:  HashDirectoryErrorTypePermission,
			Path:  cleanPath,
			Cause: err,
		}
	}

	return cleanPath, nil
}

// validatePathComponentsSecurely validates all path components from root to target
// using the same secure approach as safefileio.ensureParentDirsNoSymlinks
func validatePathComponentsSecurely(absPath string) error {
	// Split path into components for step-by-step validation
	components := splitHashDirPathComponents(absPath)

	// Start from root and validate each component
	currentPath := filepath.VolumeName(absPath) + string(os.PathSeparator)

	for _, component := range components {
		currentPath = filepath.Join(currentPath, component)

		// Use os.Lstat to detect symlinks without following them
		fi, err := os.Lstat(currentPath)
		if err != nil {
			return fmt.Errorf("failed to validate path component %s: %w", currentPath, err)
		}

		// Reject any symlinks in the path hierarchy
		if fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink found in path: %s", safefileio.ErrIsSymlink, currentPath)
		}

		// Ensure each component is a directory
		if !fi.IsDir() {
			return fmt.Errorf("%w: path component is not a directory: %s", safefileio.ErrInvalidFilePath, currentPath)
		}
	}

	return nil
}

// splitHashDirPathComponents splits directory path into components
// Similar to safefileio.splitPathComponents but includes target directory
func splitHashDirPathComponents(dirPath string) []string {
	components := []string{}
	current := dirPath

	for {
		parent := filepath.Dir(current)
		if parent == current {
			// Reached root directory
			break
		}

		components = append(components, filepath.Base(current))
		current = parent
	}

	// Reverse slice to get root-to-target order
	for i, j := 0, len(components)-1; i < j; i, j = i+1, j-1 {
		components[i], components[j] = components[j], components[i]
	}

	return components
}

// ValidateDefaultHashDirectory validates that the provided default hash directory is an absolute path
func ValidateDefaultHashDirectory(defaultHashDirectory string) error {
	if !filepath.IsAbs(defaultHashDirectory) {
		return fmt.Errorf("%w, got: %s", ErrDefaultHashDirectoryNotAbsolute, defaultHashDirectory)
	}
	return nil
}

// GetWithValidation determines hash directory with priority-based resolution and validation
func GetWithValidation(hashDirectory *string, defaultHashDirectory string) (string, error) {
	var path string

	// Priority 1: Command line argument
	if hashDirectory != nil && *hashDirectory != "" {
		path = *hashDirectory
	} else if envPath := os.Getenv("HASH_DIRECTORY"); envPath != "" {
		// Priority 2: Environment variable
		path = envPath
	} else {
		// Priority 3: Default value (already validated at startup)
		path = defaultHashDirectory
	}

	// Validate the resolved path securely
	return ValidateSecurely(path)
}
