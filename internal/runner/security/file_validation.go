package security

import (
	"fmt"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
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
	// Only bypass this check if explicitly configured for permissive testing
	if perm&0o002 != 0 && !v.config.testPermissiveMode {
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
		// Only bypass this check if explicitly configured for permissive testing
		if !v.config.testPermissiveMode && (stat.Uid != UIDRoot || stat.Gid != GIDRoot) {
			return fmt.Errorf("%w: directory %s has group write permissions (%04o) but is not owned by root (uid=%d, gid=%d)",
				ErrInvalidDirPermissions, dirPath, perm, stat.Uid, stat.Gid)
		}
	}

	// Check that only root can write to the directory
	// Only bypass this check if explicitly configured for permissive testing
	if perm&0o200 != 0 && stat.Uid != UIDRoot && !v.config.testPermissiveMode {
		return fmt.Errorf("%w: directory %s is writable by non-root user (uid=%d)",
			ErrInvalidDirPermissions, dirPath, stat.Uid)
	}

	return nil
}

// ValidateOutputWritePermission validates write permission for output file creation
// This method is specifically designed for output capture functionality
// It leverages the existing secure path validation infrastructure to prevent symlink attacks
func (v *Validator) ValidateOutputWritePermission(outputPath string, realUID int) error {
	if outputPath == "" {
		return fmt.Errorf("%w: empty output path", ErrInvalidPath)
	}

	// Ensure absolute path
	if !filepath.IsAbs(outputPath) {
		return fmt.Errorf("%w: output path must be absolute, got: %s", ErrInvalidPath, outputPath)
	}

	cleanPath := filepath.Clean(outputPath)
	dir := filepath.Dir(cleanPath)

	// Use unified validation that combines security validation and write permission checks
	// This efficiently validates the directory hierarchy in a single traversal
	if err := v.validateOutputDirectoryAccesss(dir, realUID); err != nil {
		return fmt.Errorf("directory validation failed: %w", err)
	}

	// If file exists, validate file write permission using secure Lstat
	if fileInfo, err := v.fs.Lstat(cleanPath); err == nil {
		if err := v.validateOutputFileWritePermission(cleanPath, fileInfo, realUID); err != nil {
			return fmt.Errorf("file write permission check failed: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat output file %s: %w", cleanPath, err)
	}

	return nil
}

// validateOutputDirectoryAccesss validates both security and write permissions
// for an output directory path in a single efficient traversal up the directory hierarchy.
// This method combines the functionality of ValidateDirectoryPermissions and write permission checks
// to avoid redundant directory traversals.
func (v *Validator) validateOutputDirectoryAccesss(dirPath string, realUID int) error {
	// Find the first existing directory in the hierarchy
	currentPath := dirPath

	// Walk up the directory tree until we find an existing directory
	for {
		if info, err := v.fs.Lstat(currentPath); err == nil {
			// Directory exists, validate security for complete path
			if err := v.ValidateDirectoryPermissions(currentPath); err != nil {
				return fmt.Errorf("directory security validation failed for %s: %w", currentPath, err)
			}

			// Check write permission for the existing directory (where files will be created)
			if err := v.checkWritePermission(currentPath, info, realUID); err != nil {
				return fmt.Errorf("write permission check failed for %s: %w", currentPath, err)
			}

			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("failed to lstat directory %s: %w", currentPath, err)
		}

		// Directory doesn't exist, move to parent
		parent := filepath.Dir(currentPath)
		if parent == currentPath {
			// Reached filesystem root without finding existing directory
			// Use a wrapped static error instead of a dynamically formatted one
			// so callers can reliably use errors.Is to compare.
			return fmt.Errorf("%w: %s", ErrNoExistingDirectoryInPathHierarchy, dirPath)
		}
		currentPath = parent
	}
}

// validateOutputFileWritePermission checks if the user can write to the existing file
// This function receives fileInfo from Lstat to ensure symlink safety
func (v *Validator) validateOutputFileWritePermission(filePath string, fileInfo os.FileInfo, realUID int) error {
	// Additional symlink check (fileInfo should be from Lstat)
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: output file %s is a symlink", ErrInvalidFilePermissions, filePath)
	}

	if !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", ErrInvalidFilePermissions, filePath)
	}

	return v.checkWritePermission(filePath, fileInfo, realUID)
}

