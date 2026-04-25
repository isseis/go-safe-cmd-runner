//go:build darwin

package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_isTrustedGroup_DarwinDefaults(t *testing.T) {
	config := DefaultConfig()
	config.TrustedGIDs = []uint32{999}

	validator, err := NewValidator(config)
	require.NoError(t, err)

	assert.True(t, validator.isTrustedGroup(0))
	assert.True(t, validator.isTrustedGroup(80))
}

func TestValidator_isTrustedGroup_DarwinIgnoresConfiguredGIDs(t *testing.T) {
	config := DefaultConfig()
	config.TrustedGIDs = []uint32{999}

	validator, err := NewValidator(config)
	require.NoError(t, err)

	assert.False(t, validator.isTrustedGroup(999))
}
