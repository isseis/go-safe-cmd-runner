//go:build !darwin && !linux

package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_isTrustedGroup_OtherDefaultAndConfigUnion(t *testing.T) {
	config := DefaultConfig()
	config.TrustedGIDs = []uint32{1234}

	validator, err := NewValidator(config)
	require.NoError(t, err)

	assert.True(t, validator.isTrustedGroup(0))
	assert.True(t, validator.isTrustedGroup(1234))
	assert.False(t, validator.isTrustedGroup(4321))
}
