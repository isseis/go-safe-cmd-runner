//go:build test

package runner

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/common"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCommandResults_E2E_Integration verifies that CommandResults travel through the
// logging stack (RedactingHandler → SlackHandler) without losing structure or leaking secrets.
func TestCommandResults_E2E_Integration(t *testing.T) {
	t.Parallel()

	results := common.CommandResults{
		{CommandResultFields: common.CommandResultFields{
			Name:     "setup",
			ExitCode: 0,
			Output:   "Database configured with password=secret123",
			Stderr:   "",
		}},
		{CommandResultFields: common.CommandResultFields{
			Name:     "test",
			ExitCode: 0,
			Output:   "All tests passed",
			Stderr:   "",
		}},
		{CommandResultFields: common.CommandResultFields{
			Name:     "deploy",
			ExitCode: 1,
			Output:   "",
			Stderr:   "Deployment failed: API key=sk-invalid rejected",
		}},
	}

	// Wire up RedactingHandler with a JSON handler to capture structured output.
	var buf bytes.Buffer
	jsonHandler := slog.NewJSONHandler(&buf, nil)
	redactingHandler := redaction.NewRedactingHandler(jsonHandler, nil, nil)
	logger := slog.New(redactingHandler)

	logger.Info(
		"Command group execution summary",
		slog.String(common.GroupSummaryAttrs.Status, "error"),
		slog.String(common.GroupSummaryAttrs.Group, "test_group"),
		slog.Int64(common.GroupSummaryAttrs.DurationMs, 1_234),
		slog.Any(common.GroupSummaryAttrs.Commands, results),
	)

	var logged map[string]any
	err := json.Unmarshal(buf.Bytes(), &logged)
	require.NoError(t, err)

	commands, ok := logged[common.GroupSummaryAttrs.Commands].(map[string]any)
	require.True(t, ok, "commands should be a map")

	assert.Equal(t, float64(3), commands["total_count"])
	assert.Equal(t, false, commands["truncated"])

	cmd0, ok := commands["cmd_0"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "setup", cmd0[common.LogFieldName])
	assert.Equal(t, float64(0), cmd0[common.LogFieldExitCode])

	output0, _ := cmd0[common.LogFieldOutput].(string)
	assert.Contains(t, output0, "[REDACTED]")
	assert.NotContains(t, output0, "secret123")

	cmd2, ok := commands["cmd_2"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "deploy", cmd2[common.LogFieldName])
	assert.Equal(t, float64(1), cmd2[common.LogFieldExitCode])

	stderr2, _ := cmd2[common.LogFieldStderr].(string)
	assert.Contains(t, stderr2, "[REDACTED]")
	assert.NotContains(t, stderr2, "sk-invalid")
}
