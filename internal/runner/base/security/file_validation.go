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

	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
)

// validatePathAndGetInfo validates and cleans a path, then returns its file info
func (v *Validator) validatePathAndGetInfo(path, pathType string) (string, os.FileInfo, error) {
	if path == "" {
		slog.Error("Empty " + pathType + " path provided for permission validation")
		return "", nil, fmt.Errorf("%w: empty path", isec.ErrInvalidPath)
	}
	if !filepath.IsAbs(path) {
		err := fmt.Errorf("%w: path must be absolute, got relative path: %s", isec.ErrInvalidPath, path)
		slog.Error("Path validation failed", slog.String("path", path), slog.Any("error", err))
		return "", nil, err
	}

	// Clean and validate the path
	cleanPath := filepath.Clean(path)
	slog.Debug("Validating "+pathType+" permissions", slog.String("path", cleanPath))

	if len(cleanPath) > v.config.MaxPathLength {
		err := fmt.Errorf("%w: path too long (%d > %d)", isec.ErrInvalidPath, len(cleanPath), v.config.MaxPathLength)
		slog.Error("Path validation failed", slog.String("path", cleanPath), slog.Any("error", err), slog.Int("max_length", v.config.MaxPathLength))
		return "", nil, err
	}

	// Get file info
	fileInfo, err := v.fs.Lstat(cleanPath)
	if err != nil {
		slog.Error("Failed to get "+pathType+" info", slog.String("path", cleanPath), slog.Any("error", err))
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
		slog.Warn("Invalid file type", slog.String("path", cleanPath), slog.String("mode", fileInfo.Mode().String()))
		return err
	}

	perm := fileInfo.Mode().Perm()
	requiredPerms := v.config.RequiredFilePermissions
	pathType := "file"

	slog.Debug("Checking "+pathType+" permissions", slog.String("path", cleanPath), slog.String("current_permissions", fmt.Sprintf("%04o", perm)), slog.String("max_allowed", fmt.Sprintf("%04o", requiredPerms)))

	disallowedBits := perm &^ requiredPerms
	if disallowedBits != 0 {
		err := fmt.Errorf(
			"%w: %s %s has permissions %o with disallowed bits %o, maximum allowed is %o",
			ErrInvalidFilePermissions, pathType, cleanPath, perm, disallowedBits, requiredPerms)

		slog.Warn(
			"Insecure "+pathType+" permissions detected",
			slog.String("path", cleanPath),
			slog.String("current_permissions", fmt.Sprintf("%04o", perm)),
			slog.String("disallowed_bits", fmt.Sprintf("%04o", disallowedBits)),
			slog.String("max_allowed", fmt.Sprintf("%04o", requiredPerms)))

		return err
	}

	slog.Debug(pathType+" permissions validated successfully", slog.String("path", cleanPath), slog.String("permissions", fmt.Sprintf("%04o", perm)))
	return nil
}

// ValidateDirectoryPermissions validates that a directory has appropriate permissions
// and checks the complete path from root to target for security.
func (v *Validator) ValidateDirectoryPermissions(dirPath string) error {
	return isec.ValidateDirectoryPermissionsWithOptions(dirPath, v.buildDirPermOpts(os.Getuid()))
}

func (v *Validator) buildDirPermOpts(realUID int) isec.DirectoryPermCheckOptions {
	opts := isec.DirectoryPermCheckOptions{
		Lstat:              v.fs.Lstat,
		MaxPathLength:      v.config.MaxPathLength,
		RealUID:            realUID,
		TestPermissiveMode: v.config.testPermissiveMode,
		IsTrustedGroup: func(gid uint32) bool {
			return v.isTrustedGroup(gid)
		},
	}
	if v.groupMembership != nil {
		opts.CanUserSafelyWrite = func(uid int, ownerUID uint32, groupGID uint32, mode os.FileMode) (bool, error) {
			return v.groupMembership.CanUserSafelyWriteFile(uid, ownerUID, groupGID, mode)
		}
	}
	return opts
}

