package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sensitiveTestHelper encapsulates common test setup and execution logic
type sensitiveTestHelper struct {
	t       *testing.T
	tempDir string
}

// newSensitiveTestHelper creates a new test helper with a temporary directory
func newSensitiveTestHelper(t *testing.T) *sensitiveTestHelper {
	t.Helper()
	return &sensitiveTestHelper{
		t:       t,
		tempDir: t.TempDir(),
	}
}

// createConfigAndPaths creates a config file and hash directory, returning their paths
func (h *sensitiveTestHelper) createConfigAndPaths(configContent string) (string, string) {
	h.t.Helper()

	configPath := filepath.Join(h.tempDir, "config.toml")
	err := os.WriteFile(configPath, []byte(configContent), 0o644)
	require.NoError(h.t, err, "Should be able to write config file")

	hashDir := filepath.Join(h.tempDir, "hashes")
	err = os.MkdirAll(hashDir, 0o700)
	require.NoError(h.t, err, "Should be able to create hash directory")

	return configPath, hashDir
}

// loadConfig loads and expands the configuration
func (h *sensitiveTestHelper) loadConfig(configPath string, runID string) (*runnertypes.ConfigSpec, *runnertypes.RuntimeGlobal, *verification.Manager) {
	h.t.Helper()

	verificationManager, err := verification.NewManagerForDryRun()
	require.NoError(h.t, err, "Should create verification manager")

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, runID)
	require.NoError(h.t, err, "Should load config")

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(h.t, err, "Should expand global config")

	return cfg, runtimeGlobal, verificationManager
}

// executeWithCapturedOutput executes the runner and captures stdout
func (h *sensitiveTestHelper) executeWithCapturedOutput(
	cfg *runnertypes.ConfigSpec,
	runtimeGlobal *runnertypes.RuntimeGlobal,
	verificationManager *verification.Manager,
	dryRunOptions *resource.DryRunOptions,
	runID string,
) string {
	h.t.Helper()

	privMgr := privilege.NewManager(slog.Default())

	runnerOptions := []runner.Option{
		runner.WithVerificationManager(verificationManager),
		runner.WithPrivilegeManager(privMgr),
		runner.WithRunID(runID),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithKeepTempDirs(false),
		runner.WithDryRun(dryRunOptions),
	}

	r1, err := runner.NewRunner(cfg, runnerOptions...)
	require.NoError(h.t, err, "Should create runner")

	// Capture stdout
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(h.t, err, "Should create pipe")
	os.Stdout = w

	// Execute all groups in dry-run mode with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		defer w.Close()
		errCh <- r1.ExecuteAll(ctx)
	}()

	// Read captured output
	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	require.NoError(h.t, err, "Should copy stdout without error")
	os.Stdout = oldStdout

	// Check for execution error
	execErr := <-errCh
	require.NoError(h.t, execErr, "ExecuteAll should run without error")

	// Verify dry-run results
	result := r1.GetDryRunResults()
	require.NotNil(h.t, result, "Should have dry-run results")
	require.Greater(h.t, len(result.ResourceAnalyses), 0, "Should have at least one resource analysis")

	return buf.String()
}

// createDryRunOptions creates DryRunOptions with common settings
func (h *sensitiveTestHelper) createDryRunOptions(cfg *runnertypes.ConfigSpec, hashDir string, showSensitive bool) *resource.DryRunOptions {
	h.t.Helper()

	return &resource.DryRunOptions{
		DetailLevel:         resource.DetailLevelFull,
		OutputFormat:        resource.OutputFormatText,
		ShowSensitive:       showSensitive,
		VerifyFiles:         true,
		VerifyStandardPaths: runnertypes.DetermineVerifyStandardPaths(cfg.Global.VerifyStandardPaths),
		HashDir:             hashDir,
	}
}

// createDryRunOptionsWithDetailLevel creates DryRunOptions with specific DetailLevel
func (h *sensitiveTestHelper) createDryRunOptionsWithDetailLevel(cfg *runnertypes.ConfigSpec, hashDir string, detailLevel resource.DryRunDetailLevel) *resource.DryRunOptions {
	h.t.Helper()

	return &resource.DryRunOptions{
		DetailLevel:         detailLevel,
		OutputFormat:        resource.OutputFormatText,
		ShowSensitive:       false,
		VerifyFiles:         true,
		VerifyStandardPaths: runnertypes.DetermineVerifyStandardPaths(cfg.Global.VerifyStandardPaths),
		HashDir:             hashDir,
	}
}

