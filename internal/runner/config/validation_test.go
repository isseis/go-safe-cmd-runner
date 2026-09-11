package config

import (
	"strconv"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
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

// TestValidateIdentifiers tests group and command identifier validation during
// config loading.
func TestValidateIdentifiers(t *testing.T) {
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
			errorContains: []string{"invalid group name", "must match pattern", "index 1"},
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
			errorContains: []string{"invalid group name", "must match pattern"},
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
			errorContains: []string{"invalid group name", "must match pattern"},
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
			errorContains: []string{"invalid group name", "must match pattern"},
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
			errorContains: []string{"invalid group name", "must match pattern"},
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
			errorContains: []string{"invalid group name", "must match pattern"},
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
			errorContains: []string{"invalid group name", "must match pattern"},
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
			errorContains: []string{"duplicate group name", "indices 0 and 2"},
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
			errorContains: []string{"duplicate group name", "indices 0 and 1"},
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
			errorContains: []string{"duplicate group name", "indices 1 and 3"},
		},
		{
			name: "valid command names",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{
						Name: "build",
						Commands: []runnertypes.CommandSpec{
							makeCommand("compile", nil),
							makeCommand("package-v2", nil),
						},
					},
				},
			},
			wantErr: false,
		},
		{
			// The same command name in two different groups is two distinct
			// scopes ("build/sync", "deploy/sync"), so it is accepted.
			name: "same command name in different groups",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand("sync", nil)}},
					{Name: "deploy", Commands: []runnertypes.CommandSpec{makeCommand("sync", nil)}},
				},
			},
			wantErr: false,
		},
		{
			name: "duplicate command names in one group",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{
						makeCommand("compile", nil),
						makeCommand("sync", nil),
						makeCommand("sync", nil),
					}},
				},
			},
			wantErr:       true,
			expectedError: ErrDuplicateCommandName,
			errorContains: []string{"duplicate command name", "groups[0].commands[1] and groups[0].commands[2]"},
		},
		{
			name: "empty command name",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand("", nil)}},
				},
			},
			wantErr:       true,
			expectedError: ErrEmptyCommandName,
			errorContains: []string{"command has empty name", "groups[0].commands[0]"},
		},
		{
			name: "control character only command name",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand("\n", nil)}},
				},
			},
			wantErr:       true,
			expectedError: ErrIdentifierContainsControlCharacter,
			errorContains: []string{"control character", "groups[0].commands[0]"},
		},
		{
			// The name still holds displayable characters, so only the
			// control-character check can reject it; removing that check makes
			// the displayable-content check pass this row.
			name: "format control with displayable characters",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand("backup\u202eevil", nil)}},
				},
			},
			wantErr:       true,
			expectedError: ErrIdentifierContainsControlCharacter,
			errorContains: []string{"control character", "groups[0].commands[0]"},
		},
		{
			name: "newline with displayable characters",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand("backup\nother", nil)}},
				},
			},
			wantErr:       true,
			expectedError: ErrIdentifierContainsControlCharacter,
			errorContains: []string{"control character", "groups[0].commands[0]"},
		},
		{
			// Half-width spaces are neither control characters nor format
			// controls, so only the displayable-content predicate rejects this
			// row.
			name: "whitespace-only command name",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand("   ", nil)}},
				},
			},
			wantErr:       true,
			expectedError: ErrIdentifierNotDisplayable,
			errorContains: []string{"displayable content", "groups[0].commands[0]"},
		},
		{
			name: "command name at byte limit",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand(strings.Repeat("a", common.MaxIdentifierBytes), nil)}},
				},
			},
			wantErr: false,
		},
		{
			name: "command name one byte over limit",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand(strings.Repeat("a", common.MaxIdentifierBytes+1), nil)}},
				},
			},
			wantErr:       true,
			expectedError: ErrIdentifierTooLong,
			errorContains: []string{"maximum length", "groups[0].commands[0]"},
		},
		{
			// 64 two-byte runes are exactly at the byte limit; a rune-counting
			// implementation would also accept 65 (130 bytes), which the next
			// row pins as a rejection.
			name: "multibyte command name at byte limit",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand(strings.Repeat("\u00e9", common.MaxIdentifierBytes/2), nil)}},
				},
			},
			wantErr: false,
		},
		{
			name: "multibyte command name over byte limit",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand(strings.Repeat("\u00e9", common.MaxIdentifierBytes/2)+"a", nil)}},
				},
			},
			wantErr:       true,
			expectedError: ErrIdentifierTooLong,
			errorContains: []string{"maximum length", "groups[0].commands[0]"},
		},
		{
			name: "group name one byte over limit",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: strings.Repeat("a", common.MaxIdentifierBytes+1)},
				},
			},
			wantErr:       true,
			expectedError: ErrIdentifierTooLong,
			errorContains: []string{"maximum length", "groups[0]"},
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
			err := ValidateIdentifiers(tt.config)

			if tt.wantErr {
				require.Error(t, err, "expected error but got none")
				if tt.expectedError != nil {
					assert.ErrorIs(t, err, tt.expectedError)
				}
				for _, substr := range tt.errorContains {
					assert.ErrorContains(t, err, substr,
						"error message should name %q", substr)
				}
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}

