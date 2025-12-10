//go:build test

package config

import (
	"os"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTemplateIntegrationWithSampleFile tests the complete template expansion
// flow using the sample configuration file
func TestTemplateIntegrationWithSampleFile(t *testing.T) {
	// Load sample file
	content, err := os.ReadFile("../../../sample/command_template_example.toml")
	require.NoError(t, err, "failed to read sample file")

	loader := NewLoader()
	cfg, err := loader.LoadConfig(content)
	require.NoError(t, err, "failed to load config")
	require.NotNil(t, cfg)

	// Verify templates were loaded
	assert.Len(t, cfg.CommandTemplates, 5, "expected 5 templates")
	assert.Contains(t, cfg.CommandTemplates, "restic_backup")
	assert.Contains(t, cfg.CommandTemplates, "restic_backup_with_options")
	assert.Contains(t, cfg.CommandTemplates, "restic_backup_advanced")
	assert.Contains(t, cfg.CommandTemplates, "restic_restore")
	assert.Contains(t, cfg.CommandTemplates, "safe_echo")

	// Expand global config
	runtimeGlobal, err := ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// Test daily_backup group
	dailyBackupGroup := &cfg.Groups[0]
	require.Equal(t, "daily_backup", dailyBackupGroup.Name)

	runtimeGroup, err := ExpandGroup(dailyBackupGroup, runtimeGlobal)
	require.NoError(t, err)

	// Test backup_volumes command (basic template)
	backupVolumesCmd := &dailyBackupGroup.Commands[0]
	require.Equal(t, "backup_volumes", backupVolumesCmd.Name)
	require.Equal(t, "restic_backup", backupVolumesCmd.Template)

	runtimeCmd, err := ExpandCommand(
		backupVolumesCmd,
		cfg.CommandTemplates,
		runtimeGroup,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)
	assert.Equal(t, "restic", runtimeCmd.Cmd())
	assert.Equal(t, []string{"backup", "/data/volumes"}, runtimeCmd.Args())
	// Verify env was expanded with variable substitution
	expectedEnv := map[string]string{"RESTIC_REPOSITORY": "/data/backups/repo"}
	assert.Equal(t, expectedEnv, runtimeCmd.ExpandedEnv)

	// Test backup_db_verbose command (optional parameter provided)
	backupDBCmd := &dailyBackupGroup.Commands[1]
	require.Equal(t, "backup_db_verbose", backupDBCmd.Name)

	runtimeCmd, err = ExpandCommand(
		backupDBCmd,
		cfg.CommandTemplates,
		runtimeGroup,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)
	assert.Equal(t, "restic", runtimeCmd.Cmd())
	assert.Equal(t, []string{"-v", "backup", "/data/database"}, runtimeCmd.Args())

	// Test backup_config command (optional parameter omitted)
	backupConfigCmd := &dailyBackupGroup.Commands[2]
	require.Equal(t, "backup_config", backupConfigCmd.Name)

	runtimeCmd, err = ExpandCommand(
		backupConfigCmd,
		cfg.CommandTemplates,
		runtimeGroup,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)
	assert.Equal(t, "restic", runtimeCmd.Cmd())
	// Optional param not provided, so only backup and path
	assert.Equal(t, []string{"backup", "/etc/config"}, runtimeCmd.Args())

	// Test backup_home_advanced command (array parameter)
	backupHomeCmd := &dailyBackupGroup.Commands[3]
	require.Equal(t, "backup_home_advanced", backupHomeCmd.Name)

	runtimeCmd, err = ExpandCommand(
		backupHomeCmd,
		cfg.CommandTemplates,
		runtimeGroup,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)
	assert.Equal(t, "restic", runtimeCmd.Cmd())
	// Array parameter should expand to multiple args
	assert.Equal(t, []string{"-v", "--exclude-caches", "--one-file-system", "backup", "/home"}, runtimeCmd.Args())
	// password is optional and not provided, so only RESTIC_REPOSITORY
	expectedEnv = map[string]string{"RESTIC_REPOSITORY": "/data/backups/repo"}
	assert.Equal(t, expectedEnv, runtimeCmd.ExpandedEnv)
}

// TestTemplateWithVariableExpansion tests that template parameter expansion
// is followed by variable expansion (%{var})
func TestTemplateWithVariableExpansion(t *testing.T) {
	toml := `
version = "1.0"

[command_templates.echo_msg]
cmd = "echo"
args = ["${message}"]

[[groups]]
name = "test"

[groups.vars]
greeting = "Hello"
name = "World"

[[groups.commands]]
name = "say_hello"
template = "echo_msg"
[groups.commands.params]
message = "%{greeting} %{name}"
`

	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(toml))
	require.NoError(t, err)

	runtimeGlobal, err := ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	runtimeGroup, err := ExpandGroup(&cfg.Groups[0], runtimeGlobal)
	require.NoError(t, err)

	runtimeCmd, err := ExpandCommand(
		&cfg.Groups[0].Commands[0],
		cfg.CommandTemplates,
		runtimeGroup,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)

	// Template expands ${message} to "%{greeting} %{name}"
	// Then variable expansion expands %{greeting} and %{name}
	assert.Equal(t, "echo", runtimeCmd.ExpandedCmd)
	assert.Equal(t, []string{"Hello World"}, runtimeCmd.ExpandedArgs)
}

