//go:build test

package security

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_SanitizeEnvironmentVariables(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("nil input", func(t *testing.T) {
		result := validator.SanitizeEnvironmentVariables(nil)
		assert.NotNil(t, result)
		assert.Equal(t, make(map[string]string), result)
	})

	t.Run("no sensitive variables", func(t *testing.T) {
		env := map[string]string{
			"PATH":     "/usr/bin:/bin",
			"HOME":     "/home/user",
			"LANGUAGE": "en_US.UTF-8",
		}
		result := validator.SanitizeEnvironmentVariables(env)
		assert.Equal(t, env, result)
	})

	t.Run("with sensitive variables", func(t *testing.T) {
		env := map[string]string{
			"PATH":         "/usr/bin:/bin",
			"HOME":         "/home/user",
			"API_PASSWORD": "secret123",
			"DB_TOKEN":     "token456",
			"NORMAL_VAR":   "value",
		}
		result := validator.SanitizeEnvironmentVariables(env)

		assert.NotEqual(t, env, result)
		assert.Equal(t, "/usr/bin:/bin", result["PATH"])
		assert.Equal(t, "/home/user", result["HOME"])
		assert.Equal(t, "value", result["NORMAL_VAR"])
		assert.Equal(t, "[REDACTED]", result["API_PASSWORD"])
		assert.Equal(t, "[REDACTED]", result["DB_TOKEN"])
	})
}

func TestValidator_ValidateEnvironmentValue(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("safe values", func(t *testing.T) {
		safeValues := []string{
			"simple_value",
			"/path/to/file",
			"user@example.com",
			"123456",
			"normal-value_with_underscores",
		}

		for _, value := range safeValues {
			err := validator.ValidateEnvironmentValue("TEST_VAR", value)
			assert.NoError(t, err, "Value %s should be safe", value)
		}
	})

	t.Run("unsafe values", func(t *testing.T) {
		unsafeValues := []string{
			"value; rm -rf /",
			"value | cat /etc/passwd",
			"value && malicious_command",
			"value || backup_command",
			"value $(malicious_command)",
			"value `malicious_command`",
			"value > /tmp/output",
			"value < /etc/passwd",
		}

		for _, value := range unsafeValues {
			err := validator.ValidateEnvironmentValue("TEST_VAR", value)
			assert.Error(t, err, "Value %s should be unsafe", value)
			assert.ErrorIs(t, err, ErrUnsafeEnvironmentVar)
		}
	})
}

func TestValidator_ValidateAllEnvironmentVars(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("all safe", func(t *testing.T) {
		env := map[string]string{
			"PATH": "/usr/bin:/bin",
			"HOME": "/home/user",
			"USER": "testuser",
		}
		err := validator.ValidateAllEnvironmentVars(env)
		assert.NoError(t, err)
	})

	t.Run("contains unsafe", func(t *testing.T) {
		env := map[string]string{
			"PATH":      "/usr/bin:/bin",
			"HOME":      "/home/user",
			"DANGEROUS": "value; rm -rf /",
		}
		err := validator.ValidateAllEnvironmentVars(env)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrUnsafeEnvironmentVar)
	})

	t.Run("empty map", func(t *testing.T) {
		env := map[string]string{}
		err := validator.ValidateAllEnvironmentVars(env)
		assert.NoError(t, err)
	})

	t.Run("nil map", func(t *testing.T) {
		err := validator.ValidateAllEnvironmentVars(nil)
		assert.NoError(t, err)
	})
}

func TestValidator_isSensitiveEnvVar(t *testing.T) {
	validator, err := NewValidator(nil)
	require.NoError(t, err)

	t.Run("sensitive patterns", func(t *testing.T) {
		sensitiveVars := []string{
			"PASSWORD",
			"API_PASSWORD",
			"DB_PASSWORD",
			"SECRET",
			"API_SECRET",
			"TOKEN",
			"ACCESS_TOKEN",
			"KEY",
			"API_KEY",
			"PRIVATE_KEY",
		}

		for _, varName := range sensitiveVars {
			assert.True(t, validator.isSensitiveEnvVar(varName), "Variable %s should be sensitive", varName)
		}
	})

	t.Run("non-sensitive patterns", func(t *testing.T) {
		nonSensitiveVars := []string{
			"PATH",
			"HOME",
			"USER",
			"LANG",
			"TMPDIR",
			"PWD",
			"SHELL",
		}

		for _, varName := range nonSensitiveVars {
			assert.False(t, validator.isSensitiveEnvVar(varName), "Variable %s should not be sensitive", varName)
		}
	})
}
