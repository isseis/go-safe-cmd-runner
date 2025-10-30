package security

import (
	"os"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	assert.NotNil(t, config)
	assert.NotEmpty(t, config.AllowedCommands)
	assert.Equal(t, os.FileMode(0o644), config.RequiredFilePermissions)
	assert.Equal(t, os.FileMode(0o755), config.RequiredDirectoryPermissions)
	assert.NotEmpty(t, config.SensitiveEnvVars)
	assert.Equal(t, 4096, config.MaxPathLength)
}

func TestNewValidator(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		config := DefaultConfig()
		// Override for this specific test
		config.AllowedCommands = []string{"^echo$"}
		config.SensitiveEnvVars = []string{".*PASSWORD.*"}
		validator, err := NewValidator(config)

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.Equal(t, config, validator.config)
		assert.Len(t, validator.allowedCommandRegexps, 1)
		assert.Len(t, validator.sensitiveEnvRegexps, 1)
		assert.GreaterOrEqual(t, len(validator.dangerousEnvRegexps), 1)
	})

	t.Run("with nil config", func(t *testing.T) {
		validator, err := NewValidator(nil)

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.NotNil(t, validator.config)
		assert.NotEmpty(t, validator.allowedCommandRegexps)
		assert.NotEmpty(t, validator.sensitiveEnvRegexps)
		assert.NotEmpty(t, validator.dangerousEnvRegexps)
	})

	t.Run("with invalid command pattern", func(t *testing.T) {
		config := DefaultConfig()
		// Set invalid pattern to test error handling
		config.AllowedCommands = []string{"[invalid"}
		config.SensitiveEnvVars = []string{}
		validator, err := NewValidator(config)

		assert.Error(t, err)
		assert.Nil(t, validator)
		assert.ErrorIs(t, err, ErrInvalidRegexPattern)
	})

	t.Run("with invalid sensitive env pattern", func(t *testing.T) {
		config := DefaultConfig()
		// Set invalid pattern to test error handling
		config.AllowedCommands = []string{".*"}
		config.SensitiveEnvVars = []string{"[invalid"}
		validator, err := NewValidator(config)

		assert.Error(t, err)
		assert.Nil(t, validator)
		assert.ErrorIs(t, err, ErrInvalidRegexPattern)
	})
}

func TestValidator_CustomConfig(t *testing.T) {
	config := DefaultConfig()
	// Override with custom values for testing
	config.DangerousPrivilegedCommands = []string{"/custom/dangerous"}
	config.ShellCommands = []string{"/custom/shell"}
	config.ShellMetacharacters = []string{"@", "#"}

	validator, err := NewValidator(config)
	require.NoError(t, err)

	// Test custom dangerous command
	assert.True(t, validator.IsDangerousPrivilegedCommand("/custom/dangerous"))
	assert.False(t, validator.IsDangerousPrivilegedCommand("/bin/bash")) // Not in custom list

	// Test custom shell command
	assert.True(t, validator.IsShellCommand("/custom/shell"))
	assert.False(t, validator.IsShellCommand("/bin/bash")) // Not in custom list

	// Test custom metacharacters
	assert.True(t, validator.HasShellMetacharacters([]string{"test@example"}))
	assert.True(t, validator.HasShellMetacharacters([]string{"test#hash"}))
	assert.False(t, validator.HasShellMetacharacters([]string{"test;semicolon"})) // Not in custom list
}

func TestNewValidator_WithOptions(t *testing.T) {
	t.Run("with no options", func(t *testing.T) {
		validator, err := NewValidator(nil)

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.NotNil(t, validator.fs)
		assert.Nil(t, validator.groupMembership)
	})

	t.Run("with WithFileSystem option", func(t *testing.T) {
		mockFS := commontesting.NewMockFileSystem()
		config := DefaultConfig()
		validator, err := NewValidator(config, WithFileSystem(mockFS))

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.Equal(t, mockFS, validator.fs)
		assert.Nil(t, validator.groupMembership)
	})

	t.Run("with WithGroupMembership option", func(t *testing.T) {
		config := DefaultConfig()
		gm := groupmembership.New()
		validator, err := NewValidator(config, WithGroupMembership(gm))

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.NotNil(t, validator.fs)
		assert.Equal(t, gm, validator.groupMembership)
	})

	t.Run("with both options", func(t *testing.T) {
		mockFS := commontesting.NewMockFileSystem()
		gm := groupmembership.New()
		config := DefaultConfig()
		validator, err := NewValidator(config, WithFileSystem(mockFS), WithGroupMembership(gm))

		assert.NoError(t, err)
		assert.NotNil(t, validator)
		assert.Equal(t, mockFS, validator.fs)
		assert.Equal(t, gm, validator.groupMembership)
	})

	t.Run("option application order independence", func(t *testing.T) {
		mockFS := commontesting.NewMockFileSystem()
		gm := groupmembership.New()
		config := DefaultConfig()

		// Test with options in different orders
		validator1, err1 := NewValidator(config, WithFileSystem(mockFS), WithGroupMembership(gm))
		validator2, err2 := NewValidator(config, WithGroupMembership(gm), WithFileSystem(mockFS))

		assert.NoError(t, err1)
		assert.NoError(t, err2)
		assert.Equal(t, validator1.fs, validator2.fs)
		assert.Equal(t, validator1.groupMembership, validator2.groupMembership)
	})
}
