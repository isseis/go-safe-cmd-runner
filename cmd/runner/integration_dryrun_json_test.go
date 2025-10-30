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

// extractJSON attempts to extract a JSON object from a string that may have a non-JSON prefix.
// It's designed to be robust against noisy output that can occur, for example, with `go run`.
func extractJSON(output string) (string, error) {
	// The most likely case is that the output is a valid JSON object, possibly with whitespace.
	trimmedOutput := strings.TrimSpace(output)
	if json.Valid([]byte(trimmedOutput)) {
		return trimmedOutput, nil
	}

	// If the full output is not valid JSON, it might have a prefix (e.g., build messages).
	// We assume the JSON object starts with '{'.
	firstBrace := strings.Index(trimmedOutput, "{")
	if firstBrace == -1 {
		return "", fmt.Errorf("no JSON object found in output: %s", output)
	}

	// The simplest case is that the JSON starts at the first brace.
	candidate := trimmedOutput[firstBrace:]
	if json.Valid([]byte(candidate)) {
		return candidate, nil
	}

	// If that fails, the prefix itself might contain a '{'.
	// We then try from the last '{', assuming it's the start of the main JSON object.
	// This is a heuristic that works for outputs like "log: {invalid}. {valid_json}".
	lastBrace := strings.LastIndex(trimmedOutput, "{")
	// No need to check for -1, as we already found at least one '{' above.
	if lastBrace > firstBrace {
		candidate = trimmedOutput[lastBrace:]
		if json.Valid([]byte(candidate)) {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not extract valid JSON from output: %s", output)
}
