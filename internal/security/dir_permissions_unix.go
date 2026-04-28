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

const (
	// UIDRoot is the root user ID.
	UIDRoot = 0
	// GIDRoot is the root group ID.
	GIDRoot = 0
)

const darwinAdminGID uint32 = 80

// dirPermChecker is a standalone implementation using the real OS file system.
type dirPermChecker struct {
	gm *groupmembership.GroupMembership
}

// NewDirectoryPermChecker creates a standalone DirectoryPermChecker.
func NewDirectoryPermChecker() (DirectoryPermChecker, error) {
	return &dirPermChecker{gm: groupmembership.New()}, nil
}

// ValidateDirectoryPermissions validates directory permissions from root to target.
func (d *dirPermChecker) ValidateDirectoryPermissions(dirPath string) error {
	cleanDir, dirInfo, err := d.validatePathAndGetInfo(dirPath)
	if err != nil {
		return err
	}

	if !dirInfo.Mode().IsDir() {
		err := fmt.Errorf("%w: %s is not a directory", ErrInvalidDirPermissions, dirPath)
		slog.Warn("Invalid directory type", slog.String("path", dirPath), slog.String("mode", dirInfo.Mode().String()))
		return err
	}

	realUID := os.Getuid()
	return d.validateCompletePath(cleanDir, dirPath, realUID)
}

func (d *dirPermChecker) validatePathAndGetInfo(path string) (string, os.FileInfo, error) {
	if path == "" {
		slog.Error("Empty directory path provided for permission validation")
		return "", nil, fmt.Errorf("%w: empty path", ErrInvalidPath)
	}
	if !filepath.IsAbs(path) {
		err := fmt.Errorf("%w: path must be absolute, got relative path: %s", ErrInvalidPath, path)
		slog.Error("Path validation failed", slog.String("path", path), slog.Any("error", err))
		return "", nil, err
	}

	cleanPath := filepath.Clean(path)
	if len(cleanPath) > DefaultMaxPathLength {
		err := fmt.Errorf("%w: path too long (%d > %d)", ErrInvalidPath, len(cleanPath), DefaultMaxPathLength)
		slog.Error("Path validation failed", slog.String("path", cleanPath), slog.Any("error", err), slog.Int("max_length", DefaultMaxPathLength))
		return "", nil, err
	}

	fileInfo, err := os.Lstat(cleanPath)
	if err != nil {
		slog.Error("Failed to get directory info", slog.String("path", cleanPath), slog.Any("error", err))
		return "", nil, fmt.Errorf("failed to stat %s: %w", cleanPath, err)
	}

	return cleanPath, fileInfo, nil
}

func (d *dirPermChecker) validateCompletePath(cleanPath string, originalPath string, realUID int) error {
	slog.Debug("Validating complete path security with UID context", slog.String("target_path", originalPath), slog.Int("realUID", realUID))

	for currentPath := cleanPath; ; {
		info, err := os.Lstat(currentPath)
		if err != nil {
			slog.Error("Failed to stat path component", slog.String("path", currentPath), slog.Any("error", err))
			return fmt.Errorf("failed to stat path component %s: %w", currentPath, err)
		}

		if err := validateDirectoryComponentMode(currentPath, info); err != nil {
			return err
		}
		if err := d.validateDirectoryComponentPermissions(currentPath, info, realUID); err != nil {
			return err
		}

		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			break
		}
		currentPath = parentPath
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

func isStickyDirectory(info os.FileInfo) bool {
	return info.Mode().IsDir() && info.Mode()&os.ModeSticky != 0
}

func (d *dirPermChecker) validateDirectoryComponentPermissions(dirPath string, info os.FileInfo, realUID int) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: failed to get system info for directory %s", ErrInsecurePathComponent, dirPath)
	}

	perm := info.Mode().Perm()

	if perm&0o002 != 0 {
		if isStickyDirectory(info) {
			slog.Debug("Directory is world-writable but has sticky bit set (safe)",
				slog.String("path", dirPath),
				slog.String("permissions", fmt.Sprintf("%04o", perm)))
		} else {
			return fmt.Errorf("%w: directory %s is writable by others (%04o)", ErrInvalidDirPermissions, dirPath, perm)
		}
	}

	if perm&0o020 != 0 && (perm&0o002 == 0 || !isStickyDirectory(info)) {
		if err := d.validateGroupWritePermissions(dirPath, info, realUID); err != nil {
			return err
		}
	}

	if perm&0o200 != 0 && stat.Uid != UIDRoot {
		if int(stat.Uid) != realUID {
			return fmt.Errorf("%w: directory %s is owned by UID %d but execution user is UID %d", ErrInvalidDirPermissions, dirPath, stat.Uid, realUID)
		}
	}

	return nil
}

func (d *dirPermChecker) validateGroupWritePermissions(dirPath string, info os.FileInfo, realUID int) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: failed to get system info for directory %s", ErrInsecurePathComponent, dirPath)
	}

	perm := info.Mode().Perm()
	if stat.Uid == UIDRoot && isTrustedGroup(stat.Gid) {
		return nil
	}

	if d.gm == nil {
		return fmt.Errorf("%w: directory %s has group write permissions (%04o) but group membership cannot be verified", ErrInvalidDirPermissions, dirPath, perm)
	}

	canSafelyWrite, err := d.gm.CanUserSafelyWriteFile(realUID, stat.Uid, stat.Gid, info.Mode())
	if err != nil {
		return fmt.Errorf("%w: directory %s failed security validation: %v", ErrInvalidDirPermissions, dirPath, err)
	}
	if !canSafelyWrite {
		return fmt.Errorf("%w: directory %s - user UID %d cannot safely write to this directory", ErrInvalidDirPermissions, dirPath, realUID)
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
