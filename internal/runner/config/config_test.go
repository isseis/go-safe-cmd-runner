package config

import (
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/pelletier/go-toml/v2"
)

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
			config, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(config.Groups) == 0 {
				t.Fatal("Expected at least one group")
			}

			if len(config.Groups[0].Commands) == 0 {
				t.Fatal("Expected at least one command")
			}

			command := config.Groups[0].Commands[0]
			if command.OutputFile != tt.wantOutput {
				t.Errorf("Expected output '%s', got '%s'", tt.wantOutput, command.OutputFile)
			}
		})
	}
}

// Test parsing GlobalConfig with max_output_size field
func TestGlobalConfigMaxOutputSizeParsing(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		wantMaxSize int64
		wantErr     bool
	}{
		{
			name: "global config with max_output_size",
			tomlContent: `
[global]
workdir = "/tmp"
output_size_limit = 10485760
`,
			wantMaxSize: 10485760, // 10MB
			wantErr:     false,
		},
		{
			name: "global config without max_output_size",
			tomlContent: `
[global]
workdir = "/tmp"
`,
			wantMaxSize: 0, // runner sets default value, so just check for no error
			wantErr:     false,
		},
		{
			name: "global config with zero max_output_size",
			tomlContent: `
[global]
workdir = "/tmp"
output_size_limit = 0
`,
			wantMaxSize: 0, // runner sets default value, so just check for no error
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewLoader()
			config, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if config.Global.OutputSizeLimit != tt.wantMaxSize {
				t.Errorf("Expected max_output_size %d, got %d", tt.wantMaxSize, config.Global.OutputSizeLimit)
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
	config, err := loader.LoadConfig([]byte(tomlContent))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Test global configuration
	if config.Global.OutputSizeLimit != 20971520 {
		t.Errorf("Expected max_output_size 20971520, got %d", config.Global.OutputSizeLimit)
	}

	// Test first group commands
	buildGroup := config.Groups[0]
	if buildGroup.Name != "build" {
		t.Errorf("Expected group name 'build', got '%s'", buildGroup.Name)
	}

	if len(buildGroup.Commands) != 2 {
		t.Fatalf("Expected 2 commands in build group, got %d", len(buildGroup.Commands))
	}

	makeCmd := buildGroup.Commands[0]
	if makeCmd.OutputFile != "/tmp/build.log" {
		t.Errorf("Expected make command output '/tmp/build.log', got '%s'", makeCmd.OutputFile)
	}

	testCmd := buildGroup.Commands[1]
	if testCmd.OutputFile != "/tmp/test.log" {
		t.Errorf("Expected test command output '/tmp/test.log', got '%s'", testCmd.OutputFile)
	}

	// Test second group command (no output specified)
	utilGroup := config.Groups[1]
	if utilGroup.Name != "utilities" {
		t.Errorf("Expected group name 'utilities', got '%s'", utilGroup.Name)
	}

	lsCmd := utilGroup.Commands[0]
	if lsCmd.OutputFile != "" {
		t.Errorf("Expected ls command output to be empty, got '%s'", lsCmd.OutputFile)
	}
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
		if err != nil {
			t.Fatalf("Failed to unmarshal command: %v", err)
		}

		if cmd.OutputFile != "/tmp/test.txt" {
			t.Errorf("Expected output '/tmp/test.txt', got '%s'", cmd.OutputFile)
		}
	})

	t.Run("GlobalConfig with max_output_size field", func(t *testing.T) {
		tomlData := `
workdir = "/tmp"
timeout = 600
output_size_limit = 5242880
`
		var global runnertypes.GlobalSpec
		err := toml.Unmarshal([]byte(tomlData), &global)
		if err != nil {
			t.Fatalf("Failed to unmarshal global config: %v", err)
		}

		if global.OutputSizeLimit != 5242880 {
			t.Errorf("Expected max_output_size 5242880, got %d", global.OutputSizeLimit)
		}
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
			config, err := loader.LoadConfig([]byte(tt.tomlContent))
			if err != nil {
				t.Fatalf("Failed to parse TOML: %v", err)
			}

			if len(config.Groups) == 0 {
				t.Fatal("Expected at least one group")
			}

			group := &config.Groups[0]
			if len(group.Commands) == 0 {
				t.Fatal("Expected at least one command")
			}

			// 2. Simulate group variable expansion (mimic what ExecuteGroup does)
			runtimeGlobal := &runnertypes.RuntimeGlobal{
				Spec:         &config.Global,
				ExpandedVars: make(map[string]string),
			}

			runtimeGroup, err := ExpandGroup(group, runtimeGlobal.ExpandedVars)
			if err != nil {
				t.Fatalf("Failed to expand group: %v", err)
			}

			// 3. Set __runner_workdir (this would normally be set by resolveGroupWorkDir)
			runtimeGroup.EffectiveWorkDir = tt.runnerWorkdir
			runtimeGroup.ExpandedVars["__runner_workdir"] = tt.runnerWorkdir

			// 4. Expand command
			cmdSpec := &group.Commands[0]
			runtimeCmd, err := ExpandCommand(cmdSpec, runtimeGroup.ExpandedVars, group.Name, nil)
			if err != nil {
				t.Fatalf("Failed to expand command: %v", err)
			}

			// 5. Verify command name
			if cmdSpec.Name != tt.expectedCmdName {
				t.Errorf("Expected command name '%s', got '%s'", tt.expectedCmdName, cmdSpec.Name)
			}

			// 6. Verify command workdir expansion (if specified)
			if cmdSpec.WorkDir != "" {
				expandedWorkDir, err := ExpandString(
					cmdSpec.WorkDir,
					runtimeGroup.ExpandedVars,
					"command["+cmdSpec.Name+"]",
					"workdir",
				)
				if err != nil {
					t.Fatalf("Failed to expand command workdir: %v", err)
				}

				if expandedWorkDir != tt.expectedWorkDir {
					t.Errorf("Expected expanded workdir '%s', got '%s'", tt.expectedWorkDir, expandedWorkDir)
				}
			} else if tt.expectedWorkDir != "" {
				t.Errorf("Expected workdir '%s', but command has no workdir field", tt.expectedWorkDir)
			}

			// 7. Verify args expansion (for the test case with args using __runner_workdir)
			if tt.name == "command args with __runner_workdir variable" {
				if len(runtimeCmd.ExpandedArgs) < 3 {
					t.Fatalf("Expected at least 3 args, got %d", len(runtimeCmd.ExpandedArgs))
				}

				expectedArg := tt.runnerWorkdir + "/dump.sql"
				if runtimeCmd.ExpandedArgs[2] != expectedArg {
					t.Errorf("Expected args[2] to be '%s', got '%s'", expectedArg, runtimeCmd.ExpandedArgs[2])
				}
			}
		})
	}
}