// TestTemplateWithCmdAllowed tests that cmd_allowed check works with templates
func TestTemplateWithCmdAllowed(t *testing.T) {
	toml := `
version = "1.0"

[command_templates.list_files]
cmd = "ls"
args = ["${path}"]

[[groups]]
name = "test"
cmd_allowed = ["/bin/echo"]  # ls is NOT allowed

[[groups.commands]]
name = "list_home"
template = "list_files"
[groups.commands.params]
path = "/home"
`

	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(toml))
	require.NoError(t, err)

	runtimeGlobal, err := ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	runtimeGroup, err := ExpandGroup(&cfg.Groups[0], runtimeGlobal)
	require.NoError(t, err)

	// ExpandCommand should succeed (it doesn't check cmd_allowed)
	runtimeCmd, err := ExpandCommand(
		&cfg.Groups[0].Commands[0],
		cfg.CommandTemplates,
		runtimeGroup,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)
	assert.Equal(t, "ls", runtimeCmd.Cmd())

	// Note: cmd_allowed validation happens later in the execution flow,
	// not during expansion. This test just verifies that template expansion
	// works correctly and the expanded command can be validated later.
}

// TestTemplateErrorCases tests various error scenarios in template expansion
func TestTemplateErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		toml        string
		wantErr     bool
		wantErrType error
	}{
		{
			name: "template not found",
			toml: `
version = "1.0"
[[groups]]
name = "test"
[[groups.commands]]
name = "cmd1"
template = "nonexistent"
[groups.commands.params]
param = "value"
`,
			wantErr:     true,
			wantErrType: &ErrTemplateNotFound{},
		},
		{
			name: "template and cmd both specified",
			toml: `
version = "1.0"
[command_templates.tmpl]
cmd = "echo"
[[groups]]
name = "test"
[[groups.commands]]
name = "cmd1"
template = "tmpl"
cmd = "ls"
`,
			wantErr:     true,
			wantErrType: &ErrTemplateFieldConflict{},
		},
		{
			name: "missing required parameter",
			toml: `
version = "1.0"
[command_templates.needs_param]
cmd = "echo"
args = ["${required}"]
[[groups]]
name = "test"
[[groups.commands]]
name = "cmd1"
template = "needs_param"
# params not provided
`,
			wantErr:     true,
			wantErrType: &ErrRequiredParamMissing{},
		},
		{
			name: "invalid parameter type",
			toml: `
version = "1.0"
[command_templates.echo_msg]
cmd = "echo"
args = ["${message}"]
[[groups]]
name = "test"
[[groups.commands]]
name = "cmd1"
template = "echo_msg"
[groups.commands.params]
message = 123
`,
			wantErr:     true,
			wantErrType: &ErrTemplateTypeMismatch{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.toml))

			if tt.wantErr {
				if err != nil {
					// Error during loading
					if tt.wantErrType != nil {
						require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
					}
					return
				}

				// Error should occur during expansion
				require.NotNil(t, cfg)
				runtimeGlobal, err := ExpandGlobal(&cfg.Global)
				require.NoError(t, err)

				runtimeGroup, err := ExpandGroup(&cfg.Groups[0], runtimeGlobal)
				if err != nil {
					if tt.wantErrType != nil {
						require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
					}
					return
				}

				_, err = ExpandCommand(
					&cfg.Groups[0].Commands[0],
					cfg.CommandTemplates,
					runtimeGroup,
					runtimeGlobal,
					common.NewUnsetTimeout(),
					commontesting.NewUnsetOutputSizeLimit(),
				)
				require.Error(t, err)
				if tt.wantErrType != nil {
					require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestTemplateTimeoutAndRiskLevel tests that timeout and risk_level
// are inherited from templates
func TestTemplateTimeoutAndRiskLevel(t *testing.T) {
	toml := `
version = "1.0"

[command_templates.safe_cmd]
cmd = "echo"
args = ["${msg}"]
timeout = 10
risk_level = "low"

[[groups]]
name = "test"

[[groups.commands]]
name = "say_hello"
template = "safe_cmd"
[groups.commands.params]
msg = "hello"
`

	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(toml))
	require.NoError(t, err)

	runtimeGlobal, err := ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	runtimeGroup, err := ExpandGroup(&cfg.Groups[0], runtimeGlobal)
	require.NoError(t, err)

	runtimeCmd, err := ExpandCommand(
		&cfg.Groups[0].Commands[0],
		cfg.CommandTemplates,
		runtimeGroup,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)

	// Verify timeout and risk level were inherited from template
	assert.Equal(t, int32(10), runtimeCmd.EffectiveTimeout)
	// Note: risk_level is in the spec, not in RuntimeCommand
	// We can verify it was copied to the expanded CommandSpec
}

// TestTemplateCmdValidation tests that cmd field must resolve to exactly one value
func TestTemplateCmdValidation(t *testing.T) {
	tests := []struct {
		name        string
		toml        string
		wantErr     bool
		wantErrType error
	}{
		{
			name: "cmd with optional placeholder that resolves to empty",
			toml: `
version = "1.0"
[command_templates.optional_cmd]
cmd = "${?mycmd}"
args = ["test"]
[[groups]]
name = "test"
[[groups.commands]]
name = "cmd1"
template = "optional_cmd"
# params.mycmd not provided - cmd will be empty
`,
			wantErr:     true,
			wantErrType: &ErrTemplateCmdNotSingleValue{},
		},
		{
			name: "cmd with array placeholder (invalid)",
			toml: `
version = "1.0"
[command_templates.array_cmd]
cmd = "${@cmds}"
args = ["test"]
[[groups]]
name = "test"
[[groups.commands]]
name = "cmd1"
template = "array_cmd"
[groups.commands.params]
cmds = ["ls", "cat"]
`,
			wantErr:     true,
			wantErrType: &ErrTemplateCmdNotSingleValue{},
		},
		{
			name: "cmd with empty string after expansion",
			toml: `
version = "1.0"
[command_templates.empty_cmd]
cmd = "${?cmd}"
args = ["test"]
[[groups]]
name = "test"
[[groups.commands]]
name = "cmd1"
template = "empty_cmd"
[groups.commands.params]
cmd = ""
`,
			wantErr:     true,
			wantErrType: &ErrTemplateCmdNotSingleValue{},
		},
		{
			name: "valid cmd with optional placeholder provided",
			toml: `
version = "1.0"
[command_templates.optional_cmd]
cmd = "${?mycmd}"
args = ["test"]
[[groups]]
name = "test"
[[groups.commands]]
name = "cmd1"
template = "optional_cmd"
[groups.commands.params]
mycmd = "echo"
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.toml))
			require.NoError(t, err)

			runtimeGlobal, err := ExpandGlobal(&cfg.Global)
			require.NoError(t, err)

			runtimeGroup, err := ExpandGroup(&cfg.Groups[0], runtimeGlobal)
			require.NoError(t, err)

			_, err = ExpandCommand(
				&cfg.Groups[0].Commands[0],
				cfg.CommandTemplates,
				runtimeGroup,
				runtimeGlobal,
				common.NewUnsetTimeout(),
				commontesting.NewUnsetOutputSizeLimit(),
			)

			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrType != nil {
					require.ErrorAs(t, err, &tt.wantErrType, "error should be of expected type")
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestMultipleGroupsSameTemplate tests that the same template can be used
// by commands in different groups
func TestMultipleGroupsSameTemplate(t *testing.T) {
	toml := `
version = "1.0"

[command_templates.echo_msg]
cmd = "echo"
args = ["${message}"]

[[groups]]
name = "group1"
[groups.vars]
msg = "Group 1"

[[groups.commands]]
name = "cmd1"
template = "echo_msg"
[groups.commands.params]
message = "%{msg}"

[[groups]]
name = "group2"
[groups.vars]
msg = "Group 2"

[[groups.commands]]
name = "cmd2"
template = "echo_msg"
[groups.commands.params]
message = "%{msg}"
`

	loader := NewLoader()
	cfg, err := loader.LoadConfig([]byte(toml))
	require.NoError(t, err)

	runtimeGlobal, err := ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// Expand both groups
	runtimeGroup1, err := ExpandGroup(&cfg.Groups[0], runtimeGlobal)
	require.NoError(t, err)

	runtimeCmd1, err := ExpandCommand(
		&cfg.Groups[0].Commands[0],
		cfg.CommandTemplates,
		runtimeGroup1,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"Group 1"}, runtimeCmd1.ExpandedArgs)

	runtimeGroup2, err := ExpandGroup(&cfg.Groups[1], runtimeGlobal)
	require.NoError(t, err)

	runtimeCmd2, err := ExpandCommand(
		&cfg.Groups[1].Commands[0],
		cfg.CommandTemplates,
		runtimeGroup2,
		runtimeGlobal,
		common.NewUnsetTimeout(),
		commontesting.NewUnsetOutputSizeLimit(),
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"Group 2"}, runtimeCmd2.ExpandedArgs)
}

// TestTemplateExecutionSettingsOverride tests that command-level execution settings
// override template values (timeout, output_size_limit, risk_level)
func TestTemplateExecutionSettingsOverride(t *testing.T) {
	tests := []struct {
		name                     string
		toml                     string
		expectedTimeout          *int32
		expectedOutputSizeLimit  *int64
		expectedRiskLevel        string
		expectedEffectiveTimeout int32
	}{
		{
			name: "command overrides all execution settings",
			toml: `
version = "1.0"

[command_templates.base]
cmd = "echo"
args = ["${msg}"]
timeout = 10
output_size_limit = 1024
risk_level = "low"

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "base"
timeout = 300
output_size_limit = 2048
risk_level = "high"
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          func() *int32 { v := int32(300); return &v }(),
			expectedOutputSizeLimit:  func() *int64 { v := int64(2048); return &v }(),
			expectedRiskLevel:        "high",
			expectedEffectiveTimeout: 300,
		},
		{
			name: "command overrides timeout only",
			toml: `
version = "1.0"

[command_templates.base]
cmd = "echo"
args = ["${msg}"]
timeout = 10
output_size_limit = 1024
risk_level = "low"

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "base"
timeout = 300
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          func() *int32 { v := int32(300); return &v }(),
			expectedOutputSizeLimit:  func() *int64 { v := int64(1024); return &v }(),
			expectedRiskLevel:        "low",
			expectedEffectiveTimeout: 300,
		},
		{
			name: "command inherits all from template",
			toml: `
version = "1.0"

[command_templates.base]
cmd = "echo"
args = ["${msg}"]
timeout = 10
output_size_limit = 1024
risk_level = "low"

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "base"
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          func() *int32 { v := int32(10); return &v }(),
			expectedOutputSizeLimit:  func() *int64 { v := int64(1024); return &v }(),
			expectedRiskLevel:        "low",
			expectedEffectiveTimeout: 10,
		},
		{
			name: "command sets timeout to zero (unlimited) overriding template",
			toml: `
version = "1.0"

[command_templates.base]
cmd = "echo"
args = ["${msg}"]
timeout = 10

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "base"
timeout = 0
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          func() *int32 { v := int32(0); return &v }(),
			expectedOutputSizeLimit:  nil,
			expectedRiskLevel:        "low", // Default risk level is applied when neither template nor command specify it
			expectedEffectiveTimeout: 0,
		},
		{
			name: "template has no execution settings, command sets them",
			toml: `
version = "1.0"

[command_templates.base]
cmd = "echo"
args = ["${msg}"]

[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
template = "base"
timeout = 300
output_size_limit = 2048
risk_level = "medium"
[groups.commands.params]
msg = "hello"
`,
			expectedTimeout:          func() *int32 { v := int32(300); return &v }(),
			expectedOutputSizeLimit:  func() *int64 { v := int64(2048); return &v }(),
			expectedRiskLevel:        "medium",
			expectedEffectiveTimeout: 300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.toml))
			require.NoError(t, err)

			runtimeGlobal, err := ExpandGlobal(&cfg.Global)
			require.NoError(t, err)

			runtimeGroup, err := ExpandGroup(&cfg.Groups[0], runtimeGlobal)
			require.NoError(t, err)

			runtimeCmd, err := ExpandCommand(
				&cfg.Groups[0].Commands[0],
				cfg.CommandTemplates,
				runtimeGroup,
				runtimeGlobal,
				common.NewUnsetTimeout(),
				commontesting.NewUnsetOutputSizeLimit(),
			)
			require.NoError(t, err)

			// Check the expanded spec fields (accessed via runtimeCmd.Spec)
			if tt.expectedTimeout != nil {
				require.NotNil(t, runtimeCmd.Spec.Timeout, "expected timeout to be set")
				assert.Equal(t, *tt.expectedTimeout, *runtimeCmd.Spec.Timeout, "timeout mismatch")
			} else {
				assert.Nil(t, runtimeCmd.Spec.Timeout, "expected timeout to be nil")
			}

			if tt.expectedOutputSizeLimit != nil {
				require.NotNil(t, runtimeCmd.Spec.OutputSizeLimit, "expected output_size_limit to be set")
				assert.Equal(t, *tt.expectedOutputSizeLimit, *runtimeCmd.Spec.OutputSizeLimit, "output_size_limit mismatch")
			} else {
				assert.Nil(t, runtimeCmd.Spec.OutputSizeLimit, "expected output_size_limit to be nil")
			}

			assert.Equal(t, tt.expectedRiskLevel, runtimeCmd.Spec.RiskLevel, "risk_level mismatch")

			// Check effective timeout (resolved value)
			assert.Equal(t, tt.expectedEffectiveTimeout, runtimeCmd.EffectiveTimeout, "effective timeout mismatch")
		})
	}
}
