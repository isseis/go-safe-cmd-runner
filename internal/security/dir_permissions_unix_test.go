package security

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
)

// fakeDirInfo is a minimal os.FileInfo backed by a syscall.Stat_t, used to
// drive ValidateDirectoryPermissionsWithOptions without touching the real
// file system.
type fakeDirInfo struct {
	name string
	mode os.FileMode
	uid  uint32
	gid  uint32
}

func (f fakeDirInfo) Name() string       { return f.name }
func (f fakeDirInfo) Size() int64        { return 0 }
func (f fakeDirInfo) Mode() os.FileMode  { return f.mode }
func (f fakeDirInfo) ModTime() time.Time { return time.Time{} }
func (f fakeDirInfo) IsDir() bool        { return f.mode.IsDir() }
func (f fakeDirInfo) Sys() any           { return &syscall.Stat_t{Uid: f.uid, Gid: f.gid} }

// TestValidateDirectoryPermissionsWithOptions_PropagatesEnumerationSentinel
// verifies that a sentinel returned by CanUserSafelyWrite survives the
// wrapping done by validateGroupWritePermissions, so a caller can still tell
// with errors.Is why the directory was refused.
func TestValidateDirectoryPermissionsWithOptions_PropagatesEnumerationSentinel(t *testing.T) {
	// "/target" is group-writable, owned by a non-root, non-caller UID, so
	// validateGroupWritePermissions delegates to CanUserSafelyWrite instead of
	// taking the root/trusted-group or "owned by caller" shortcuts. The
	// CanUserSafelyWrite failure returns before the hierarchy walk would ever
	// reach "/", so no fixture for it is needed here.
	target := fakeDirInfo{name: "target", mode: os.ModeDir | 0o030, uid: 2000, gid: 2000}

	lstat := func(path string) (os.FileInfo, error) {
		if path == "/target" {
			return target, nil
		}
		return nil, fmt.Errorf("unexpected path %s", path)
	}

	cause := fmt.Errorf("enumeration incomplete: %w", groupmembership.ErrGroupMemberEnumerationIncomplete)
	opts := DirectoryPermCheckOptions{
		Lstat:   lstat,
		RealUID: 1000,
		CanUserSafelyWrite: func(_ int, _ uint32, _ uint32, _ os.FileMode) (bool, error) {
			return false, cause
		},
	}

	err := ValidateDirectoryPermissionsWithOptions("/target", opts)

	assert.ErrorIs(t, err, groupmembership.ErrGroupMemberEnumerationIncomplete,
		"the sentinel from CanUserSafelyWrite must survive the wrapping")
	assert.ErrorIs(t, err, ErrInvalidDirPermissions,
		"the directory-permission sentinel must still be reachable alongside the new one")
	assert.False(t, errors.Is(err, groupmembership.ErrGroupMemberCompletenessUnstated),
		"a different sentinel must not be reported as present")
}
