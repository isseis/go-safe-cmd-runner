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
			level:        "group:mygroup",
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
			variableName: "Var123",
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
				require.Error(t, err)
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
		{
			name: "invalid - negative timeout in template",
			config: &runnertypes.ConfigSpec{
				CommandTemplates: map[string]runnertypes.CommandTemplate{
					"test_template": {
						Cmd:     "echo",
						Timeout: commontesting.Int32Ptr(-1),
					},
				},
				Groups: []runnertypes.GroupSpec{
					makeGroup("test_group", makeCommand("test_cmd", nil)),
				},
			},
			expectError: true,
			expectedErr: ErrNegativeTimeout,
			errorMustContain: []string{
				"test_template",
				"-1",
			},
		},
		{
			name: "valid - positive timeout in template",
			config: &runnertypes.ConfigSpec{
				CommandTemplates: map[string]runnertypes.CommandTemplate{
					"test_template": {
						Cmd:     "echo",
						Timeout: commontesting.Int32Ptr(30),
					},
				},
				Groups: []runnertypes.GroupSpec{
					makeGroup("test_group", makeCommand("test_cmd", nil)),
				},
			},
			expectError: false,
		},
		{
			name: "valid - zero timeout in template",
			config: &runnertypes.ConfigSpec{
				CommandTemplates: map[string]runnertypes.CommandTemplate{
					"test_template": {
						Cmd:     "echo",
						Timeout: commontesting.Int32Ptr(0),
					},
				},
				Groups: []runnertypes.GroupSpec{
					makeGroup("test_group", makeCommand("test_cmd", nil)),
				},
			},
			expectError: false,
		},
		{
			name: "invalid - multiple negative template timeouts",
			config: &runnertypes.ConfigSpec{
				CommandTemplates: map[string]runnertypes.CommandTemplate{
					"template1": {
						Cmd:     "echo",
						Timeout: commontesting.Int32Ptr(-10),
					},
					"template2": {
						Cmd:     "cat",
						Timeout: commontesting.Int32Ptr(-20),
					},
				},
				Groups: []runnertypes.GroupSpec{
					makeGroup("test_group", makeCommand("test_cmd", nil)),
				},
			},
			expectError: true,
			expectedErr: ErrNegativeTimeout,
			errorMustContain: []string{
				"-10",
				"-20",
			},
		},
		{
			name: "invalid - negative timeouts in global, template, and command",
			config: &runnertypes.ConfigSpec{
				Global: runnertypes.GlobalSpec{
					Timeout: commontesting.Int32Ptr(-5),
				},
				CommandTemplates: map[string]runnertypes.CommandTemplate{
					"bad_template": {
						Cmd:     "echo",
						Timeout: commontesting.Int32Ptr(-15),
					},
				},
				Groups: []runnertypes.GroupSpec{
					makeGroup("test_group", makeCommand("bad_cmd", commontesting.Int32Ptr(-25))),
				},
			},
			expectError: true,
			expectedErr: ErrNegativeTimeout,
			errorMustContain: []string{
				"-5",  // global
				"-15", // template
				"-25", // command
				"bad_template",
				"bad_cmd",
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

// TestValidateWorkDir tests WorkDir validation with nil and absolute path checking
func TestValidateWorkDir(t *testing.T) {
	tests := []struct {
		name    string
		workdir *string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil workdir",
			workdir: nil,
			wantErr: false,
		},
		{
			name:    "empty string workdir",
			workdir: commontesting.StringPtr(""),
			wantErr: false,
		},
		{
			name:    "absolute path",
			workdir: commontesting.StringPtr("/home/user/dir"),
			wantErr: false,
		},
		{
			name:    "root directory",
			workdir: commontesting.StringPtr("/"),
			wantErr: false,
		},
		{
			name:    "relative path",
			workdir: commontesting.StringPtr("relative/path"),
			wantErr: true,
			errMsg:  "working directory",
		},
		{
			name:    "relative path with dot",
			workdir: commontesting.StringPtr("./path"),
			wantErr: true,
			errMsg:  "working directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkDir(tt.workdir)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestValidateEnvImport tests EnvImport validation against env_allowed list
func TestValidateEnvImport(t *testing.T) {
	tests := []struct {
		name       string
		envImport  []string
		envAllowed []string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "empty env_import",
			envImport:  []string{},
			envAllowed: []string{"HOME", "PATH"},
			wantErr:    false,
		},
		{
			name:       "nil env_import",
			envImport:  nil,
			envAllowed: []string{"HOME", "PATH"},
			wantErr:    false,
		},
		{
			name:       "allowed variables with mapping format",
			envImport:  []string{"home=HOME", "path=PATH"},
			envAllowed: []string{"HOME", "PATH"},
			wantErr:    false,
		},
		{
			name:       "single allowed variable with mapping",
			envImport:  []string{"home_dir=HOME"},
			envAllowed: []string{"HOME"},
			wantErr:    false,
		},
		{
			name:       "variable not in allowlist",
			envImport:  []string{"home=HOME", "secret=SECRET"},
			envAllowed: []string{"HOME", "PATH"},
			wantErr:    true,
			errMsg:     "SECRET",
		},
		{
			name:       "first of multiple not allowed",
			envImport:  []string{"secret=SECRET", "home=HOME"},
			envAllowed: []string{"HOME", "PATH"},
			wantErr:    true,
			errMsg:     "SECRET",
		},
		{
			name:       "empty allowlist rejects all",
			envImport:  []string{"home=HOME"},
			envAllowed: []string{},
			wantErr:    true,
			errMsg:     "HOME",
		},
		{
			name:       "invalid format - missing equals sign",
			envImport:  []string{"HOME"},
			envAllowed: []string{"HOME"},
			wantErr:    true,
			errMsg:     "invalid format",
		},
		{
			name:       "empty system var name not in allowlist",
			envImport:  []string{"home="},
			envAllowed: []string{"HOME"},
			wantErr:    true,
			errMsg:     "\"\"", // Empty string is valid format but not in allowlist
		},
		{
			name:       "same internal name different system var",
			envImport:  []string{"dir=HOME", "dir=PATH"},
			envAllowed: []string{"HOME", "PATH"},
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEnvImport(tt.envImport, tt.envAllowed)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
