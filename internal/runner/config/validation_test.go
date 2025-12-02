package config

import (
	"testing"

	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
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
					assert.ErrorIs(t, err, tt.errType)
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
			name: "valid group names - single character",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "a"},
					{Name: "Z"},
				},
			},
			wantErr: false,
		},
		{
			name: "valid group names - underscore only",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "_"},
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
			name: "invalid group name with special character @",
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
			name: "invalid group name with special character #",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build#test"},
				},
			},
			wantErr:       true,
			expectedError: ErrInvalidGroupName,
			errorContains: []string{"invalid group name", "build#test"},
		},
		{
			name: "invalid group name with special character $",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build$test"},
				},
			},
			wantErr:       true,
			expectedError: ErrInvalidGroupName,
			errorContains: []string{"invalid group name", "build$test"},
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
					assert.ErrorIs(t, err, tt.expectedError)
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

func TestValidateTimeouts(t *testing.T) {
	// Helper function to create a basic command spec
	makeCommand := func(name string, timeout *int32) runnertypes.CommandSpec {
		return runnertypes.CommandSpec{
			Name:    name,
			Cmd:     "/bin/echo",
			Timeout: timeout,
		}
	}

	// Helper function to create a group spec
	makeGroup := func(name string, commands ...runnertypes.CommandSpec) runnertypes.GroupSpec {
		return runnertypes.GroupSpec{
			Name:     name,
			Commands: commands,
		}
	}

	// Helper function to create a config spec
	makeConfig := func(globalTimeout *int32, groups ...runnertypes.GroupSpec) *runnertypes.ConfigSpec {
		cfg := &runnertypes.ConfigSpec{
			Groups: groups,
		}
		if globalTimeout != nil {
			cfg.Global.Timeout = globalTimeout
		}
		return cfg
	}

	tests := []struct {
		name             string
		config           *runnertypes.ConfigSpec
		expectError      bool
		expectedErr      error
		errorMustContain []string // All strings that must appear in error message
	}{
		{
			name:        "valid - no timeout specified",
			config:      makeConfig(nil, makeGroup("test_group", makeCommand("test_cmd", nil))),
			expectError: false,
		},
		{
			name:        "valid - positive global timeout",
			config:      makeConfig(commontesting.Int32Ptr(30), makeGroup("test_group", makeCommand("test_cmd", nil))),
			expectError: false,
		},
		{
			name:        "valid - zero global timeout",
			config:      makeConfig(commontesting.Int32Ptr(0), makeGroup("test_group", makeCommand("test_cmd", nil))),
			expectError: false,
		},
		{
			name:        "invalid - negative global timeout",
			config:      makeConfig(commontesting.Int32Ptr(-10), makeGroup("test_group", makeCommand("test_cmd", nil))),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
		},
		{
			name:        "valid - positive command timeout",
			config:      makeConfig(nil, makeGroup("test_group", makeCommand("test_cmd", commontesting.Int32Ptr(60)))),
			expectError: false,
		},
		{
			name:        "valid - zero command timeout",
			config:      makeConfig(nil, makeGroup("test_group", makeCommand("test_cmd", commontesting.Int32Ptr(0)))),
			expectError: false,
		},
		{
			name:        "invalid - negative command timeout",
			config:      makeConfig(nil, makeGroup("test_group", makeCommand("test_cmd", commontesting.Int32Ptr(-5)))),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
		},
		{
			name: "invalid - multiple negative command timeouts",
			config: makeConfig(nil, makeGroup("test_group",
				makeCommand("cmd1", commontesting.Int32Ptr(-1)),
				makeCommand("cmd2", commontesting.Int32Ptr(-2)),
			)),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
		},
		{
			name: "invalid - negative timeout in second group",
			config: makeConfig(nil,
				makeGroup("group1", makeCommand("cmd1", commontesting.Int32Ptr(30))),
				makeGroup("group2", makeCommand("cmd2", commontesting.Int32Ptr(-15))),
			),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
		},
		{
			name: "invalid - multiple errors reported together",
			config: makeConfig(commontesting.Int32Ptr(-5),
				makeGroup("group1", makeCommand("cmd1", commontesting.Int32Ptr(-10))),
				makeGroup("group2", makeCommand("cmd2", commontesting.Int32Ptr(-20))),
			),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
			errorMustContain: []string{
				"-5",   // global timeout value
				"cmd1", // first command name
				"-10",  // first command timeout value
				"cmd2", // second command name
				"-20",  // second command timeout value
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTimeouts(tt.config)

			if tt.expectError {
				require.Error(t, err, "expected error but got none")
				assert.ErrorIs(t, err, tt.expectedErr)
				for _, mustContain := range tt.errorMustContain {
					assert.Contains(t, err.Error(), mustContain,
						"error message should contain %q", mustContain)
				}
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}