// TestValidateIdentifiers_DoesNotEchoRejectedNames pins that a rejected
// identifier never reaches the error text. These errors travel to stderr
// without redaction, and a rejected name can be a credential shape.
func TestValidateIdentifiers_DoesNotEchoRejectedNames(t *testing.T) {
	const credential = "AKIAIOSFODNN7EXAMPLE"

	tests := []struct {
		name          string
		config        *runnertypes.ConfigSpec
		expectedError error
	}{
		{
			name: "control character in a command name",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{makeCommand(credential+"\n", nil)}},
				},
			},
			expectedError: ErrIdentifierContainsControlCharacter,
		},
		{
			name: "pattern violation in a group name",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: credential + "/x"},
				},
			},
			expectedError: ErrInvalidGroupName,
		},
		{
			name: "duplicate group name",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: credential},
					{Name: credential},
				},
			},
			expectedError: ErrDuplicateGroupName,
		},
		{
			name: "duplicate command name",
			config: &runnertypes.ConfigSpec{
				Groups: []runnertypes.GroupSpec{
					{Name: "build", Commands: []runnertypes.CommandSpec{
						makeCommand(credential, nil),
						makeCommand(credential, nil),
					}},
				},
			},
			expectedError: ErrDuplicateCommandName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateIdentifiers(tt.config)
			require.ErrorIs(t, err, tt.expectedError)
			assert.NotContains(t, err.Error(), credential,
				"the rejected identifier must not appear in the error text")
		})
	}
}

// TestValidateIdentifiers_ChecksEveryCommand pins that the command loop does
// not stop at the first group: the violation sits in the second group's second
// command, and the error names that position.
func TestValidateIdentifiers_ChecksEveryCommand(t *testing.T) {
	cfg := &runnertypes.ConfigSpec{
		Groups: []runnertypes.GroupSpec{
			{Name: "backup", Commands: []runnertypes.CommandSpec{makeCommand("pg_dump", nil)}},
			{Name: "restore", Commands: []runnertypes.CommandSpec{makeCommand("psql", nil), makeCommand("", nil)}},
		},
	}

	err := ValidateIdentifiers(cfg)
	require.ErrorIs(t, err, ErrEmptyCommandName)
	assert.ErrorContains(t, err, "groups[1].commands[1]")
}

// TestLoadConfigRejectsInvalidCommandNames pins that the loader actually calls
// ValidateIdentifiers. Every check above passes when the call site is removed,
// and then a bad command name would be accepted for execution.
func TestLoadConfigRejectsInvalidCommandNames(t *testing.T) {
	tests := []struct {
		name        string
		commandName string
		wantErr     error
	}{
		{name: "empty command name", commandName: "", wantErr: ErrEmptyCommandName},
		{name: "control character command name", commandName: "backup\nother", wantErr: ErrIdentifierContainsControlCharacter},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := []byte("[[groups]]\nname = \"build\"\n\n[[groups.commands]]\nname = " +
				strconv.Quote(tt.commandName) + "\ncmd = \"/bin/echo\"\n")

			_, err := NewLoaderForTest().LoadConfigForTest(content)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestValidateTimeouts(t *testing.T) {
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
			config:      makeConfig(new(int32(30)), makeGroup("test_group", makeCommand("test_cmd", nil))),
			expectError: false,
		},
		{
			name:        "valid - zero global timeout",
			config:      makeConfig(new(int32(0)), makeGroup("test_group", makeCommand("test_cmd", nil))),
			expectError: false,
		},
		{
			name:        "invalid - negative global timeout",
			config:      makeConfig(new(int32(-10)), makeGroup("test_group", makeCommand("test_cmd", nil))),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
		},
		{
			name:        "valid - positive command timeout",
			config:      makeConfig(nil, makeGroup("test_group", makeCommand("test_cmd", new(int32(60))))),
			expectError: false,
		},
		{
			name:        "valid - zero command timeout",
			config:      makeConfig(nil, makeGroup("test_group", makeCommand("test_cmd", new(int32(0))))),
			expectError: false,
		},
		{
			name:        "invalid - negative command timeout",
			config:      makeConfig(nil, makeGroup("test_group", makeCommand("test_cmd", new(int32(-5))))),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
		},
		{
			name: "invalid - multiple negative command timeouts",
			config: makeConfig(nil, makeGroup("test_group",
				makeCommand("cmd1", new(int32(-1))),
				makeCommand("cmd2", new(int32(-2))),
			)),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
		},
		{
			name: "invalid - negative timeout in second group",
			config: makeConfig(nil,
				makeGroup("group1", makeCommand("cmd1", new(int32(30)))),
				makeGroup("group2", makeCommand("cmd2", new(int32(-15)))),
			),
			expectError: true,
			expectedErr: ErrNegativeTimeout,
		},
		{
			name: "invalid - multiple errors reported together",
			config: makeConfig(new(int32(-5)),
				makeGroup("group1", makeCommand("cmd1", new(int32(-10)))),
				makeGroup("group2", makeCommand("cmd2", new(int32(-20)))),
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
						Timeout: new(int32(-1)),
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
						Timeout: new(int32(30)),
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
						Timeout: new(int32(0)),
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
						Timeout: new(int32(-10)),
					},
					"template2": {
						Cmd:     "cat",
						Timeout: new(int32(-20)),
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
					Timeout: new(int32(-5)),
				},
				CommandTemplates: map[string]runnertypes.CommandTemplate{
					"bad_template": {
						Cmd:     "echo",
						Timeout: new(int32(-15)),
					},
				},
				Groups: []runnertypes.GroupSpec{
					makeGroup("test_group", makeCommand("bad_cmd", new(int32(-25)))),
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
					assert.ErrorContains(t, err, mustContain,
						"error message should name %q", mustContain)
				}
			} else {
				require.NoError(t, err, "expected no error but got: %v", err)
			}
		})
	}
}

// Note: WorkDir validation (absolute path check) is performed at expansion time
// in group_executor.go (resolveGroupWorkDir and resolveCommandWorkDir).
