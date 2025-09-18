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
output = "/tmp/ls-output.txt"
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
output = ""
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
			if command.Output != tt.wantOutput {
				t.Errorf("Expected output '%s', got '%s'", tt.wantOutput, command.Output)
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
max_output_size = 10485760
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
			wantMaxSize: 0, // Default value
			wantErr:     false,
		},
		{
			name: "global config with zero max_output_size",
			tomlContent: `
[global]
workdir = "/tmp"
max_output_size = 0
`,
			wantMaxSize: 0,
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

			if config.Global.MaxOutputSize != tt.wantMaxSize {
				t.Errorf("Expected max_output_size %d, got %d", tt.wantMaxSize, config.Global.MaxOutputSize)
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
max_output_size = 20971520

[[groups]]
name = "build"
description = "Build commands"

[[groups.commands]]
name = "make"
description = "Run make command"
cmd = "make"
args = ["all"]
output = "/tmp/build.log"

[[groups.commands]]
name = "test"
description = "Run tests"
cmd = "make"
args = ["test"]
output = "/tmp/test.log"

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
	if config.Global.MaxOutputSize != 20971520 {
		t.Errorf("Expected max_output_size 20971520, got %d", config.Global.MaxOutputSize)
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
	if makeCmd.Output != "/tmp/build.log" {
		t.Errorf("Expected make command output '/tmp/build.log', got '%s'", makeCmd.Output)
	}

	testCmd := buildGroup.Commands[1]
	if testCmd.Output != "/tmp/test.log" {
		t.Errorf("Expected test command output '/tmp/test.log', got '%s'", testCmd.Output)
	}

	// Test second group command (no output specified)
	utilGroup := config.Groups[1]
	if utilGroup.Name != "utilities" {
		t.Errorf("Expected group name 'utilities', got '%s'", utilGroup.Name)
	}

	lsCmd := utilGroup.Commands[0]
	if lsCmd.Output != "" {
		t.Errorf("Expected ls command output to be empty, got '%s'", lsCmd.Output)
	}
}

// Test direct unmarshaling of structures with new fields
func TestDirectUnmarshalingOfNewFields(t *testing.T) {
	t.Run("Command with output field", func(t *testing.T) {
		tomlData := `
name = "test-cmd"
cmd = "echo"
args = ["hello"]
output = "/tmp/test.txt"
`
		var cmd runnertypes.Command
		err := toml.Unmarshal([]byte(tomlData), &cmd)
		if err != nil {
			t.Fatalf("Failed to unmarshal command: %v", err)
		}

		if cmd.Output != "/tmp/test.txt" {
			t.Errorf("Expected output '/tmp/test.txt', got '%s'", cmd.Output)
		}
	})

	t.Run("GlobalConfig with max_output_size field", func(t *testing.T) {
		tomlData := `
workdir = "/tmp"
timeout = 600
max_output_size = 5242880
`
		var global runnertypes.GlobalConfig
		err := toml.Unmarshal([]byte(tomlData), &global)
		if err != nil {
			t.Fatalf("Failed to unmarshal global config: %v", err)
		}

		if global.MaxOutputSize != 5242880 {
			t.Errorf("Expected max_output_size 5242880, got %d", global.MaxOutputSize)
		}
	})
}
