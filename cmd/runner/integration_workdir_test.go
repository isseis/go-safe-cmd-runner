package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/executor"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testOutputBuffer captures command output for testing
type testOutputBuffer struct {
	stdout bytes.Buffer
	stderr bytes.Buffer //nolint:unused // Reserved for future use (stderr capture)
	mu     sync.Mutex
}

// Write implements io.Writer interface
func (b *testOutputBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stdout.Write(p)
}

// String returns the captured output as a string
func (b *testOutputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stdout.String()
}

// executorWithOutput wraps an executor to capture command output
//
//nolint:unused // Used in Phase 2+ (createRunnerWithOutputCapture helper function)
type executorWithOutput struct {
	executor.CommandExecutor
	output io.Writer
}

// ExecuteCommand executes a command and captures its output
//
//nolint:unused // Used in Phase 2+ (createRunnerWithOutputCapture helper function)
func (e *executorWithOutput) ExecuteCommand(
	ctx context.Context,
	cmd *runnertypes.RuntimeCommand,
	_ *runnertypes.GroupSpec,
	env map[string]string,
) (int, error) {
	// Create os/exec.Cmd
	execCmd := exec.CommandContext(ctx, cmd.ExpandedCmd, cmd.ExpandedArgs...)
	execCmd.Env = buildEnvSlice(env)
	execCmd.Dir = cmd.EffectiveWorkDir

	// Redirect output to both standard output and capture buffer
	execCmd.Stdout = io.MultiWriter(os.Stdout, e.output)
	execCmd.Stderr = os.Stderr

	// Execute command
	err := execCmd.Run()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return 0, err
		}
	}

	return exitCode, nil
}

// buildEnvSlice converts environment map to slice format
//
//nolint:unused // Used in Phase 2+ (called by executorWithOutput.ExecuteCommand)
func buildEnvSlice(env map[string]string) []string {
	result := make([]string, 0, len(env))
	for k, v := range env {
		result = append(result, k+"="+v)
	}
	return result
}

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
max_risk_level = "medium"
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
max_risk_level = "medium"
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
max_risk_level = "medium"
`,
			expectTempDir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary hash directory for test
			tempHashDir, err := os.MkdirTemp("", "test-hash-*")
			require.NoError(t, err)
			defer os.RemoveAll(tempHashDir)

			// Create temporary config file
			tempConfigFile, err := os.CreateTemp("", "test_config_*.toml")
			require.NoError(t, err)
			defer os.Remove(tempConfigFile.Name())

			_, err = tempConfigFile.WriteString(tt.configContent)
			require.NoError(t, err)
			tempConfigFile.Close()

			// Parse configuration with test verification manager (file validation disabled for dynamic test files)
			verificationManager, err := verification.NewManagerForTest(tempHashDir, verification.WithFileValidatorDisabled())
			require.NoError(t, err)

			cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, tempConfigFile.Name(), "test-run-id")
			require.NoError(t, err)

			// Expand global configuration
			runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
			require.NoError(t, err)

			// Initialize privilege manager
			privMgr := privilege.NewManager(slog.Default())

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
max_risk_level = "medium"
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
	privMgr := privilege.NewManager(slog.Default())

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

// TestOutputCapture verifies that output capture infrastructure works correctly
// This is a temporary test for Phase 1 verification and will be removed after Phase 1 completion
func TestOutputCapture(t *testing.T) {
	buf := &testOutputBuffer{}

	n, err := buf.Write([]byte("test output\n"))
	require.NoError(t, err)
	require.Equal(t, 12, n)

	output := buf.String()
	require.Equal(t, "test output\n", output)
}
