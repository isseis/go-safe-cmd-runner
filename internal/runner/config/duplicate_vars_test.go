//go:build test

package config_test

import (
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This test file verifies that the TOML parser (go-toml/v2) automatically
// detects and rejects duplicate variable definitions at all levels:
// - [global.vars]
// - [groups.vars]
// - [groups.commands.vars]
//
// This means we do NOT need to implement additional duplicate checking logic
// in our application code. The TOML parser provides this guarantee as part
// of the TOML specification.

func TestTOMLDuplicateVariables_Global(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		expectError bool
	}{
		{
			name: "duplicate global variable - rejected by TOML parser",
			tomlContent: `
[global.vars]
TestVar = "first"
TestVar = "second"

[[groups]]
name = "test"
`,
			expectError: true,
		},
		{
			name: "unique global variables",
			tomlContent: `
[global.vars]
TestVar1 = "value1"
TestVar2 = "value2"

[[groups]]
name = "test"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := config.NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.expectError {
				require.Error(t, err)
				t.Logf("Error from TOML parser: %v", err)
				// Verify that the error indicates duplicate key (parser-level rejection)
				// Check for both "key" and "already defined" to make the test robust
				errMsg := err.Error()
				assert.True(t,
					strings.Contains(errMsg, "key") && strings.Contains(errMsg, "already defined"),
					"expected error message to indicate duplicate key, got: %s", errMsg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestTOMLDuplicateVariables_GroupLevel(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		expectError bool
	}{
		{
			name: "duplicate group-level variable - rejected by TOML parser",
			tomlContent: `
[[groups]]
name = "test"

[groups.vars]
test_var = "first"
test_var = "second"
`,
			expectError: true,
		},
		{
			name: "unique group-level variables",
			tomlContent: `
[[groups]]
name = "test"

[groups.vars]
test_var1 = "value1"
test_var2 = "value2"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := config.NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.expectError {
				require.Error(t, err)
				t.Logf("Error from TOML parser: %v", err)
				// Verify that the error indicates duplicate key (parser-level rejection)
				// Check for both "key" and "already defined" to make the test robust
				errMsg := err.Error()
				assert.True(t,
					strings.Contains(errMsg, "key") && strings.Contains(errMsg, "already defined"),
					"expected error message to indicate duplicate key, got: %s", errMsg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestTOMLDuplicateVariables_CommandLevel(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		expectError bool
	}{
		{
			name: "duplicate command-level variable - rejected by TOML parser",
			tomlContent: `
[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"

[groups.commands.vars]
cmd_var = "first"
cmd_var = "second"
`,
			expectError: true,
		},
		{
			name: "unique command-level variables",
			tomlContent: `
[[groups]]
name = "test"

[[groups.commands]]
name = "cmd1"
cmd = "/bin/echo"

[groups.commands.vars]
cmd_var1 = "value1"
cmd_var2 = "value2"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := config.NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.expectError {
				require.Error(t, err)
				t.Logf("Error from TOML parser: %v", err)
				// Verify that the error indicates duplicate key (parser-level rejection)
				// Check for both "key" and "already defined" to make the test robust
				errMsg := err.Error()
				assert.True(t,
					strings.Contains(errMsg, "key") && strings.Contains(errMsg, "already defined"),
					"expected error message to indicate duplicate key, got: %s", errMsg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestTOMLDuplicateVariables_InlineTable(t *testing.T) {
	// Test if TOML allows duplicate keys in inline table syntax
	tests := []struct {
		name        string
		tomlContent string
		expectError bool
	}{
		{
			name: "duplicate in inline table syntax - rejected by TOML parser",
			tomlContent: `
[global]
vars = { TestVar = "first", TestVar = "second" }

[[groups]]
name = "test"
`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := config.NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.expectError {
				require.Error(t, err)
				t.Logf("Error from TOML parser: %v", err)
				// Verify that the error indicates duplicate key (parser-level rejection)
				// Check for both "key" and "already defined" to make the test robust
				errMsg := err.Error()
				assert.True(t,
					strings.Contains(errMsg, "key") && strings.Contains(errMsg, "already defined"),
					"expected error message to indicate duplicate key, got: %s", errMsg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}

func TestTOMLDuplicateFields_CommandTemplates(t *testing.T) {
	tests := []struct {
		name        string
		tomlContent string
		expectError bool
	}{
		{
			name: "duplicate cmd field in template - rejected by TOML parser",
			tomlContent: `
[command_templates.test]
cmd = "first"
cmd = "second"
`,
			expectError: true,
		},
		{
			name: "duplicate args field in template - rejected by TOML parser",
			tomlContent: `
[command_templates.test]
cmd = "test"
args = ["first"]
args = ["second"]
`,
			expectError: true,
		},
		{
			name: "duplicate env field in template - rejected by TOML parser",
			tomlContent: `
[command_templates.test]
cmd = "test"
env = ["A=1"]
env = ["B=2"]
`,
			expectError: true,
		},
		{
			name: "duplicate workdir field in template - rejected by TOML parser",
			tomlContent: `
[command_templates.test]
cmd = "test"
workdir = "/first"
workdir = "/second"
`,
			expectError: true,
		},
		{
			name: "unique template fields",
			tomlContent: `
[command_templates.test]
cmd = "test"
args = ["arg1"]
env = ["VAR=value"]
workdir = "/path"
`,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := config.NewLoader()
			cfg, err := loader.LoadConfig([]byte(tt.tomlContent))

			if tt.expectError {
				require.Error(t, err)
				t.Logf("Error from TOML parser: %v", err)
				// Verify that the error indicates duplicate key (parser-level rejection)
				errMsg := err.Error()
				assert.True(t,
					strings.Contains(errMsg, "key") && strings.Contains(errMsg, "already defined"),
					"expected error message to indicate duplicate key, got: %s", errMsg)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, cfg)
			}
		})
	}
}
