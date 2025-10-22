package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
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

// createRunnerWithOutputCapture creates a Runner with output capture enabled
//
//nolint:unused // Used in Phase 3 (TestIntegration_TempDirHandling enhancement)
func createRunnerWithOutputCapture(
	t *testing.T,
	configContent string,
	keepTempDirs bool,
) (*runner.Runner, *testOutputBuffer) {
	t.Helper()

	// 1. Create temporary config file
	tempConfigFile, err := os.CreateTemp("", "test_config_*.toml")
	require.NoError(t, err)
	defer os.Remove(tempConfigFile.Name())

	_, err = tempConfigFile.WriteString(configContent)
	require.NoError(t, err)
	tempConfigFile.Close()

	// 2. Load configuration
	verificationManager, err := verification.NewManager()
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, tempConfigFile.Name(), "test-run-id")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// 3. Create output buffer
	outputBuf := &testOutputBuffer{}

	// 4. Create executor with output redirect
	baseExec := executor.NewDefaultExecutor()
	exec := &executorWithOutput{
		CommandExecutor: baseExec,
		output:          outputBuf,
	}

	// 5. Initialize privilege manager
	privMgr := privilege.NewManager(slog.Default())

	// 6. Create runner
	runnerOptions := []runner.Option{
		runner.WithVerificationManager(verificationManager),
		runner.WithPrivilegeManager(privMgr),
		runner.WithRunID("test-run-id"),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithKeepTempDirs(keepTempDirs),
		runner.WithExecutor(exec),
	}

	r, err := runner.NewRunner(cfg, runnerOptions...)
	require.NoError(t, err)

	return r, outputBuf
}

// extractWorkdirFromOutput extracts the __runner_workdir path from command output
//
//nolint:unused // Used in Phase 3 (TestIntegration_TempDirHandling enhancement)
func extractWorkdirFromOutput(t *testing.T, output string) string {
	t.Helper()

	// Regular expression pattern: "working in: <path>"
	pattern := regexp.MustCompile(`working in: (.+)`)
	matches := pattern.FindStringSubmatch(output)

	require.Len(t, matches, 2,
		"Failed to extract workdir from output. Expected 'working in: <path>', got: %s",
		output)

	workdirPath := strings.TrimSpace(matches[1])
	require.NotEmpty(t, workdirPath, "Extracted workdir path is empty")

	return workdirPath
}

// validateTempDirBehavior validates temporary directory creation and cleanup behavior
//
//nolint:unused // Used in Phase 3 (TestIntegration_TempDirHandling enhancement)
func validateTempDirBehavior(
	t *testing.T,
	workdirPath string,
	expectTempDir bool,
	keepTempDirs bool,
	afterCleanup bool,
) {
	t.Helper()

	// Case 1: Fixed workdir (expectTempDir=false)
	if !expectTempDir {
		// Verify it's not a temp dir pattern
		assert.NotContains(t, workdirPath, "scr-temp-",
			"Expected fixed workdir, but got temp dir: %s", workdirPath)

		// Fixed workdir should not be deleted
		info, err := os.Stat(workdirPath)
		assert.NoError(t, err, "Fixed workdir should exist: %s", workdirPath)
		assert.True(t, info.IsDir(), "Fixed workdir should be a directory: %s", workdirPath)

		return
	}

	// Case 2: Temp dir (expectTempDir=true)

	// Verify temp dir naming pattern
	assert.Contains(t, workdirPath, "scr-temp-",
		"Expected temp dir pattern 'scr-temp-*', but got: %s", workdirPath)

	// Security check: temp dir should be under system temp dir
	tempRoot := os.TempDir()
	absPath, err := filepath.Abs(workdirPath)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(absPath, tempRoot),
		"Temp dir should be under system temp dir %s, but got: %s", tempRoot, absPath)

	if afterCleanup {
		// Verification after cleanup
		_, err := os.Stat(workdirPath)

		if keepTempDirs {
			// With keepTempDirs=true, directory should exist
			assert.NoError(t, err,
				"Temp dir should exist after cleanup with keepTempDirs=true: %s", workdirPath)

			// Manual cleanup at test end
			t.Cleanup(func() {
				os.RemoveAll(workdirPath)
			})
		} else {
			// With keepTempDirs=false, directory should be deleted
			assert.True(t, os.IsNotExist(err),
				"Temp dir should be deleted after cleanup with keepTempDirs=false, but exists: %s",
				workdirPath)
		}
	} else {
		// Verification before cleanup
		info, err := os.Stat(workdirPath)
		require.NoError(t, err, "Temp dir should exist before cleanup: %s", workdirPath)
		require.True(t, info.IsDir(), "Path should be a directory: %s", workdirPath)

		// Permission verification (Linux/Unix only)
		if runtime.GOOS != "windows" {
			mode := info.Mode()
			assert.Equal(t, os.FileMode(0o700), mode.Perm(),
				"Temp dir permissions should be 0700, got: %o", mode.Perm())
		}
	}
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
