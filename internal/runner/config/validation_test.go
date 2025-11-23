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
	tests := []struct {
		name          string
		config        *runnertypes.ConfigSpec
		wantErr       bool
		expectedError error
		errorContains []string
	}{
		{
			name: "valid group names - lowercase",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build"},
					{Name: "test"},
					{Name: "deploy"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid group names - uppercase",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "BUILD"},
					{Name: "TEST"},
					{Name: "DEPLOY"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid group names - mixed case with underscore",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "Build_Stage"},
					{Name: "Test_Unit"},
					{Name: "Deploy_Prod"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid group names - starting with underscore",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "_internal"},
					{Name: "_private_build"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid group names - with numbers",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build123"},
					{Name: "test_v2"},
					{Name: "Deploy_123"},
				},
			},
			wantErr: false,
		},
		{
			name: "single valid group",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty groups slice - valid",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{},
			},
			wantErr: false,
		},
		{
			name: "empty group name at index 0",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: ""},
				},
			},
			wantErr:       true,
			expectedError: ErrEmptyGroupName,
			errorContains: []string{"empty name", "index 0"},
		},
		{
			name: "empty group name at index 1",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build"},
					{Name: ""},
				},
			},
			wantErr:       true,
			expectedError: ErrEmptyGroupName,
			errorContains: []string{"empty name", "index 1"},
		},
		{
			name: "invalid group name with hyphen",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build"},
					{Name: "test-deploy"},
				},
			},
			wantErr:       true,
			expectedError: ErrInvalidGroupName,
			errorContains: []string{"invalid group name", "test-deploy", "index 1"},
		},
		{
			name: "invalid group name with dot",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "test.deploy"},
				},
			},
			wantErr:       true,
			expectedError: ErrInvalidGroupName,
			errorContains: []string{"invalid group name", "test.deploy"},
		},
		{
			name: "invalid group name with space",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "test deploy"},
				},
			},
			wantErr:       true,
			expectedError: ErrInvalidGroupName,
			errorContains: []string{"invalid group name", "test deploy"},
		},
		{
			name: "invalid group name starting with number",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "123build"},
				},
			},
			wantErr:       true,
			expectedError: ErrInvalidGroupName,
			errorContains: []string{"invalid group name", "123build"},
		},
		{
			name: "invalid group name with special characters",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build@test"},
				},
			},
			wantErr:       true,
			expectedError: ErrInvalidGroupName,
			errorContains: []string{"invalid group name", "build@test"},
		},
		{
			name: "duplicate group names - simple case",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build"},
					{Name: "test"},
					{Name: "build"},
				},
			},
			wantErr:       true,
			expectedError: ErrDuplicateGroupName,
			errorContains: []string{"duplicate group name", "build", "indices 0 and 2"},
		},
		{
			name: "duplicate group names - adjacent",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "test"},
					{Name: "test"},
				},
			},
			wantErr:       true,
			expectedError: ErrDuplicateGroupName,
			errorContains: []string{"duplicate group name", "test", "indices 0 and 1"},
		},
		{
			name: "duplicate group names - at end",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build"},
					{Name: "test"},
					{Name: "deploy"},
					{Name: "test"},
				},
			},
			wantErr:       true,
			expectedError: ErrDuplicateGroupName,
			errorContains: []string{"duplicate group name", "test", "indices 1 and 3"},
		},
		{
			name:          "nil config",
			config:        nil,
			wantErr:       true,
			expectedError: ErrNilConfig,
			errorContains: []string{"must not be nil"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateGroupNames(tt.config)

			if tt.wantErr {
				require.Error(t, err, "expected error but got none")
				if tt.expectedError != nil {
					assert.True(t, errors.Is(err, tt.expectedError),
						"expected error type %v, got %v", tt.expectedError, err)
				}
				for _, substr := range tt.errorContains {
					assert.Contains(t, err.Error(), substr,
						"error message should contain %q", substr)
				}
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateGroupName tests the internal validateGroupName function
func TestValidateGroupName(t *testing.T) {
	tests := []struct {
		name      string
		groupName string
		wantErr   bool
	}{
		{
			name:      "valid lowercase",
			groupName: "build",
			wantErr:   false,
		},
		{
			name:      "valid uppercase",
			groupName: "BUILD",
			wantErr:   false,
		},
		{
			name:      "valid mixed case",
			groupName: "Build_Test",
			wantErr:   false,
		},
		{
			name:      "valid starting with underscore",
			groupName: "_internal",
			wantErr:   false,
		},
		{
			name:      "valid with numbers",
			groupName: "build123",
			wantErr:   false,
		},
		{
			name:      "valid single character",
			groupName: "a",
			wantErr:   false,
		},
		{
			name:      "valid underscore only",
			groupName: "_",
			wantErr:   false,
		},
		{
			name:      "invalid starting with number",
			groupName: "123build",
			wantErr:   true,
		},
		{
			name:      "invalid with hyphen",
			groupName: "build-test",
			wantErr:   true,
		},
		{
			name:      "invalid with dot",
			groupName: "build.test",
			wantErr:   true,
		},
		{
			name:      "invalid with space",
			groupName: "build test",
			wantErr:   true,
		},
		{
			name:      "invalid with special character @",
			groupName: "build@test",
			wantErr:   true,
		},
		{
			name:      "invalid with special character #",
			groupName: "build#test",
			wantErr:   true,
		},
		{
			name:      "invalid with special character $",
			groupName: "build$test",
			wantErr:   true,
		},
		{
			name:      "invalid empty string",
			groupName: "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGroupName(tt.groupName)

			if tt.wantErr {
				require.Error(t, err, "expected error but got none")
				assert.True(t, errors.Is(err, ErrInvalidGroupName),
					"error should wrap ErrInvalidGroupName")
				assert.Contains(t, err.Error(), tt.groupName,
					"error message should contain the invalid group name")
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}