// ValidateOutputWritePermission validates write permission for output file creation
// This method is specifically designed for output capture functionality
// It leverages the existing secure path validation infrastructure to prevent symlink attacks
func (v *Validator) ValidateOutputWritePermission(outputPath string, realUID int) error {
	if outputPath == "" {
		return fmt.Errorf("%w: empty output path", isec.ErrInvalidPath)
	}

	// Ensure absolute path
	if !filepath.IsAbs(outputPath) {
		return fmt.Errorf("%w: output path must be absolute, got: %s", isec.ErrInvalidPath, outputPath)
	}

	cleanPath := filepath.Clean(outputPath)
	dir := filepath.Dir(cleanPath)

	// Use unified validation that combines security validation and write permission checks
	// This efficiently validates the directory hierarchy in a single traversal
	if err := v.validateOutputDirectoryAccess(dir, realUID); err != nil {
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

// validateOutputDirectoryAccess validates both security and write permissions
// for an output directory path in a single efficient traversal up the directory hierarchy.
// This method combines the functionality of ValidateDirectoryPermissions and write permission checks
// to avoid redundant directory traversals.
func (v *Validator) validateOutputDirectoryAccess(dirPath string, realUID int) error {
	// Find the first existing directory in the hierarchy
	currentPath := dirPath

	// Walk up the directory tree until we find an existing directory
	for {
		if _, err := v.fs.Lstat(currentPath); err == nil {
			if err := v.validateAllowedOutputPathSymlinks(currentPath); err != nil {
				return fmt.Errorf("directory security validation failed for %s: %w", currentPath, err)
			}

			// Resolve symlinks to get the canonical path for security validation.
			// On some systems (e.g. macOS), system paths like /tmp are symlinks
			// (e.g. /tmp -> /private/tmp). EvalSymlinks resolves the real path so
			// that validateCompletePath can verify each component without tripping
			// over OS-provided symlinks.
			resolvedPath, err := v.fs.EvalSymlinks(currentPath)
			if err != nil {
				return fmt.Errorf("failed to resolve path %s: %w", currentPath, err)
			}

			resolvedInfo, err := v.fs.Lstat(resolvedPath)
			if err != nil {
				return fmt.Errorf("failed to stat resolved path %s: %w", resolvedPath, err)
			}

			// Directory exists, validate security for complete path with realUID context.
			if err := isec.ValidateDirectoryPermissionsWithOptions(resolvedPath, v.buildDirPermOpts(realUID)); err != nil {
				return fmt.Errorf("directory security validation failed for %s: %w", currentPath, err)
			}

			// Check write permission for the existing directory (where files will be created)
			if err := v.checkWritePermission(resolvedPath, resolvedInfo, realUID); err != nil {
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

// validateAllowedOutputPathSymlinks rejects user-controlled symlink components in
// output paths while allowing specific OS-managed aliases on macOS.
func (v *Validator) validateAllowedOutputPathSymlinks(path string) error {
	for currentPath := filepath.Clean(path); ; {
		info, err := v.fs.Lstat(currentPath)
		if err != nil {
			return fmt.Errorf("failed to stat path component %s: %w", currentPath, err)
		}

		if info.Mode()&os.ModeSymlink != 0 && !isAllowedOSManagedSymlink(currentPath) {
			return fmt.Errorf("%w: path component %s is a symlink", isec.ErrInsecurePathComponent, currentPath)
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return nil
		}
		currentPath = parentPath
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
		if v.groupMembership != nil {
			inGroup, err := v.isUserInGroup(realUID, sysstat.Gid)
			if err != nil {
				return fmt.Errorf("failed to check group membership: %w", err)
			}
			if inGroup {
				return nil // User is in group and group has write permission
			}
		}
	}

	// Check other permissions (world-writable check)
	// Exception: If sticky bit is set (like /tmp), world-writable is safe for directories
	// Only allow world-writable access in permissive test mode for security
	if stat.Mode()&0o002 != 0 {
		if !v.config.testPermissiveMode {
			// For directories with sticky bit, world-writable is acceptable
			if stat.Mode().IsDir() && stat.Mode()&os.ModeSticky != 0 {
				slog.Debug("Directory is world-writable but has sticky bit set (safe)",
					slog.String("path", path),
					slog.String("permissions", fmt.Sprintf("%04o", stat.Mode().Perm())))
				return nil
			}
			// Distinguish between directories and files in error messages
			if stat.Mode().IsDir() {
				slog.Error("Directory writable by others detected",
					slog.String("path", path),
					slog.String("permissions", fmt.Sprintf("%04o", stat.Mode().Perm())),
					slog.Int("uid", realUID))
				return fmt.Errorf("%w: directory %s is writable by others (%04o), which poses security risks",
					isec.ErrInvalidDirPermissions, path, stat.Mode().Perm())
			}
			slog.Error("File writable by others detected",
				slog.String("path", path),
				slog.String("permissions", fmt.Sprintf("%04o", stat.Mode().Perm())),
				slog.Int("uid", realUID))
			return fmt.Errorf("%w: file %s is writable by others (%04o), which poses security risks",
				ErrInvalidFilePermissions, path, stat.Mode().Perm())
		}
		// In permissive test mode, allow world-writable access
		pathType := "file"
		if stat.Mode().IsDir() {
			pathType = "directory"
		}
		slog.Warn("Allowing world-writable access in test mode",
			slog.String("path", path),
			slog.String("path_type", pathType),
			slog.String("permissions", fmt.Sprintf("%04o", stat.Mode().Perm())),
			slog.Int("uid", realUID))
		return nil
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
	members, err := v.groupMembership.GetGroupMembers(gid)
	if err != nil {
		return false, fmt.Errorf("failed to get group members for GID %d: %w", gid, err)
	}
	for _, member := range members {
		if member == user.Username {
			return true, nil
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
			return runnertypes.RiskLevelUnknown, fmt.Errorf("%w: workDir must be absolute, got: %s", isec.ErrInvalidPath, workDir)
		}
		if filepath.Clean(workDir) != workDir {
			// Programming error: workDir must be pre-cleaned
			return runnertypes.RiskLevelUnknown, fmt.Errorf("%w: workDir must be pre-cleaned, got: %s", isec.ErrInvalidPath, workDir)
		}
	}

	// Handle empty path as a programming error
	if path == "" {
		return runnertypes.RiskLevelUnknown, fmt.Errorf("%w: empty path provided", isec.ErrInvalidPath)
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
		if strings.HasPrefix(cleanPath, workDir) {
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
