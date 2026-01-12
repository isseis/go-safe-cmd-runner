package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	commontesting "github.com/isseis/go-safe-cmd-runner/internal/common/testing"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertOutputFile compares expected string with actual *string OutputFile
func assertOutputFile(t *testing.T, expected string, actual *string, msg string) {
	t.Helper()
	if expected == "" {
		if actual != nil && *actual != "" {
			t.Errorf("%s: expected empty, got %q", msg, *actual)
		}
	} else {
		require.NotNil(t, actual, msg)
		assert.Equal(t, expected, *actual, msg)
	}
}

// Test parsing Command with output field
func TestCommandOutputFieldParsing(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		wantOutput  string
		wantErr     bool
	}{
		{
			name: "command with output field",
			tomlContent: `
[[groups]]
name = "test"
[[groups.commands]]
name = "ls"
cmd = "ls"
args = ["-la"]
output_file = "/tmp/ls-output.txt"
`,
			wantOutput: "/tmp/ls-output.txt",
			wantErr:    false,
		},
		{
			name: "command without output field",
			tomlContent: `
[[groups]]
name = "test"
[[groups.commands]]
name = "ls"
cmd = "ls"
args = ["-la"]
`,
			wantOutput: "",
			wantErr:    false,
		},
		{
			name: "command with empty output field",
			tomlContent: `
[[groups]]
name = "test"
[[groups.commands]]
name = "ls"
cmd = "ls"
args = ["-la"]
output_file = ""
`,
			wantOutput: "",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			config, err := loader.LoadConfigForTest([]byte(tt.tomlContent))

			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
				return
			}

			require.NoError(t, err, "Unexpected error")

			require.NotEmpty(t, config.Groups, "Expected at least one group")
			require.NotEmpty(t, config.Groups[0].Commands, "Expected at least one command")

			command := config.Groups[0].Commands[0]
			assertOutputFile(t, tt.wantOutput, command.OutputFile, "Output file mismatch")
		})
	}
}

// Test parsing GlobalConfig with max_output_size field
func TestGlobalConfigMaxOutputSizeParsing(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		wantMaxSize *int64
		wantErr     bool
	}{
		{
			name: "global config with max_output_size",
			tomlContent: `
[global]
workdir = "/tmp"
output_size_limit = 10485760
`,
			wantMaxSize: commontesting.Int64Ptr(10485760), // 10MB
			wantErr:     false,
		},
		{
			name: "global config without max_output_size",
			tomlContent: `
[global]
workdir = "/tmp"
`,
			wantMaxSize: nil, // Not specified, should be nil
			wantErr:     false,
		},
		{
			name: "global config with zero max_output_size",
			tomlContent: `
[global]
workdir = "/tmp"
output_size_limit = 0
`,
			wantMaxSize: commontesting.Int64Ptr(0), // Explicitly set to 0 (unlimited)
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			config, err := loader.LoadConfigForTest([]byte(tt.tomlContent))

			if tt.wantErr {
				assert.Error(t, err, "Expected error but got none")
				return
			}

			require.NoError(t, err, "Unexpected error")
			if tt.wantMaxSize == nil {
				assert.Nil(t, config.Global.OutputSizeLimit, "max_output_size should be nil")
			} else {
				require.NotNil(t, config.Global.OutputSizeLimit, "max_output_size should not be nil")
				assert.Equal(t, *tt.wantMaxSize, *config.Global.OutputSizeLimit, "max_output_size mismatch")
			}
		})
	}
}

// Test complete TOML configuration with output fields
func TestCompleteConfigWithOutput(t *testing.T) {
	tomlContent := `
version = "1.0"

[global]
workdir = "/tmp"
timeout = 300
output_size_limit = 20971520

[[groups]]
name = "build"
description = "Build commands"

[[groups.commands]]
name = "make"
description = "Run make command"
cmd = "make"
args = ["all"]
output_file = "/tmp/build.log"

[[groups.commands]]
name = "test"
description = "Run tests"
cmd = "make"
args = ["test"]
output_file = "/tmp/test.log"

[[groups]]
name = "utilities"
description = "Utility commands"

[[groups.commands]]
name = "ls"
description = "List files"
cmd = "ls"
args = ["-la"]
# No output field - should default to empty string
`

	loader := NewLoader()
	config, err := loader.LoadConfigForTest([]byte(tomlContent))
	require.NoError(t, err, "Unexpected error")

	// Test global configuration
	require.NotNil(t, config.Global.OutputSizeLimit, "OutputSizeLimit should not be nil")
	assert.Equal(t, int64(20971520), *config.Global.OutputSizeLimit, "max_output_size mismatch")

	// Test first group commands
	buildGroup := config.Groups[0]
	assert.Equal(t, "build", buildGroup.Name, "Group name mismatch")
	require.Len(t, buildGroup.Commands, 2, "Expected 2 commands in build group")

	makeCmd := buildGroup.Commands[0]
	assertOutputFile(t, "/tmp/build.log", makeCmd.OutputFile, "make command output mismatch")

	testCmd := buildGroup.Commands[1]
	assertOutputFile(t, "/tmp/test.log", testCmd.OutputFile, "test command output mismatch")

	// Test second group command (no output specified)
	utilGroup := config.Groups[1]
	assert.Equal(t, "utilities", utilGroup.Name, "Group name mismatch")

	lsCmd := utilGroup.Commands[0]
	assertOutputFile(t, "", lsCmd.OutputFile, "ls command output should be empty")
}

