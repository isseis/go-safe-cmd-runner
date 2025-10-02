//go:build test

package security

import (
	"os"
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/stretchr/testify/assert"
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

func TestNewValidatorWithFS(t *testing.T) {
	mockFS := commontesting.NewMockFileSystem()
	config := DefaultConfig()
	// Override for this specific test
	config.AllowedCommands = []string{"^echo$"}
	config.SensitiveEnvVars = []string{".*PASSWORD.*"}
	validator, err := NewValidatorWithFS(config, mockFS)

	assert.NoError(t, err)
	assert.NotNil(t, validator)
	assert.Equal(t, config, validator.config)
	assert.Equal(t, mockFS, validator.fs)
}