// TestIntegration_DryRunSensitiveDataMasking tests that sensitive environment
// variables are masked by default in dry-run mode with --dry-run-detail=full
func TestIntegration_DryRunSensitiveDataMasking(t *testing.T) {
	tests := []struct {
		name          string
		showSensitive bool
		expectMasked  bool
	}{
		{
			name:          "default (showSensitive=false) masks sensitive data",
			showSensitive: false,
			expectMasked:  true,
		},
		{
			name:          "showSensitive=true displays sensitive data",
			showSensitive: true,
			expectMasked:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create TOML config with sensitive environment variables
			configContent := `
[global]
env_allowed = ["DB_PASSWORD", "API_TOKEN", "AWS_SECRET_KEY", "NORMAL_VAR"]

[[groups]]
name = "sensitive_test_group"

[[groups.commands]]
name = "test_sensitive_cmd"
cmd = "echo"
args = ["testing sensitive data"]
env_vars = [
    "DB_PASSWORD=super_secret_db_password_123",
    "API_TOKEN=ghp_github_token_abcdefg12345",
    "AWS_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/secretKey",
    "NORMAL_VAR=public_value",
]
risk_level = "medium"
`

			helper := newSensitiveTestHelper(t)
			configPath, hashDir := helper.createConfigAndPaths(configContent)
			cfg, runtimeGlobal, verificationManager := helper.loadConfig(configPath, "test-run-sensitive")

			dryRunOptions := helper.createDryRunOptions(cfg, hashDir, tt.showSensitive)
			output := helper.executeWithCapturedOutput(cfg, runtimeGlobal, verificationManager, dryRunOptions, "test-run-sensitive")

			// Verify PrintFinalEnvironment output
			if tt.expectMasked {
				// Default behavior: sensitive values should be masked
				assert.Contains(t, output, "===== Final Process Environment =====",
					"Should contain final environment header")
				assert.Contains(t, output, "DB_PASSWORD=[REDACTED]",
					"DB_PASSWORD should be masked")
				assert.Contains(t, output, "API_TOKEN=[REDACTED]",
					"API_TOKEN should be masked")
				assert.Contains(t, output, "AWS_SECRET_KEY=[REDACTED]",
					"AWS_SECRET_KEY should be masked")
				assert.Contains(t, output, "NORMAL_VAR=public_value",
					"NORMAL_VAR should not be masked")

				// Sensitive values should NOT appear in plain text
				assert.NotContains(t, output, "super_secret_db_password_123",
					"DB password should not appear in plain text")
				assert.NotContains(t, output, "ghp_github_token_abcdefg12345",
					"GitHub token should not appear in plain text")
				assert.NotContains(t, output, "wJalrXUtnFEMI/K7MDENG/secretKey",
					"AWS secret key should not appear in plain text")
			} else {
				// showSensitive=true: sensitive values should be displayed
				assert.Contains(t, output, "===== Final Process Environment =====",
					"Should contain final environment header")
				assert.Contains(t, output, "super_secret_db_password_123",
					"DB password should appear in plain text")
				assert.Contains(t, output, "ghp_github_token_abcdefg12345",
					"GitHub token should appear in plain text")
				assert.Contains(t, output, "wJalrXUtnFEMI/K7MDENG/secretKey",
					"AWS secret key should appear in plain text")
				assert.Contains(t, output, "NORMAL_VAR=public_value",
					"NORMAL_VAR should appear normally")

				// [REDACTED] should NOT appear
				assert.NotContains(t, output, "[REDACTED]",
					"Values should not be redacted when showSensitive=true")
			}
		})
	}
}

// TestIntegration_DryRunSensitiveDataDefault tests that the default behavior
// (without explicitly setting showSensitive) masks sensitive data
func TestIntegration_DryRunSensitiveDataDefault(t *testing.T) {
	// Create TOML config with sensitive environment variables
	configContent := `
[global]
env_allowed = ["SECRET_API_KEY", "DATABASE_PASSWORD"]

[[groups]]
name = "default_test_group"

[[groups.commands]]
name = "test_default_cmd"
cmd = "echo"
args = ["default test"]
env_vars = [
    "SECRET_API_KEY=my_secret_api_key_12345",
    "DATABASE_PASSWORD=my_db_password_67890",
]
risk_level = "low"
`

	helper := newSensitiveTestHelper(t)
	configPath, hashDir := helper.createConfigAndPaths(configContent)
	cfg, runtimeGlobal, verificationManager := helper.loadConfig(configPath, "test-run-default")

	dryRunOptions := helper.createDryRunOptions(cfg, hashDir, false) // showSensitive defaults to false
	output := helper.executeWithCapturedOutput(cfg, runtimeGlobal, verificationManager, dryRunOptions, "test-run-default")

	// CRITICAL SECURITY TEST: Verify that sensitive data is masked BY DEFAULT
	assert.Contains(t, output, "SECRET_API_KEY=[REDACTED]",
		"SECRET_API_KEY must be masked by default (security requirement)")
	assert.Contains(t, output, "DATABASE_PASSWORD=[REDACTED]",
		"DATABASE_PASSWORD must be masked by default (security requirement)")

	// Verify plain text values do NOT appear
	assert.NotContains(t, output, "my_secret_api_key_12345",
		"Secret API key must not appear in plain text by default")
	assert.NotContains(t, output, "my_db_password_67890",
		"Database password must not appear in plain text by default")
}

// TestIntegration_DryRunDetailLevelWithoutFull tests that PrintFinalEnvironment
// is NOT called when detail level is not Full
func TestIntegration_DryRunDetailLevelWithoutFull(t *testing.T) {
	tests := []struct {
		name        string
		detailLevel resource.DryRunDetailLevel
	}{
		{
			name:        "DetailLevelSummary does not print final environment",
			detailLevel: resource.DetailLevelSummary,
		},
		{
			name:        "DetailLevelDetailed does not print final environment",
			detailLevel: resource.DetailLevelDetailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configContent := `
[global]
env_allowed = ["SECRET_VAR"]

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["test"]
env_vars = ["SECRET_VAR=secret123"]
risk_level = "low"
`
			helper := newSensitiveTestHelper(t)
			configPath, hashDir := helper.createConfigAndPaths(configContent)
			cfg, runtimeGlobal, verificationManager := helper.loadConfig(configPath, "test-run")

			dryRunOptions := helper.createDryRunOptionsWithDetailLevel(cfg, hashDir, tt.detailLevel)
			output := helper.executeWithCapturedOutput(cfg, runtimeGlobal, verificationManager, dryRunOptions, "test-run")

			// PrintFinalEnvironment should NOT be called
			assert.NotContains(t, output, "===== Final Process Environment =====",
				"Final environment should not be printed when detail level is not Full")
			// Neither masked nor plaintext sensitive values should appear
			assert.NotContains(t, output, "SECRET_VAR=[REDACTED]",
				"Masked values should not appear")
			assert.NotContains(t, output, "SECRET_VAR=secret123",
				"Plaintext values should not appear")
		})
	}
}
