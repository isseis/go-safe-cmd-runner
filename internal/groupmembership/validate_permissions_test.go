package groupmembership

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/stretchr/testify/assert"
)

func TestValidateRequestedPermissions(t *testing.T) {
	gm := New()

	t.Run("read_operation_valid_permissions", func(t *testing.T) {
		tests := []struct {
			name string
			perm os.FileMode
		}{
			{"normal_read_644", 0o644},
			{"read_only_444", 0o444},
			{"group_read_664", 0o664},
			{"owner_read_600", 0o600},
			{"execute_755", 0o755},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := gm.ValidateRequestedPermissions(tt.perm, FileOpRead)
				assert.NoError(t, err, "Valid read permissions should not return error")
			})
		}
	})

	t.Run("write_operation_valid_permissions", func(t *testing.T) {
		tests := []struct {
			name string
			perm os.FileMode
		}{
			{"normal_write_644", 0o644},
			{"owner_write_600", 0o600},
			{"group_write_664", 0o664},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := gm.ValidateRequestedPermissions(tt.perm, FileOpWrite)
				assert.NoError(t, err, "Valid write permissions should not return error")
			})
		}
	})

	t.Run("write_operation_invalid_permissions", func(t *testing.T) {
		tests := []struct {
			name string
			perm os.FileMode
		}{
			{"world_writable_666", 0o666},
			{"world_writable_777", 0o777},
			{"other_write_only_002", 0o002},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := gm.ValidateRequestedPermissions(tt.perm, FileOpWrite)
				assert.Error(t, err, "Invalid write permissions should return error")
				assert.ErrorIs(t, err, ErrPermissionsExceedMaximum)
			})
		}
	})

	t.Run("setuid_setgid_permissions", func(t *testing.T) {
		tests := []struct {
			name      string
			perm      os.FileMode
			operation FileOperation
			wantErr   bool
		}{
			{"setuid_read_allowed", 0o4755, FileOpRead, false},
			{"setgid_read_allowed", 0o2755, FileOpRead, false},
			{"setuid_write_denied", 0o4755, FileOpWrite, true},
			{"setgid_write_denied", 0o2755, FileOpWrite, true},
			{"sticky_read_denied", 0o1755, FileOpRead, true}, // sticky bit exceeds MaxAllowedReadPerms (0o6775)
			{"sticky_write_denied", 0o1755, FileOpWrite, true},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := gm.ValidateRequestedPermissions(tt.perm, tt.operation)
				if tt.wantErr {
					assert.Error(t, err)
					assert.ErrorIs(t, err, ErrPermissionsExceedMaximum)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("unknown_operation", func(t *testing.T) {
		err := gm.ValidateRequestedPermissions(0o644, FileOperation(999))
		assert.Error(t, err)
		assert.ErrorIs(t, err, common.ErrInvalidFileOperation)
	})

	t.Run("boundary_values", func(t *testing.T) {
		tests := []struct {
			name      string
			perm      os.FileMode
			operation FileOperation
			wantErr   bool
		}{
			{"max_allowed_read", MaxAllowedReadPerms, FileOpRead, false},
			{"max_allowed_write", MaxAllowedWritePerms, FileOpWrite, false},
			{"over_max_read_sticky", MaxAllowedReadPerms | 0o1000, FileOpRead, true}, // sticky bit exceeds max
			{"over_max_write", MaxAllowedWritePerms | 0o002, FileOpWrite, true},
			{"zero_permissions_read", 0o000, FileOpRead, false},
			{"zero_permissions_write", 0o000, FileOpWrite, false},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := gm.ValidateRequestedPermissions(tt.perm, tt.operation)
				if tt.wantErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
			})
		}
	})

	t.Run("all_permission_bits", func(t *testing.T) {
		// Test that all permission bits are properly checked
		fullPerms := os.FileMode(0o7777) // setuid + setgid + sticky + all perms

		err := gm.ValidateRequestedPermissions(fullPerms, FileOpRead)
		// Should deny for read (sticky bit exceeds MaxAllowedReadPerms)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrPermissionsExceedMaximum)

		err = gm.ValidateRequestedPermissions(fullPerms, FileOpWrite)
		// Should deny for write (special bits not allowed for write)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrPermissionsExceedMaximum)
	})
}
