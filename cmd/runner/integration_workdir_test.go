package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIntegration_TempDirHandling tests temporary directory handling
func TestIntegration_TempDirHandling(t *testing.T) {
	tests := []struct {
		name          string
		keepTempDirs  bool
		configContent string
		expectTempDir bool
	}{
		{
			name:         "Auto temp dir without keep flag",
			keepTempDirs: false,
			configContent: `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
`,
			expectTempDir: true,
		},
		{
			name:         "Auto temp dir with keep flag",
			keepTempDirs: true,
			configContent: `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
`,
			expectTempDir: true,
		},
		{
			name:         "Fixed workdir",
			keepTempDirs: false,
			configContent: `
[[groups]]
name = "test_group"
workdir = "/tmp"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
`,
			expectTempDir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary config file
			tempConfigFile, err := os.CreateTemp("", "test_config_*.toml")
			require.NoError(t, err)
			defer os.Remove(tempConfigFile.Name())

			_, err = tempConfigFile.WriteString(tt.configContent)
			require.NoError(t, err)
			tempConfigFile.Close()

			// Parse configuration
			verificationManager, err := verification.NewManager()
			require.NoError(t, err)

			cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, tempConfigFile.Name(), "test-run-id")
			require.NoError(t, err)

			// Expand global configuration
			runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
			require.NoError(t, err)

			// Initialize privilege manager
			privMgr := privilege.NewManager(nil)

			// Create runner with keepTempDirs option
			runnerOptions := []runner.Option{
				runner.WithVerificationManager(verificationManager),
				runner.WithPrivilegeManager(privMgr),
				runner.WithRunID("test-run-id"),
				runner.WithRuntimeGlobal(runtimeGlobal),
				runner.WithKeepTempDirs(tt.keepTempDirs),
			}

			r, err := runner.NewRunner(cfg, runnerOptions...)
			require.NoError(t, err)

			// Load system environment
			err = r.LoadSystemEnvironment()
			require.NoError(t, err)

			// Execute all groups
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			err = r.ExecuteAll(ctx)
			require.NoError(t, err)

			// Cleanup
			err = r.CleanupAllResources()
			require.NoError(t, err)

			// For this test, we primarily verify that the execution completes successfully
			// The actual temp directory behavior is tested at the unit level
		})
	}
}

// TestIntegration_DryRunWithTempDir tests dry-run mode with temporary directories
func TestIntegration_DryRunWithTempDir(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
`

	// Create temporary config file
	tempConfigFile, err := os.CreateTemp("", "test_config_*.toml")
	require.NoError(t, err)
	defer os.Remove(tempConfigFile.Name())

	_, err = tempConfigFile.WriteString(configContent)
	require.NoError(t, err)
	tempConfigFile.Close()

	// Parse configuration
	verificationManager, err := verification.NewManagerForDryRun()
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, tempConfigFile.Name(), "test-run-id")
	require.NoError(t, err)

	// Expand global configuration
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// Initialize privilege manager
	privMgr := privilege.NewManager(nil)

	// Setup dry-run options
	dryRunOptions := &resource.DryRunOptions{
		DetailLevel:       resource.DetailLevelDetailed,
		OutputFormat:      resource.OutputFormatText,
		ShowSensitive:     false,
		VerifyFiles:       true,
		SkipStandardPaths: cfg.Global.SkipStandardPaths,
		HashDir:           "/tmp/scr-hash",
	}

	// Create runner with dry-run option
	runnerOptions := []runner.Option{
		runner.WithVerificationManager(verificationManager),
		runner.WithPrivilegeManager(privMgr),
		runner.WithRunID("test-run-id"),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithKeepTempDirs(false),
		runner.WithDryRun(dryRunOptions),
	}

	r, err := runner.NewRunner(cfg, runnerOptions...)
	require.NoError(t, err)

	// Load system environment
	err = r.LoadSystemEnvironment()
	require.NoError(t, err)

	// Execute all groups in dry-run mode
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = r.ExecuteAll(ctx)
	require.NoError(t, err)

	// Get dry-run results
	result := r.GetDryRunResults()
	require.NotNil(t, result)

	// Verify that the result contains resource analyses
	assert.Greater(t, len(result.ResourceAnalyses), 0)

	// Cleanup
	err = r.CleanupAllResources()
	require.NoError(t, err)
}