// Test direct unmarshaling of structures with new fields
func TestDirectUnmarshalingOfNewFields(t *testing.T) {
	t.Run("Command with output field", func(t *testing.T) {
		tomlData := `
name = "test-cmd"
cmd = "echo"
args = ["hello"]
output_file = "/tmp/test.txt"
`
		var cmd runnertypes.CommandSpec
		err := toml.Unmarshal([]byte(tomlData), &cmd)
		require.NoError(t, err, "Failed to unmarshal command")
		assertOutputFile(t, "/tmp/test.txt", cmd.OutputFile, "output field mismatch")
	})

	t.Run("GlobalConfig with max_output_size field", func(t *testing.T) {
		tomlData := `
workdir = "/tmp"
timeout = 600
output_size_limit = 5242880
`
		var global runnertypes.GlobalSpec
		err := toml.Unmarshal([]byte(tomlData), &global)
		require.NoError(t, err, "Failed to unmarshal global config")
		require.NotNil(t, global.OutputSizeLimit, "OutputSizeLimit should not be nil")
		assert.Equal(t, int64(5242880), *global.OutputSizeLimit, "max_output_size mismatch")
	})
}

// TestCommandWorkdirWithRunnerWorkdirVariable tests that command-level workdir
// can use the %{__runner_workdir} variable after expansion
func TestCommandWorkdirWithRunnerWorkdirVariable(t *testing.T) {
	tests := []struct {
		name            string
		tomlContent     string
		expectedCmdName string
		expectedWorkDir string // Expected after variable expansion
		runnerWorkdir   string // Value to set for __runner_workdir
	}{
		{
			name: "command workdir with __runner_workdir variable",
			tomlContent: `
[[groups]]
name = "build"
workdir = "/opt/project"

[[groups.commands]]
name = "compile"
cmd = "make"
args = ["build"]
workdir = "%{__runner_workdir}/src"
`,
			expectedCmdName: "compile",
			expectedWorkDir: "/opt/project/src",
			runnerWorkdir:   "/opt/project",
		},
		{
			name: "command workdir with __runner_workdir in temp dir scenario",
			tomlContent: `
[[groups]]
name = "test"

[[groups.commands]]
name = "run-tests"
cmd = "pytest"
workdir = "%{__runner_workdir}/tests"
`,
			expectedCmdName: "run-tests",
			expectedWorkDir: "/tmp/scr-test-abc123/tests",
			runnerWorkdir:   "/tmp/scr-test-abc123",
		},
		{
			name: "command without workdir should not fail",
			tomlContent: `
[[groups]]
name = "simple"

[[groups.commands]]
name = "echo-test"
cmd = "echo"
args = ["hello"]
`,
			expectedCmdName: "echo-test",
			expectedWorkDir: "", // No workdir specified
			runnerWorkdir:   "/tmp/scr-simple-xyz",
		},
		{
			name: "command args with __runner_workdir variable",
			tomlContent: `
[[groups]]
name = "backup"
workdir = "/var/backup"

[[groups.commands]]
name = "dump"
cmd = "pg_dump"
args = ["mydb", "-f", "%{__runner_workdir}/dump.sql"]
`,
			expectedCmdName: "dump",
			expectedWorkDir: "", // No command-level workdir
			runnerWorkdir:   "/var/backup",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Parse TOML
			loader := NewLoader()
			config, err := loader.LoadConfigForTest([]byte(tt.tomlContent))
			require.NoError(t, err, "Failed to parse TOML")
			require.NotEmpty(t, config.Groups, "Expected at least one group")

			group := &config.Groups[0]
			require.NotEmpty(t, group.Commands, "Expected at least one command")

			// 2. Simulate group variable expansion (mimic what ExecuteGroup does)
			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec:         &config.Global,
				ExpandedVars: make(map[string]string),
			}

			runtimeGroup, err := ExpandGroup(group, runtimeGlobal)
			require.NoError(t, err, "Failed to expand group")

			// 3. Set __runner_workdir (this would normally be set by resolveGroupWorkDir)
			runtimeGroup.EffectiveWorkDir = tt.runnerWorkdir
			runtimeGroup.ExpandedVars["__runner_workdir"] = tt.runnerWorkdir

			// 4. Expand command
			cmdSpec := &group.Commands[0]
			runtimeCmd, err := ExpandCommand(cmdSpec, nil, runtimeGroup, runtimeGlobal, common.NewUnsetTimeout(), commontesting.NewUnsetOutputSizeLimit())
			require.NoError(t, err, "Failed to expand command")

			// 5. Verify command name
			assert.Equal(t, tt.expectedCmdName, cmdSpec.Name, "Command name mismatch")

			// 6. Verify command workdir expansion (if specified)
			if cmdSpec.WorkDir != nil && *cmdSpec.WorkDir != "" {
				expandedWorkDir, err := ExpandString(
					*cmdSpec.WorkDir,
					runtimeGroup.ExpandedVars,
					"command["+cmdSpec.Name+"]",
					"workdir",
				)
				require.NoError(t, err, "Failed to expand command workdir")
				assert.Equal(t, tt.expectedWorkDir, expandedWorkDir, "Expanded workdir mismatch")
			} else if tt.expectedWorkDir != "" {
				assert.Fail(t, "Expected workdir field missing", "Expected workdir '%s', but command has no workdir field", tt.expectedWorkDir)
			}

			// 7. Verify args expansion (for the test case with args using __runner_workdir)
			if tt.name == "command args with __runner_workdir variable" {
				require.GreaterOrEqual(t, len(runtimeCmd.ExpandedArgs), 3, "Expected at least 3 args")

				expectedArg := tt.runnerWorkdir + "/dump.sql"
				assert.Equal(t, expectedArg, runtimeCmd.ExpandedArgs[2], "Args[2] mismatch")
			}
		})
	}
}
