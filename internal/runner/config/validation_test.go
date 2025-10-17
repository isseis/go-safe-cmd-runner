package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidateEnvList tests environment variable list validation
func TestValidateEnvList(t *testing.T) {
	tests := []struct {
		name    string
		envList []string
		context string
		wantErr bool
		errType error
	}{
		{
			name:    "no duplicates",
			envList: []string{"VAR1=value1", "VAR2=value2", "VAR3=value3"},
			context: "global.env",
			wantErr: false,
		},
		{
			name:    "duplicate keys",
			envList: []string{"VAR1=value1", "VAR2=value2", "VAR1=value3"},
			context: "global.env",
			wantErr: true,
			errType: ErrDuplicateEnvVariable,
		},
		{
			name:    "different case keys are allowed (case-sensitive)",
			envList: []string{"PATH=/usr/bin", "path=/usr/local/bin"},
			context: "group.env",
			wantErr: false,
		},
		{
			name:    "empty list",
			envList: []string{},
			context: "global.env",
			wantErr: false,
		},
		{
			name:    "nil list",
			envList: nil,
			context: "global.env",
			wantErr: false,
		},
		{
			name:    "invalid format without equals",
			envList: []string{"VAR1=value1", "INVALID_NO_EQUALS"},
			context: "global.env",
			wantErr: true,
			errType: ErrMalformedEnvVariable,
		},
		{
			name:    "empty key",
			envList: []string{"=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrMalformedEnvVariable,
		},
		{
			name:    "invalid key starting with number",
			envList: []string{"123VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrInvalidEnvKey,
		},
		{
			name:    "invalid key with hyphen",
			envList: []string{"MY-VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrInvalidEnvKey,
		},
		{
			name:    "invalid key with dot",
			envList: []string{"MY.VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrInvalidEnvKey,
		},
		{
			name:    "invalid key with space",
			envList: []string{"MY VAR=value"},
			context: "global.env",
			wantErr: true,
			errType: ErrInvalidEnvKey,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateEnvList(tt.envList, tt.context)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.True(t, errors.Is(err, tt.errType))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestValidateAndParseEnvList tests environment variable list validation and parsing
func TestValidateAndParseEnvList(t *testing.T) {
	tests := []struct {
		name    string
		envList []string
		context string
		wantErr bool
		wantMap map[string]string
		errType error
	}{
		{
			name:    "valid environment variables",
			envList: []string{"VAR1=value1", "VAR2=value2"},
			context: "global.env",
			wantErr: false,
			wantMap: map[string]string{"VAR1": "value1", "VAR2": "value2"},
		},
		{
			name:    "empty list returns nil map",
			envList: []string{},
			context: "global.env",
			wantErr: false,
			wantMap: nil,
		},
		{
			name:    "nil list returns nil map",
			envList: nil,
			context: "global.env",
			wantErr: false,
			wantMap: nil,
		},
		{
			name:    "duplicate keys return error",
			envList: []string{"VAR1=value1", "VAR1=value2"},
			context: "global.env",
			wantErr: true,
			wantMap: nil,
			errType: ErrDuplicateEnvVariable,
		},
		{
			name:    "invalid format returns error",
			envList: []string{"INVALID_FORMAT"},
			context: "global.env",
			wantErr: true,
			wantMap: nil,
			errType: ErrMalformedEnvVariable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validateAndParseEnvList(tt.envList, tt.context)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.errType != nil {
					assert.True(t, errors.Is(err, tt.errType))
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantMap, result)
			}
		})
	}
}

// TestValidateVariableName tests internal variable name validation with detailed errors
func TestValidateVariableName(t *testing.T) {
	tests := []struct {
		name         string
		variableName string
		level        string
		field        string
		wantErr      bool
		errType      error
	}{
		{
			name:         "valid lowercase name",
			variableName: "home",
			level:        "global",
			field:        "vars",
			wantErr:      false,
		},
		{
			name:         "valid uppercase name",
			variableName: "MY_VAR",
			level:        "global",
			field:        "vars",
			wantErr:      false,
		},
		{
			name:         "valid mixed case name",
			variableName: "user_path",
			level:        "group:mygroup",
			field:        "vars",
			wantErr:      false,
		},
		{
			name:         "valid name starting with underscore",
			variableName: "_private",
			level:        "cmd:mycmd",
			field:        "vars",
			wantErr:      false,
		},
		{
			name:         "valid name with numbers",
			variableName: "var123",
			level:        "global",
			field:        "vars",
			wantErr:      false,
		},
		{
			name:         "invalid name starting with number",
			variableName: "123var",
			level:        "global",
			field:        "vars",
			wantErr:      true,
			errType:      ErrInvalidVariableName,
		},
		{
			name:         "invalid name with hyphen",
			variableName: "my-var",
			level:        "global",
			field:        "vars",
			wantErr:      true,
			errType:      ErrInvalidVariableName,
		},
		{
			name:         "invalid name with dot",
			variableName: "my.var",
			level:        "global",
			field:        "vars",
			wantErr:      true,
			errType:      ErrInvalidVariableName,
		},
		{
			name:         "invalid name with space",
			variableName: "my var",
			level:        "global",
			field:        "vars",
			wantErr:      true,
			errType:      ErrInvalidVariableName,
		},
		{
			name:         "reserved prefix __runner_",
			variableName: "__runner_foo",
			level:        "global",
			field:        "vars",
			wantErr:      true,
			errType:      ErrReservedVariablePrefix,
		},
		{
			name:         "empty string",
			variableName: "",
			level:        "global",
			field:        "vars",
			wantErr:      true,
			errType:      ErrInvalidVariableName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateVariableName(tt.variableName, tt.level, tt.field)
			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.True(t, errors.Is(err, tt.errType))
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
