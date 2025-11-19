package config

import (
	"errors"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestValidateGroupNames tests group name validation during config loading
func TestValidateGroupNames(t *testing.T) {
	t.Run("valid group names", func(t *testing.T) {
		cfg := &runnertypes.ConfigSpec{
			Groups: []runnertypes.GroupSpec{
				{Name: "build"},
				{Name: "test"},
				{Name: "Deploy_123"},
			},
		}
		require.NoError(t, ValidateGroupNames(cfg))
	})

	t.Run("empty group name", func(t *testing.T) {
		cfg := &runnertypes.ConfigSpec{
			Groups: []runnertypes.GroupSpec{
				{Name: "build"},
				{Name: ""},
			},
		}
		err := ValidateGroupNames(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "empty name")
	})

	t.Run("invalid group name with hyphen", func(t *testing.T) {
		cfg := &runnertypes.ConfigSpec{
			Groups: []runnertypes.GroupSpec{
				{Name: "build"},
				{Name: "test-deploy"},
			},
		}
		err := ValidateGroupNames(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid group name")
		require.Contains(t, err.Error(), "test-deploy")
	})

	t.Run("invalid group name starting with number", func(t *testing.T) {
		cfg := &runnertypes.ConfigSpec{
			Groups: []runnertypes.GroupSpec{
				{Name: "123build"},
			},
		}
		err := ValidateGroupNames(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid group name")
	})

	t.Run("duplicate group names", func(t *testing.T) {
		cfg := &runnertypes.ConfigSpec{
			Groups: []runnertypes.GroupSpec{
				{Name: "build"},
				{Name: "test"},
				{Name: "build"},
			},
		}
		err := ValidateGroupNames(cfg)
		require.Error(t, err)
		require.Contains(t, err.Error(), "duplicate group name")
		require.Contains(t, err.Error(), "build")
	})

	t.Run("nil config", func(t *testing.T) {
		err := ValidateGroupNames(nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "must not be nil")
	})

	t.Run("empty groups slice", func(t *testing.T) {
		cfg := &runnertypes.ConfigSpec{
			Groups: []runnertypes.GroupSpec{},
		}
		require.NoError(t, ValidateGroupNames(cfg))
	})
}
