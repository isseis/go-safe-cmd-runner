package security

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
)

// validatePathAndGetInfo validates and cleans a path, then returns its file info
func (v *Validator) validatePathAndGetInfo(path, pathType string) (string, os.FileInfo, error) {
	if path == "" {
		slog.Error("Empty " + pathType + " path provided for permission validation")
		return "", nil, fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	if !filepath.IsAbs(path) {
		err := fmt.Errorf("%w: path must be absolute, got relative path: %s", ErrInvalidPath, path)
		slog.Error("Path validation failed", "path", path, "error", err)
		return "", nil, err
	}

	// Clean and validate the path
	cleanPath := filepath.Clean(path)
	slog.Debug("Validating "+pathType+" permissions", "path", cleanPath)

	if len(cleanPath) > v.config.MaxPathLength {
		err := fmt.Errorf("%w: path too long (%d > %d)", ErrInvalidPath, len(cleanPath), v.config.MaxPathLength)
		slog.Error("Path validation failed", "path", cleanPath, "error", err, "max_length", v.config.MaxPathLength)
		return "", nil, err
	}

	// Get file info
	fileInfo, err := v.fs.Lstat(cleanPath)
	if err != nil {
		slog.Error("Failed to get "+pathType+" info", "path", cleanPath, "error", err)
		return "", nil, fmt.Errorf("failed to stat %s: %w", cleanPath, err)
	}

	return cleanPath, fileInfo, nil
}

// ValidateFilePermissions validates that a file has appropriate permissions
func (v *Validator) ValidateFilePermissions(filePath string) error {
	cleanPath, fileInfo, err := v.validatePathAndGetInfo(filePath, "file")
	if err != nil {
		return err
	}

	// Check if it's a regular file
	if !fileInfo.Mode().IsRegular() {
		err := fmt.Errorf("%w: %s is not a regular file", ErrInvalidFilePermissions, cleanPath)
		slog.Warn("Invalid file type", "path", cleanPath, "mode", fileInfo.Mode().String())
		return err
	}

	perm := fileInfo.Mode().Perm()
	requiredPerms := v.config.RequiredFilePermissions
	pathType := "file"

	slog.Debug("Checking "+pathType+" permissions", "path", cleanPath, "current_permissions", fmt.Sprintf("%04o", perm), "max_allowed", fmt.Sprintf("%04o", requiredPerms))

	disallowedBits := perm &^ requiredPerms
	if disallowedBits != 0 {
		err := fmt.Errorf(
			"%w: %s %s has permissions %o with disallowed bits %o, maximum allowed is %o",
			ErrInvalidFilePermissions, pathType, cleanPath, perm, disallowedBits, requiredPerms)

		slog.Warn(
			"Insecure "+pathType+" permissions detected",
			"path", cleanPath,
			"current_permissions", fmt.Sprintf("%04o", perm),
			"disallowed_bits", fmt.Sprintf("%04o", disallowedBits),
			"max_allowed", fmt.Sprintf("%04o", requiredPerms))

		return err
	}

	slog.Debug(pathType+" permissions validated successfully", "path", cleanPath, "permissions", fmt.Sprintf("%04o", perm))
	return nil
}

// ValidateDirectoryPermissions validates that a directory has appropriate permissions
// and checks the complete path from root to target for security
func (v *Validator) ValidateDirectoryPermissions(dirPath string) error {
	cleanDir, dirInfo, err := v.validatePathAndGetInfo(dirPath, "directory")
	if err != nil {
		return err
	}

	// Check if it's a directory
	if !dirInfo.Mode().IsDir() {
		err := fmt.Errorf("%w: %s is not a directory", ErrInvalidDirPermissions, dirPath)
		slog.Warn("Invalid directory type", "path", dirPath, "mode", dirInfo.Mode().String())
		return err
	}

	// SECURITY: Validate complete path from root to target directory
	// This prevents attacks through compromised intermediate directories
	return v.validateCompletePath(cleanDir, dirPath)
}

// validateCompletePath validates the security of the complete path from root to target
// This prevents attacks through compromised intermediate directories
// cleanDir must be absolute and cleaned.
func (v *Validator) validateCompletePath(cleanPath string, originalPath string) error {
	slog.Debug("Validating complete path security", "target_path", originalPath)

	// Validate each directory component from target to root
	for currentPath := cleanPath; ; {
		slog.Debug("Validating path component", "component_path", currentPath)

		info, err := v.fs.Lstat(currentPath)
		if err != nil {
			slog.Error("Failed to stat path component", "path", currentPath, "error", err)
			return fmt.Errorf("failed to stat path component %s: %w", currentPath, err)
		}

		if err := v.validateDirectoryComponentMode(currentPath, info); err != nil {
			return err
		}
		if err := v.validateDirectoryComponentPermissions(currentPath, info); err != nil {
			return err
		}

		// Move to parent directory, or break if we reached root
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			break
		}
		currentPath = parentPath
	}

	slog.Debug("Complete path validation successful", "original_path", originalPath, "final_path", cleanPath)
	return nil
}

// validateDirectoryComponentMode validates that a directory component is a directory and not a symlink
func (v *Validator) validateDirectoryComponentMode(dirPath string, info os.FileInfo) error {
	// Check if the component is not a symlink
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: path component %s is a symlink", ErrInsecurePathComponent, dirPath)
	}

	// Ensure the component is a directory
	if !info.Mode().IsDir() {
		return fmt.Errorf("%w: path component %s is not a directory", ErrInsecurePathComponent, dirPath)
	}
	return nil
}

// validateDirectoryComponentPermissions validates that a directory component has secure permissions
// info parameter should be the FileInfo for the directory at dirPath to avoid redundant filesystem calls
func (v *Validator) validateDirectoryComponentPermissions(dirPath string, info os.FileInfo) error {
	// Get system-level file info for ownership checks
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: failed to get system info for directory %s", ErrInsecurePathComponent, dirPath)
	}

	perm := info.Mode().Perm()

	// Check that other users cannot write (world-writable check)
	if perm&0o002 != 0 {
		slog.Error("Directory writable by others detected",
			"path", dirPath,
			"permissions", fmt.Sprintf("%04o", perm))
		return fmt.Errorf("%w: directory %s is writable by others (%04o)",
			ErrInvalidDirPermissions, dirPath, perm)
	}

	// Check that group cannot write unless owned by root
	if perm&0o020 != 0 {
		slog.Error("Directory has group write permissions",
			"path", dirPath,
			"permissions", fmt.Sprintf("%04o", perm),
			"owner_uid", stat.Uid,
			"owner_gid", stat.Gid)
		// Only allow group write if owned by root (uid=0) and group (gid=0)
		if stat.Uid != UIDRoot || stat.Gid != GIDRoot {
			return fmt.Errorf("%w: directory %s has group write permissions (%04o) but is not owned by root (uid=%d, gid=%d)",
				ErrInvalidDirPermissions, dirPath, perm, stat.Uid, stat.Gid)
		}
	}

	// Check that only root can write to the directory
	if perm&0o200 != 0 && stat.Uid != UIDRoot {
		return fmt.Errorf("%w: directory %s is writable by non-root user (uid=%d)",
			ErrInvalidDirPermissions, dirPath, stat.Uid)
	}

	return nil
}
