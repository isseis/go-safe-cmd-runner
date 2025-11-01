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

func TestDryRun_JSON_Phase55_ErrorHandling(t *testing.T) {
	tests := []struct {
		name             string
		configContent    string
		expectedStatus   string
		expectedPhase    string
		expectError      bool
		expectErrorField bool
	}{
		{
			name: "success case with status and summary",
			configContent: `
[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`,
			expectedStatus:   "success",
			expectedPhase:    "completed",
			expectError:      false,
			expectErrorField: false,
		},
		{
			name: "pre-execution error with env allowlist violation",
			configContent: `
[global.env_allowed]
NOT_A_REAL_VAR = "internal_name"

[[groups]]
name = "test-group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`,
			expectedStatus:   "error",
			expectedPhase:    "pre_execution",
			expectError:      true,
			expectErrorField: true,
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

			output, err := cmd.CombinedOutput()

			if tt.expectError {
				// For pre-execution errors, the command should fail
				require.Error(t, err, "expected command to fail for error case")
			} else {
				require.NoError(t, err, "dry-run should succeed")
			}

			// Parse JSON output - need to find the JSON part
			// For pre-execution errors, JSON might not be output, so this test may need adjustment
			var result struct {
				Status string `json:"status"`
				Phase  string `json:"phase"`
				Error  *struct {
					Type      string         `json:"type"`
					Message   string         `json:"message"`
					Component string         `json:"component"`
					Details   map[string]any `json:"details,omitempty"`
				} `json:"error,omitempty"`
				Summary *struct {
					TotalResources int `json:"total_resources"`
					Successful     int `json:"successful"`
					Failed         int `json:"failed"`
					Skipped        int `json:"skipped"`
				} `json:"summary"`
				ResourceAnalyses []map[string]any `json:"resource_analyses"`
			}

			// Try to parse JSON from output
			outputStr := string(output)
			jsonStart := strings.Index(outputStr, "{")
			if jsonStart == -1 && !tt.expectError {
				t.Fatalf("No JSON found in output: %s", outputStr)
			}

			if jsonStart >= 0 {
				jsonOutput := outputStr[jsonStart:]
				err = json.Unmarshal([]byte(jsonOutput), &result)
				require.NoError(t, err, "output should be valid JSON: %s", jsonOutput)

				// Check status and phase
				assert.Equal(t, tt.expectedStatus, result.Status, "status should match")
				assert.Equal(t, tt.expectedPhase, result.Phase, "phase should match")

				// Check error field
				if tt.expectErrorField {
					require.NotNil(t, result.Error, "error field should be present")
					assert.NotEmpty(t, result.Error.Type, "error type should be set")
					assert.NotEmpty(t, result.Error.Message, "error message should be set")
				} else {
					assert.Nil(t, result.Error, "error field should not be present")
				}

				// Check summary is always present
				require.NotNil(t, result.Summary, "summary should always be present")
			}
		})
	}
}

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
				Status  string `json:"status"`
				Phase   string `json:"phase"`
				Summary *struct {
					TotalResources int `json:"total_resources"`
				} `json:"summary"`
				ResourceAnalyses []struct {
					Type       string         `json:"type"`
					Status     string         `json:"status"`
					Parameters map[string]any `json:"parameters"`
				} `json:"resource_analyses"`
			}

			err = json.Unmarshal(output, &result)
			require.NoError(t, err, "output should be valid JSON: %s", string(output))

			// Phase 5.5: Check top-level status and phase
			assert.Equal(t, "success", result.Status, "status should be success")
			assert.Equal(t, "completed", result.Phase, "phase should be completed")
			require.NotNil(t, result.Summary, "summary should be present")

			// Find command analysis (may not be the first element due to group analysis)
			require.NotEmpty(t, result.ResourceAnalyses, "should have at least one analysis")

			var cmdAnalysis *struct {
				Type       string         `json:"type"`
				Status     string         `json:"status"`
				Parameters map[string]any `json:"parameters"`
			}
			for i := range result.ResourceAnalyses {
				if result.ResourceAnalyses[i].Type == "command" {
					cmdAnalysis = &result.ResourceAnalyses[i]
					break
				}
			}
			require.NotNil(t, cmdAnalysis, "should have a command analysis")

			// Phase 5.5: Check resource-level status
			assert.Equal(t, "success", cmdAnalysis.Status, "command status should be success")

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
