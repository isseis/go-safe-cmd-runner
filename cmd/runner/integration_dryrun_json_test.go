//go:build test

package main

import (
	"encoding/json"
	"fmt"
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

			// Extract JSON from output (handles potential non-JSON prefix robustly)
			jsonOutput, err := extractJSON(string(output))
			require.NoError(t, err, "should be able to extract valid JSON from output")

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

// extractJSON attempts to extract valid JSON from output that may contain non-JSON prefix.
// It tries three strategies in order:
// 1. Parse the entire output as JSON (fast path for clean output)
// 2. Find and parse from the first '{' (handles simple prefix like timestamps)
// 3. Find and parse from the last '{' (handles error logs that might contain braces)
//
// Returns the extracted JSON string and an error if no valid JSON could be found.
func extractJSON(output string) (string, error) {
	output = strings.TrimSpace(output)

	// Strategy 1: Try parsing the entire output as-is
	if json.Valid([]byte(output)) {
		return output, nil
	}

	// Strategy 2: Try from first '{'
	if firstBrace := strings.Index(output, "{"); firstBrace != -1 {
		candidate := strings.TrimSpace(output[firstBrace:])
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}

	// Strategy 3: Try from last '{' (handles error logs with braces in strings)
	if lastBrace := strings.LastIndex(output, "{"); lastBrace != -1 {
		candidate := strings.TrimSpace(output[lastBrace:])
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("no valid JSON found in output")
}
