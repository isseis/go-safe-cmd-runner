//go:build !windows

package security

import (
	"os"
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
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

func isTrustedGroup(gid uint32) bool {
	if gid == GIDRoot {
		return true
	}

	if runtime.GOOS == "darwin" && gid == darwinAdminGID {
		return true
	}

	return false
}
