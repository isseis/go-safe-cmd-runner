//go:build test

package main

import (
	"encoding/json"
	"os"
	"os/exec"
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

			// With Phase 5 changes, stdout contains pure JSON (logs go to stderr)
			var result struct {
				ResourceAnalyses []struct {
					Type       string         `json:"type"`
					Parameters map[string]any `json:"parameters"`
				} `json:"resource_analyses"`
			}

			err = json.Unmarshal(output, &result)
			require.NoError(t, err, "output should be valid JSON: %s", string(output))

			// Find command analysis (may not be the first element due to group analysis)
			require.NotEmpty(t, result.ResourceAnalyses, "should have at least one analysis")

			var cmdAnalysis *struct {
				Type       string         `json:"type"`
				Parameters map[string]any `json:"parameters"`
			}
			for i := range result.ResourceAnalyses {
				if result.ResourceAnalyses[i].Type == "command" {
					cmdAnalysis = &result.ResourceAnalyses[i]
					break
				}
			}
			require.NotNil(t, cmdAnalysis, "should have a command analysis")

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
