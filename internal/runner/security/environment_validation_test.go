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

func TestValidateVariableName(t *testing.T) {
	t.Run("valid variable names", func(t *testing.T) {
		validNames := []string{
			"PATH",
			"HOME",
			"USER",
			"_",
			"_VAR",
			"VAR_",
			"VAR123",
			"a",
			"A",
			"_123",
			"MY_VAR_123",
			"lowercase_var",
			"UPPERCASE_VAR",
			"Mixed_Case_Var",
			"var1",
			"var_1_2_3",
		}

		for _, name := range validNames {
			err := ValidateVariableName(name)
			assert.NoError(t, err, "Variable name %s should be valid", name)
		}
	})

	t.Run("empty variable name", func(t *testing.T) {
		err := ValidateVariableName("")
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrVariableNameEmpty)
	})

	t.Run("invalid start character", func(t *testing.T) {
		invalidStartNames := []string{
			"1VAR",
			"2test",
			"9abc",
			"-var",
			"+var",
			"=var",
			"@var",
			"#var",
			"$var",
			"%var",
			"&var",
			"*var",
			"(var",
			")var",
			"{var",
			"}var",
			"[var",
			"]var",
			"|var",
			"\\var",
			"/var",
			"?var",
			".var",
			",var",
			"<var",
			">var",
			";var",
			":var",
			"'var",
			"\"var",
			"`var",
			"~var",
			"!var",
		}

		for _, name := range invalidStartNames {
			err := ValidateVariableName(name)
			assert.Error(t, err, "Variable name %s should be invalid (bad start)", name)
			assert.ErrorIs(t, err, ErrVariableNameInvalidStart)
		}
	})

	t.Run("invalid characters in name", func(t *testing.T) {
		invalidCharNames := []string{
			"VAR-NAME",
			"VAR+NAME",
			"VAR=NAME",
			"VAR@NAME",
			"VAR#NAME",
			"VAR$NAME",
			"VAR%NAME",
			"VAR&NAME",
			"VAR*NAME",
			"VAR(NAME",
			"VAR)NAME",
			"VAR{NAME",
			"VAR}NAME",
			"VAR[NAME",
			"VAR]NAME",
			"VAR|NAME",
			"VAR\\NAME",
			"VAR/NAME",
			"VAR?NAME",
			"VAR.NAME",
			"VAR,NAME",
			"VAR<NAME",
			"VAR>NAME",
			"VAR;NAME",
			"VAR:NAME",
			"VAR'NAME",
			"VAR\"NAME",
			"VAR`NAME",
			"VAR~NAME",
			"VAR!NAME",
			"VAR NAME",  // space
			"VAR\tNAME", // tab
			"VAR\nNAME", // newline
		}

		for _, name := range invalidCharNames {
			err := ValidateVariableName(name)
			assert.Error(t, err, "Variable name %s should be invalid (bad char)", name)
			assert.ErrorIs(t, err, ErrVariableNameInvalidChar)
		}
	})

	t.Run("edge cases", func(t *testing.T) {
		// Single character valid names
		singleCharValid := []string{"a", "A", "z", "Z", "_"}
		for _, name := range singleCharValid {
			err := ValidateVariableName(name)
			assert.NoError(t, err, "Single character %s should be valid", name)
		}

		// Very long but valid name
		longName := "VERY_LONG_VARIABLE_NAME_WITH_MANY_CHARACTERS_123_456_789"
		err := ValidateVariableName(longName)
		assert.NoError(t, err, "Long variable name should be valid")

		// Name with all valid character types
		mixedName := "aB_123_cD_456"
		err = ValidateVariableName(mixedName)
		assert.NoError(t, err, "Mixed character name should be valid")
	})
}

func TestIsVariableValueSafe(t *testing.T) {
	t.Run("safe values", func(t *testing.T) {
		safeValues := []struct {
			name  string
			value string
		}{
			{"PATH", "/usr/bin:/bin"},
			{"HOME", "/home/user"},
			{"USER", "testuser"},
			{"EMAIL", "user@example.com"},
			{"NUMBER", "123456"},
			{"TEXT", "simple_text_value"},
			{"MIXED", "value-with_various.characters"},
		}

		for _, test := range safeValues {
			err := IsVariableValueSafe(test.name, test.value)
			assert.NoError(t, err, "Value %s for variable %s should be safe", test.value, test.name)
		}
	})

	t.Run("unsafe values", func(t *testing.T) {
		unsafeValues := []struct {
			name  string
			value string
		}{
			{"DANGEROUS", "value; rm -rf /"},
			{"INJECTION", "value | cat /etc/passwd"},
			{"COMMAND", "value && malicious_command"},
			{"FALLBACK", "value || backup_command"},
			{"SUBSHELL", "value $(malicious_command)"},
			{"BACKTICK", "value `malicious_command`"},
			{"REDIRECT_OUT", "value > /tmp/output"},
			{"REDIRECT_IN", "value < /etc/passwd"},
		}

		for _, test := range unsafeValues {
			err := IsVariableValueSafe(test.name, test.value)
			assert.Error(t, err, "Value %s for variable %s should be unsafe", test.value, test.name)
			assert.ErrorIs(t, err, ErrUnsafeEnvironmentVar)
		}
	})

	t.Run("empty values", func(t *testing.T) {
		err := IsVariableValueSafe("TEST_VAR", "")
		assert.NoError(t, err, "Empty value should be safe")
	})
}
