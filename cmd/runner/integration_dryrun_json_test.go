//go:build test

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDryRun_JSON_TimeoutResolutionContext(t *testing.T) {
	tests := []struct {
		name            string
		configContent   string
		expectedTimeout float64
		expectedLevel   string
	}{
		{
			name: "command level timeout in JSON dry-run",
			configContent: `
[global]
timeout = 60

[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
timeout = 30
`,
			expectedTimeout: 30,
			expectedLevel:   "command",
		},
		{
			name: "global level timeout in JSON dry-run",
			configContent: `
[global]
timeout = 45

[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`,
			expectedTimeout: 45,
			expectedLevel:   "global",
		},
		{
			name: "default timeout in JSON dry-run",
			configContent: `
[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`,
			expectedTimeout: 60,
			expectedLevel:   "default",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tmpFile, err := os.CreateTemp("", "test-config-*.toml")
			require.NoError(t, err)
			defer os.Remove(tmpFile.Name())

			_, err = tmpFile.WriteString(tt.configContent)
			require.NoError(t, err)
			tmpFile.Close()

			// Run command in dry-run mode with JSON output
			cmd := exec.Command("go", "run", ".", "-config", tmpFile.Name(), "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "json", "-log-level", "error")
			cmd.Dir = "."

			output, err := cmd.Output() // Use Output() instead of CombinedOutput() to get only stdout
			require.NoError(t, err, "dry-run should succeed")

			// Find the start of JSON output (skip debug information)
			outputStr := string(output)
jsonOutput := strings.TrimSpace(outputStr)
isJsonObject := strings.HasPrefix(jsonOutput, "{") && strings.HasSuffix(jsonOutput, "}")
require.True(t, isJsonObject, "output should be a JSON object: %s", jsonOutput)

			// Parse JSON output
			var result struct {
				ResourceAnalyses []struct {
					Type       string         `json:"type"`
					Parameters map[string]any `json:"parameters"`
				} `json:"resource_analyses"`
			}

			err = json.Unmarshal([]byte(jsonOutput), &result)
			require.NoError(t, err, "output should be valid JSON: %s", jsonOutput)

			// Find command analysis
			require.NotEmpty(t, result.ResourceAnalyses, "should have at least one analysis")

			cmdAnalysis := result.ResourceAnalyses[0]
			assert.Equal(t, "command", cmdAnalysis.Type)

			// Check timeout parameters
			timeout, ok := cmdAnalysis.Parameters["timeout"]
			require.True(t, ok, "parameters should contain timeout")
			assert.Equal(t, tt.expectedTimeout, timeout, "timeout should match expected value")

			timeoutLevel, ok := cmdAnalysis.Parameters["timeout_level"]
			require.True(t, ok, "parameters should contain timeout_level")
			assert.Equal(t, tt.expectedLevel, timeoutLevel, "timeout_level should match expected value")
		})
	}
}