// checkWritePermission performs the actual permission check for a given UID
func (v *Validator) checkWritePermission(path string, stat os.FileInfo, realUID int) error {
	sysstat, ok := stat.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: failed to get system info for %s", ErrInvalidFilePermissions, path)
	}

	// Check owner permissions
	if int(sysstat.Uid) == realUID {
		if stat.Mode()&0o200 != 0 {
			return nil // Owner has write permission
		}
		return fmt.Errorf("%w: owner write permission denied for %s", ErrInvalidFilePermissions, path)
	}

	// Check group permissions
	if stat.Mode()&0o020 != 0 {
		inGroup, err := v.isUserInGroup(realUID, sysstat.Gid)
		if err != nil {
			return fmt.Errorf("failed to check group membership: %w", err)
		}
		if inGroup {
			return nil // User is in group and group has write permission
		}
	}

	// Check other permissions
	if stat.Mode()&0o002 != 0 {
		return nil // Others have write permission
	}

	return fmt.Errorf("%w: write permission denied for %s", ErrInvalidFilePermissions, path)
}

// isUserInGroup checks if a user (by UID) is a member of a group (by GID)
// This is a simplified version that checks primary group and supplementary groups
// Returns (inGroup, error) where error indicates system-level failures
func (v *Validator) isUserInGroup(uid int, gid uint32) (bool, error) {
	// Get user information
	user, err := user.LookupId(strconv.Itoa(uid))
	if err != nil {
		return false, fmt.Errorf("failed to lookup user %d: %w", uid, err)
	}

	// Check primary group
	userGid, err := strconv.ParseUint(user.Gid, 10, 32)
	if err != nil {
		return false, fmt.Errorf("failed to parse user's primary GID %s: %w", user.Gid, err)
	}
	if uint32(userGid) == gid {
		return true, nil
	}

	// Check supplementary groups using groupmembership
	if v.groupMembership != nil {
		members, err := v.groupMembership.GetGroupMembers(gid)
		if err != nil {
			return false, fmt.Errorf("failed to get group members for GID %d: %w", gid, err)
		}
		for _, member := range members {
			if member == user.Username {
				return true, nil
			}
		}
	}

	return false, nil
}

// EvaluateOutputSecurityRisk evaluates the security risk level for an output path
// This method provides centralized security risk assessment for output capture functionality
//
// Requirements:
// - workDir must be absolute and cleaned (filepath.Clean) when provided
// - Passing non-absolute or non-clean workDir indicates a programming error and returns an error
// - Passing empty path indicates a programming error and returns an error
func (v *Validator) EvaluateOutputSecurityRisk(path, workDir string) (runnertypes.RiskLevel, error) {
	// Validate workDir requirements - programming error if violated
	if workDir != "" {
		if !filepath.IsAbs(workDir) {
			// Programming error: workDir must be absolute
			return runnertypes.RiskLevelUnknown, fmt.Errorf("%w: workDir must be absolute, got: %s", ErrInvalidPath, workDir)
		}
		if filepath.Clean(workDir) != workDir {
			// Programming error: workDir must be pre-cleaned
			return runnertypes.RiskLevelUnknown, fmt.Errorf("%w: workDir must be pre-cleaned, got: %s", ErrInvalidPath, workDir)
		}
	}

	// Handle empty path as a programming error
	if path == "" {
		return runnertypes.RiskLevelUnknown, fmt.Errorf("%w: empty path provided", ErrInvalidPath)
	}

	var cleanPath string

	// Handle relative paths by resolving them against workDir
	if !filepath.IsAbs(path) {
		if workDir == "" {
			// Cannot resolve relative path without workDir
			return runnertypes.RiskLevelHigh, nil
		}
		cleanPath = filepath.Clean(filepath.Join(workDir, path))
	} else {
		cleanPath = filepath.Clean(path)
	}

	pathLower := strings.ToLower(cleanPath)

	// Critical: System important files and patterns (hardcoded for robustness)
	for _, pattern := range v.config.OutputCriticalPathPatterns {
		if strings.Contains(pathLower, strings.ToLower(pattern)) {
			return runnertypes.RiskLevelCritical, nil
		}
	}

	// High: System directories and high-risk patterns (hardcoded for robustness)
	for _, pattern := range v.config.OutputHighRiskPathPatterns {
		if strings.Contains(pathLower, strings.ToLower(pattern)) {
			return runnertypes.RiskLevelHigh, nil
		}
	}

	// Low: WorkDir internal files
	if workDir != "" && filepath.IsAbs(workDir) {
		cleanWorkDir := filepath.Clean(workDir)
		if strings.HasPrefix(cleanPath, cleanWorkDir) {
			return runnertypes.RiskLevelLow, nil
		}
	}

	// Low: Current user's home directory
	if currentUser, err := user.Current(); err == nil {
		homeDir := currentUser.HomeDir
		if homeDir != "" && filepath.IsAbs(homeDir) {
			cleanHomePath := filepath.Clean(homeDir)
			if strings.HasPrefix(cleanPath, cleanHomePath) {
				return runnertypes.RiskLevelLow, nil
			}
		}
	}

	// Medium: Other locations
	return runnertypes.RiskLevelMedium, nil
}
