//go:build !windows

package security

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
)

const darwinAdminGID uint32 = 80

// dirPermChecker is a standalone implementation using the real OS file system.
type dirPermChecker struct {
	gm *groupmembership.GroupMembership
}

// DirectoryPermCheckOptions configures shared directory permission validation.
type DirectoryPermCheckOptions struct {
	Lstat              func(path string) (os.FileInfo, error)
	MaxPathLength      int
	RealUID            int
	TestPermissiveMode bool
	IsTrustedGroup     func(gid uint32) bool
	CanUserSafelyWrite func(realUID int, ownerUID uint32, groupGID uint32, mode os.FileMode) (bool, error)
}

// NewDirectoryPermChecker creates a standalone DirectoryPermChecker.
func NewDirectoryPermChecker() (DirectoryPermChecker, error) {
	return &dirPermChecker{gm: groupmembership.New()}, nil
}

// ValidateDirectoryPermissions validates directory permissions from root to target.
func (d *dirPermChecker) ValidateDirectoryPermissions(dirPath string) error {
	realUID := os.Getuid()
	opts := DirectoryPermCheckOptions{
		Lstat:          os.Lstat,
		MaxPathLength:  DefaultMaxPathLength,
		RealUID:        realUID,
		IsTrustedGroup: isTrustedGroup,
	}
	if d.gm != nil {
		opts.CanUserSafelyWrite = func(uid int, ownerUID uint32, groupGID uint32, mode os.FileMode) (bool, error) {
			return d.gm.CanUserSafelyWriteFile(uid, ownerUID, groupGID, mode)
		}
	}
	return ValidateDirectoryPermissionsWithOptions(dirPath, opts)
}

// ValidateDirectoryPermissionsWithOptions validates directory permissions from
// root to target using injected dependencies.
func ValidateDirectoryPermissionsWithOptions(dirPath string, opts DirectoryPermCheckOptions) error {
	if opts.Lstat == nil {
		return fmt.Errorf("%w: lstat function is required", ErrInvalidPath)
	}

	if dirPath == "" {
		slog.Error("Empty directory path provided for permission validation")
		return fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	if !filepath.IsAbs(dirPath) {
		err := fmt.Errorf("%w: path must be absolute, got relative path: %s", ErrInvalidPath, dirPath)
		slog.Error("Path validation failed", slog.String("path", dirPath), slog.Any("error", err))
		return err
	}

	cleanPath := filepath.Clean(dirPath)
	maxPathLength := opts.MaxPathLength
	if maxPathLength <= 0 {
		maxPathLength = DefaultMaxPathLength
	}
	if len(cleanPath) > maxPathLength {
		err := fmt.Errorf("%w: path too long (%d > %d)", ErrInvalidPath, len(cleanPath), maxPathLength)
		slog.Error("Path validation failed", slog.String("path", cleanPath), slog.Any("error", err), slog.Int("max_length", maxPathLength))
		return err
	}

	fileInfo, err := opts.Lstat(cleanPath)
	if err != nil {
		slog.Error("Failed to get directory info", slog.String("path", cleanPath), slog.Any("error", err))
		return fmt.Errorf("failed to stat %s: %w", cleanPath, err)
	}
	if !fileInfo.Mode().IsDir() {
		err := fmt.Errorf("%w: %s is not a directory", ErrInvalidDirPermissions, dirPath)
		slog.Warn("Invalid directory type", slog.String("path", dirPath), slog.String("mode", fileInfo.Mode().String()))
		return err
	}

	return validateDirectoryHierarchy(cleanPath, dirPath, fileInfo, opts)
}

func validateDirectoryHierarchy(cleanPath string, originalPath string, firstInfo os.FileInfo, opts DirectoryPermCheckOptions) error {
	slog.Debug("Validating complete path security with UID context", slog.String("target_path", originalPath), slog.Int("realUID", opts.RealUID))

	currentPath := cleanPath
	info := firstInfo
	for {
		if err := validateDirectoryComponentMode(currentPath, info); err != nil {
			return err
		}
		if err := validateDirectoryComponentPermissions(currentPath, info, opts); err != nil {
			return err
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			break
		}
		currentPath = parentPath

		var err error
		info, err = opts.Lstat(currentPath)
		if err != nil {
			slog.Error("Failed to stat path component", slog.String("path", currentPath), slog.Any("error", err))
			return fmt.Errorf("failed to stat path component %s: %w", currentPath, err)
		}
	}

	return nil
}

func validateDirectoryComponentMode(dirPath string, info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: path component %s is a symlink", ErrInsecurePathComponent, dirPath)
	}
	if !info.Mode().IsDir() {
		return fmt.Errorf("%w: path component %s is not a directory", ErrInsecurePathComponent, dirPath)
	}
	return nil
}

func isStickyDirectoryMode(info os.FileInfo) bool {
	return info.Mode().IsDir() && info.Mode()&os.ModeSticky != 0
}

func validateDirectoryComponentPermissions(dirPath string, info os.FileInfo, opts DirectoryPermCheckOptions) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: failed to get system info for directory %s", ErrInsecurePathComponent, dirPath)
	}

	perm := info.Mode().Perm()

	if perm&0o002 != 0 && !opts.TestPermissiveMode {
		if isStickyDirectoryMode(info) {
			slog.Debug("Directory is world-writable but has sticky bit set (safe)",
				slog.String("path", dirPath),
				slog.String("permissions", fmt.Sprintf("%04o", perm)))
		} else {
			return fmt.Errorf("%w: directory %s is writable by others (%04o)", ErrInvalidDirPermissions, dirPath, perm)
		}
	}

	if perm&0o020 != 0 && (perm&0o002 == 0 || !isStickyDirectoryMode(info)) {
		if err := validateGroupWritePermissions(dirPath, info, opts); err != nil {
			return err
		}
	}

	if perm&0o200 != 0 && !opts.TestPermissiveMode {
		if stat.Uid != UIDRoot && int(stat.Uid) != opts.RealUID {
			return fmt.Errorf("%w: directory %s is owned by UID %d but execution user is UID %d", ErrInvalidDirPermissions, dirPath, stat.Uid, opts.RealUID)
		}
	}

	return nil
}

func validateGroupWritePermissions(dirPath string, info os.FileInfo, opts DirectoryPermCheckOptions) error {
	if opts.TestPermissiveMode {
		return nil
	}

	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: failed to get system info for directory %s", ErrInsecurePathComponent, dirPath)
	}

	perm := info.Mode().Perm()
	if stat.Uid == UIDRoot && opts.IsTrustedGroup != nil && opts.IsTrustedGroup(stat.Gid) {
		return nil
	}

	if opts.CanUserSafelyWrite == nil {
		return fmt.Errorf("%w: directory %s has group write permissions (%04o) but group membership cannot be verified", ErrInvalidDirPermissions, dirPath, perm)
	}

	canSafelyWrite, err := opts.CanUserSafelyWrite(opts.RealUID, stat.Uid, stat.Gid, info.Mode())
	if err != nil {
		return fmt.Errorf("%w: directory %s failed security validation: %v", ErrInvalidDirPermissions, dirPath, err)
	}
	if !canSafelyWrite {
		return fmt.Errorf("%w: directory %s - user UID %d cannot safely write to this directory", ErrInvalidDirPermissions, dirPath, opts.RealUID)
	}

	return nil
}

func isTrustedGroup(gid uint32) bool {
	if gid == GIDRoot {
		return true
	}

	if runtime.GOOS == "darwin" && gid == darwinAdminGID {
		return true
	}

	return false
}
