package variable

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetermineScope(t *testing.T) {
	tests := []struct {
		name          string
		variableName  string
		expectedScope VariableScope
		expectError   bool
		errorType     string
	}{
		// Global variable success cases (uppercase start)
		{
			name:          "global_uppercase_single",
			variableName:  "A",
			expectedScope: ScopeGlobal,
			expectError:   false,
		},
		{
			name:          "global_uppercase_word",
			variableName:  "AwsPath",
			expectedScope: ScopeGlobal,
			expectError:   false,
		},
		{
			name:          "global_uppercase_snake",
			variableName:  "AWS_PATH",
			expectedScope: ScopeGlobal,
			expectError:   false,
		},
		{
			name:          "global_uppercase_with_digits",
			variableName:  "Path123",
			expectedScope: ScopeGlobal,
			expectError:   false,
		},

		// Local variable success cases (lowercase start)
		{
			name:          "local_lowercase_single",
			variableName:  "a",
			expectedScope: ScopeLocal,
			expectError:   false,
		},
		{
			name:          "local_lowercase_word",
			variableName:  "dataDir",
			expectedScope: ScopeLocal,
			expectError:   false,
		},
		{
			name:          "local_lowercase_snake",
			variableName:  "data_dir",
			expectedScope: ScopeLocal,
			expectError:   false,
		},
		{
			name:          "local_lowercase_with_digits",
			variableName:  "path123",
			expectedScope: ScopeLocal,
			expectError:   false,
		},

		// Local variable success cases (underscore start)
		{
			name:          "local_underscore_single",
			variableName:  "_",
			expectedScope: ScopeLocal,
			expectError:   false,
		},
		{
			name:          "local_underscore_word",
			variableName:  "_internal",
			expectedScope: ScopeLocal,
			expectError:   false,
		},
		{
			name:          "local_underscore_mixed",
			variableName:  "_Private_Path",
			expectedScope: ScopeLocal,
			expectError:   false,
		},

		// Reserved variable error cases (__ prefix)
		{
			name:          "reserved_double_underscore",
			variableName:  "__reserved",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrReservedVariableName",
		},
		{
			name:          "reserved_double_underscore_exact",
			variableName:  "__",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrReservedVariableName",
		},
		{
			name:          "reserved_double_underscore_uppercase",
			variableName:  "__RESERVED",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrReservedVariableName",
		},

		// Invalid variable name error cases (digit start)
		{
			name:          "invalid_digit_start",
			variableName:  "123invalid",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrInvalidVariableName",
		},
		{
			name:          "invalid_digit_only",
			variableName:  "0",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrInvalidVariableName",
		},

		// Invalid variable name error cases (special characters)
		{
			name:          "invalid_special_char_start",
			variableName:  "$PATH",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrInvalidVariableName",
		},
		{
			name:          "invalid_hyphen_start",
			variableName:  "-path",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrInvalidVariableName",
		},
		{
			name:          "invalid_dot_start",
			variableName:  ".path",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrInvalidVariableName",
		},

		// Empty string error case
		{
			name:          "empty_string",
			variableName:  "",
			expectedScope: ScopeError,
			expectError:   true,
			errorType:     "ErrInvalidVariableName",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scope, err := DetermineScope(tt.variableName)

			if tt.expectError {
				require.Error(t, err, "expected error for variable name: %q", tt.variableName)
				assert.Equal(t, ScopeError, scope, "scope should be ScopeError on error")

				// Check error type
				switch tt.errorType {
				case "ErrReservedVariableName":
					assert.IsType(t, &ErrReservedVariableName{}, err)
				case "ErrInvalidVariableName":
					assert.IsType(t, &ErrInvalidVariableName{}, err)
				}
			} else {
				require.NoError(t, err, "unexpected error for variable name: %q", tt.variableName)
				assert.Equal(t, tt.expectedScope, scope, "scope mismatch for variable name: %q", tt.variableName)
			}
		})
	}
}

