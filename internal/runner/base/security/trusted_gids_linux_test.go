//go:build linux

package security

import (
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testutil"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_isTrustedGroup_LinuxDefaultAndConfigUnion(t *testing.T) {
	config := DefaultConfig()
	config.TrustedGIDs = []uint32{10}

	validator, err := NewValidator(config)
	require.NoError(t, err)

	assert.True(t, validator.isTrustedGroup(0))
	assert.True(t, validator.isTrustedGroup(10))
	assert.False(t, validator.isTrustedGroup(11))
}

func TestValidator_validateGroupWritePermissions_LinuxTrustedGIDConfigDifference(t *testing.T) {
	newValidatorWithDir := func(t *testing.T, config *Config, gid uint32) *Validator {
		t.Helper()

		mockFS := commontesting.NewMockFileSystem()
		err := mockFS.AddDirWithOwner("/test", 0o775, UIDRoot, gid)
		require.NoError(t, err)

		validator, err := NewValidator(config, WithFileSystem(mockFS), WithGroupMembership(nil))
		require.NoError(t, err)
		return validator
	}

	t.Run("with_configured_trusted_gid", func(t *testing.T) {
		config := DefaultConfig()
		config.TrustedGIDs = []uint32{10}
		validator := newValidatorWithDir(t, config, 10)

		err := validator.ValidateDirectoryPermissions("/test")
		assert.NoError(t, err)
	})

	t.Run("without_configured_trusted_gid", func(t *testing.T) {
		config := DefaultConfig()
		validator := newValidatorWithDir(t, config, 10)

		err := validator.ValidateDirectoryPermissions("/test")
		assert.ErrorIs(t, err, isec.ErrInvalidDirPermissions)
	})
}