func TestValidateVariableNameForScope(t *testing.T) {
	tests := []struct {
		name          string
		variableName  string
		expectedScope VariableScope
		location      string
		expectError   bool
		errorContains string
	}{
		// Global scope valid names
		{
			name:          "global_valid_uppercase",
			variableName:  "AwsPath",
			expectedScope: ScopeGlobal,
			location:      "[global.vars]",
			expectError:   false,
		},
		{
			name:          "global_valid_uppercase_snake",
			variableName:  "AWS_PATH",
			expectedScope: ScopeGlobal,
			location:      "[global.vars]",
			expectError:   false,
		},

		// Local scope valid names
		{
			name:          "local_valid_lowercase",
			variableName:  "dataDir",
			expectedScope: ScopeLocal,
			location:      "[groups.vars]",
			expectError:   false,
		},
		{
			name:          "local_valid_lowercase_snake",
			variableName:  "data_dir",
			expectedScope: ScopeLocal,
			location:      "[groups.vars]",
			expectError:   false,
		},
		{
			name:          "local_valid_underscore",
			variableName:  "_internal",
			expectedScope: ScopeLocal,
			location:      "[groups.vars]",
			expectError:   false,
		},

		// Scope mismatch errors (global with lowercase)
		{
			name:          "global_mismatch_lowercase",
			variableName:  "awsPath",
			expectedScope: ScopeGlobal,
			location:      "[global.vars]",
			expectError:   true,
			errorContains: "must be global",
		},
		{
			name:          "global_mismatch_underscore",
			variableName:  "_internal",
			expectedScope: ScopeGlobal,
			location:      "[global.vars]",
			expectError:   true,
			errorContains: "must be global",
		},

		// Scope mismatch errors (local with uppercase)
		{
			name:          "local_mismatch_uppercase",
			variableName:  "AwsPath",
			expectedScope: ScopeLocal,
			location:      "[groups.vars]",
			expectError:   true,
			errorContains: "must be local",
		},
		{
			name:          "local_mismatch_uppercase_snake",
			variableName:  "AWS_PATH",
			expectedScope: ScopeLocal,
			location:      "[groups.commands.vars]",
			expectError:   true,
			errorContains: "must be local",
		},

		// Reserved variable errors
		{
			name:          "reserved_global_scope",
			variableName:  "__reserved",
			expectedScope: ScopeGlobal,
			location:      "[global.vars]",
			expectError:   true,
			errorContains: "is reserved",
		},
		{
			name:          "reserved_local_scope",
			variableName:  "__reserved",
			expectedScope: ScopeLocal,
			location:      "[groups.vars]",
			expectError:   true,
			errorContains: "is reserved",
		},

		// Invalid characters (caught by security.ValidateVariableName)
		{
			name:          "invalid_char_hyphen",
			variableName:  "data-dir",
			expectedScope: ScopeLocal,
			location:      "[groups.vars]",
			expectError:   true,
			errorContains: "invalid",
		},
		{
			name:          "invalid_char_dot",
			variableName:  "data.dir",
			expectedScope: ScopeLocal,
			location:      "[groups.vars]",
			expectError:   true,
			errorContains: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVariableNameForScope(tt.variableName, tt.expectedScope, tt.location)

			if tt.expectError {
				require.Error(t, err, "expected error for variable name: %q in scope: %s", tt.variableName, tt.expectedScope)
				assert.Contains(t, err.Error(), tt.errorContains, "error message should contain expected text")
				assert.Contains(t, err.Error(), tt.location, "error message should contain location")
			} else {
				require.NoError(t, err, "unexpected error for variable name: %q in scope: %s", tt.variableName, tt.expectedScope)
			}
		})
	}
}

func TestErrorMessages(t *testing.T) {
	t.Run("ErrReservedVariableName", func(t *testing.T) {
		err := &ErrReservedVariableName{Name: "__reserved"}
		msg := err.Error()
		assert.Contains(t, msg, "__reserved")
		assert.Contains(t, msg, "reserved")
		assert.Contains(t, msg, "__")
	})

	t.Run("ErrInvalidVariableName", func(t *testing.T) {
		err := &ErrInvalidVariableName{
			Name:   "123invalid",
			Reason: "must start with letter",
		}
		msg := err.Error()
		assert.Contains(t, msg, "123invalid")
		assert.Contains(t, msg, "must start with letter")
	})

	t.Run("ErrScopeMismatch", func(t *testing.T) {
		err := &ErrScopeMismatch{
			Name:          "awsPath",
			Location:      "[global.vars]",
			ExpectedScope: ScopeGlobal,
			ActualScope:   ScopeLocal,
		}
		msg := err.Error()
		assert.Contains(t, msg, "awsPath")
		assert.Contains(t, msg, "[global.vars]")
		assert.Contains(t, msg, "global")
		assert.Contains(t, msg, "local")
		assert.Contains(t, msg, "uppercase")
		assert.Contains(t, msg, "lowercase")
	})

	t.Run("ErrUndefinedGlobalVariable", func(t *testing.T) {
		err := &ErrUndefinedGlobalVariable{Name: "AwsPath"}
		msg := err.Error()
		assert.Contains(t, msg, "AwsPath")
		assert.Contains(t, msg, "undefined")
		assert.Contains(t, msg, "global")
	})

	t.Run("ErrUndefinedLocalVariable", func(t *testing.T) {
		err := &ErrUndefinedLocalVariable{Name: "dataDir"}
		msg := err.Error()
		assert.Contains(t, msg, "dataDir")
		assert.Contains(t, msg, "undefined")
		assert.Contains(t, msg, "local")
	})
}

func TestVariableScopeString(t *testing.T) {
	tests := []struct {
		scope    VariableScope
		expected string
	}{
		{ScopeGlobal, "global"},
		{ScopeLocal, "local"},
		{ScopeError, "error"},
		{VariableScope(999), "unknown"}, // invalid scope
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.scope.String())
		})
	}
}
