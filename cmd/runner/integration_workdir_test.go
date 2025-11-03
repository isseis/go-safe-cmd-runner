package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
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
	mu     sync.RWMutex
}

// Write implements io.Writer interface
func (b *testOutputBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stdout.Write(p)
}

// String returns the captured output as a string
func (b *testOutputBuffer) String() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.stdout.String()
}

// executorWithOutput wraps an executor to capture command output
type executorWithOutput struct {
	baseExecutor executor.CommandExecutor
	output       io.Writer
}

// Execute executes a command and captures its output
func (e *executorWithOutput) Execute(
	ctx context.Context,
	cmd *runnertypes.RuntimeCommand,
	env map[string]string,
	outputWriter executor.OutputWriter,
) (*executor.Result, error) {
	// Create a custom output writer that writes to both the provided writer and our capture buffer
	captureWriter := &captureOutputWriter{
		wrapped: outputWriter,
		capture: e.output,
	}

	// Use the base executor with our custom output writer
	return e.baseExecutor.Execute(ctx, cmd, env, captureWriter)
}

// Validate validates a command without executing it
func (e *executorWithOutput) Validate(cmd *runnertypes.RuntimeCommand) error {
	return e.baseExecutor.Validate(cmd)
}

// captureOutputWriter wraps an OutputWriter to also capture output
type captureOutputWriter struct {
	wrapped executor.OutputWriter
	capture io.Writer
	mu      sync.Mutex
}

// Write writes data to both the wrapped writer and capture buffer
func (w *captureOutputWriter) Write(stream string, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Write to capture buffer (only stdout for simplicity)
	if stream == executor.StdoutStream {
		_, _ = w.capture.Write(data)
	}

	// Write to wrapped writer if provided
	if w.wrapped != nil {
		return w.wrapped.Write(stream, data)
	}

	return nil
}

// Close closes the wrapped writer if it implements io.Closer
func (w *captureOutputWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.wrapped != nil {
		return w.wrapped.Close()
	}
	return nil
}

// createRunnerWithOutputCapture creates a Runner with output capture enabled
func createRunnerWithOutputCapture(
	t *testing.T,
	configContent string,
	keepTempDirs bool,
) (*runner.Runner, *testOutputBuffer) {
	t.Helper()

	// 1. Create temporary hash directory for test
	tempHashDir, err := os.MkdirTemp("", "test-hash-*")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.RemoveAll(tempHashDir)
	})

	// 2. Create temporary config file using helper
	configPath := setupTestConfig(t, configContent)

	// 3. Load configuration with test verification manager (file validation disabled for dynamic test files)
	verificationManager, err := verification.NewManagerForTest(tempHashDir, verification.WithFileValidatorDisabled())
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-id")
	require.NoError(t, err)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// 4. Create output buffer
	outputBuf := &testOutputBuffer{}

	// 5. Create executor with output redirect
	baseExec := executor.NewDefaultExecutor()
	exec := &executorWithOutput{
		baseExecutor: baseExec,
		output:       outputBuf,
	}

	// 6. Initialize privilege manager
	privMgr := privilege.NewManager(slog.Default())

	// 7. Create runner
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

// setupTestConfig creates a temporary config file with the given content
func setupTestConfig(t *testing.T, configContent string) string {
	t.Helper()

	tempConfigFile, err := os.CreateTemp("", "test_config_*.toml")
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Remove(tempConfigFile.Name())
	})

	_, err = tempConfigFile.WriteString(configContent)
	require.NoError(t, err)
	tempConfigFile.Close()

	return tempConfigFile.Name()
}

// executeRunnerWithTimeout executes a runner with LoadSystemEnvironment and ExecuteAll
//
//nolint:unparam // timeout parameter kept for test flexibility even though currently always receives same value
func executeRunnerWithTimeout(t *testing.T, r *runner.Runner, timeout time.Duration) {
	t.Helper()

	err := r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = r.ExecuteAll(ctx)
	require.NoError(t, err)
}

// extractWorkdirFromOutput extracts the __runner_workdir path from command output
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

// extractPathsFromOutput extracts all paths (e.g., file system paths) from output lines using a regular expression pattern.
// The provided pattern must contain at least one capture group; the function extracts the last capture group from each matching line.
func extractPathsFromOutput(t *testing.T, output string, pattern *regexp.Regexp) []string {
	t.Helper()

	lines := strings.Split(output, "\n")
	var paths []string

	for _, line := range lines {
		matches := pattern.FindStringSubmatch(line)
		if len(matches) >= 2 {
			// Take the last capture group as the path (matches[len(matches)-1])
			path := strings.TrimSpace(matches[len(matches)-1])
			if path != "" {
				paths = append(paths, path)
			}
		}
	}

	return paths
}

// assertUniquePaths verifies that all paths in the slice are unique
func assertUniquePaths(t *testing.T, paths []string, context string) {
	t.Helper()

	uniquePaths := make(map[string]struct{})
	for _, path := range paths {
		uniquePaths[path] = struct{}{}
	}

	assert.Len(t, uniquePaths, len(paths),
		"%s: Expected %d unique paths, but got duplicates: %v",
		context, len(paths), paths)
}

// assertAllPathsAreTempDirs verifies all paths match temp directory pattern
func assertAllPathsAreTempDirs(t *testing.T, paths []string, context string) {
	t.Helper()

	for i, path := range paths {
		assert.True(t, isTempDirPattern(path),
			"%s: Expected temp dir pattern for path %d, but got: %s",
			context, i+1, path)
	}
}

// assertAllPathsExist verifies all paths exist as directories
func assertAllPathsExist(t *testing.T, paths []string, context string) {
	t.Helper()

	for i, path := range paths {
		info, err := os.Stat(path)
		require.NoError(t, err,
			"%s: Path %d should exist: %s", context, i+1, path)
		assert.True(t, info.IsDir(),
			"%s: Path %d should be a directory: %s", context, i+1, path)
	}
}

// registerPathsForCleanup registers paths for cleanup at test end
func registerPathsForCleanup(t *testing.T, paths []string) {
	t.Helper()

	for _, path := range paths {
		p := path // capture loop variable
		t.Cleanup(func() {
			os.RemoveAll(p)
		})
	}
}

// isTempDirPattern checks if a path matches the temporary directory pattern (scr-*-*)
func isTempDirPattern(path string) bool {
	baseName := filepath.Base(path)
	hasScrPrefix := strings.HasPrefix(baseName, "scr-")
	hasMultipleDashes := strings.Count(baseName, "-") >= 2
	return hasScrPrefix && hasMultipleDashes
}

// validateTempDirBehavior validates temporary directory creation and cleanup behavior
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
		// Fixed workdir should not match temp dir pattern (scr-*-*)
		assert.False(t, isTempDirPattern(workdirPath),
			"Expected fixed workdir, but got temp dir: %s", workdirPath)

		// Fixed workdir should not be deleted
		info, err := os.Stat(workdirPath)
		assert.NoError(t, err, "Fixed workdir should exist: %s", workdirPath)
		assert.True(t, info.IsDir(), "Fixed workdir should be a directory: %s", workdirPath)

		return
	}

	// Case 2: Temp dir (expectTempDir=true)

	// Verify temp dir naming pattern (matches 'scr-*-*' pattern)
	assert.True(t, isTempDirPattern(workdirPath),
		"Expected temp dir pattern 'scr-*-*', but got: %s", workdirPath)

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
		// Verification before CleanupAllResources()
		// Note: With keepTempDirs=false, temp dirs are auto-deleted after group execution,
		// so they may not exist at this point.
		info, err := os.Stat(workdirPath)

		if keepTempDirs {
			// With keepTempDirs=true, directory should still exist
			require.NoError(t, err, "Temp dir should exist before cleanup with keepTempDirs=true: %s", workdirPath)
			require.True(t, info.IsDir(), "Path should be a directory: %s", workdirPath)

			// Permission verification (Linux/Unix only)
			if runtime.GOOS != "windows" {
				mode := info.Mode()
				assert.Equal(t, os.FileMode(0o700), mode.Perm(),
					"Temp dir permissions should be 0700, got: %o", mode.Perm())
			}
		} else {
			// With keepTempDirs=false, directory is auto-deleted after group execution
			// So it's expected to not exist here
			assert.True(t, os.IsNotExist(err),
				"Temp dir should be auto-deleted after ExecuteAll with keepTempDirs=false, but exists: %s",
				workdirPath)
		}
	}
}

// TestIntegration_TempDirHandling tests temporary directory handling
func TestIntegration_TempDirHandling(t *testing.T) {
	tests := []struct {
		name             string
		keepTempDirs     bool
		configContent    string
		expectTempDir    bool
		usesFixedWorkdir bool
	}{
		{
			name:             "Auto temp dir without keep flag",
			keepTempDirs:     false,
			usesFixedWorkdir: false,
			configContent: `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
risk_level = "medium"
`,
			expectTempDir: true,
		},
		{
			name:             "Auto temp dir with keep flag",
			keepTempDirs:     true,
			usesFixedWorkdir: false,
			configContent: `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
risk_level = "medium"
`,
			expectTempDir: true,
		},
		{
			name:             "Fixed workdir",
			keepTempDirs:     false,
			usesFixedWorkdir: true,
			// configContent is dynamically generated for this test case (with fixed workdir)
			configContent: "",
			expectTempDir: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TC-003: Create fixed workdir if needed
			var fixedWorkdir string
			configContent := tt.configContent
			if tt.usesFixedWorkdir {
				var err error
				fixedWorkdir, err = os.MkdirTemp("", "test-fixed-workdir-*")
				require.NoError(t, err)
				defer os.RemoveAll(fixedWorkdir)

				// Escape path for TOML string (Windows compatibility: backslashes must be escaped)
				escapedPath := strings.ReplaceAll(fixedWorkdir, `\`, `\\`)

				// Generate configContent dynamically with fixed workdir path
				configContent = `
[[groups]]
name = "test_group"
workdir = "` + escapedPath + `"

[[groups.commands]]
name = "test_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
risk_level = "medium"
`
			}

			// 1. Create Runner with output capture enabled
			r, outputBuf := createRunnerWithOutputCapture(t, configContent, tt.keepTempDirs)

			// 2. Execute all groups with timeout
			executeRunnerWithTimeout(t, r, 30*time.Second)

			// 3. Extract __runner_workdir value from command output
			output := outputBuf.String()
			workdirPath := extractWorkdirFromOutput(t, output)

			// 4. TC-003: Verify that fixed workdir is used
			if tt.usesFixedWorkdir {
				assert.Equal(t, fixedWorkdir, workdirPath,
					"Expected fixed workdir to be used: %s, got: %s", fixedWorkdir, workdirPath)
			}

			// 5. Validate temp dir behavior before cleanup
			validateTempDirBehavior(t, workdirPath, tt.expectTempDir, tt.keepTempDirs, false)

			// 6. Cleanup all resources
			err := r.CleanupAllResources()
			require.NoError(t, err)

			// 7. Validate temp dir behavior after cleanup
			validateTempDirBehavior(t, workdirPath, tt.expectTempDir, tt.keepTempDirs, true)
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
risk_level = "medium"
`

	// Create temporary config file using helper
	configPath := setupTestConfig(t, configContent)

	// Parse configuration
	verificationManager, err := verification.NewManagerForDryRun()
	require.NoError(t, err)

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-id")
	require.NoError(t, err)

	// Expand global configuration
	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	require.NoError(t, err)

	// Initialize privilege manager
	privMgr := privilege.NewManager(slog.Default())

	// Setup dry-run options
	hashDir := t.TempDir()

	dryRunOptions := &resource.DryRunOptions{
		DetailLevel:         resource.DetailLevelDetailed,
		OutputFormat:        resource.OutputFormatText,
		ShowSensitive:       false,
		VerifyFiles:         true,
		VerifyStandardPaths: runnertypes.DetermineVerifyStandardPaths(cfg.Global.VerifyStandardPaths),
		HashDir:             hashDir,
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

	// Execute all groups in dry-run mode with timeout
	executeRunnerWithTimeout(t, r, 30*time.Second)

	// Get dry-run results
	result := r.GetDryRunResults()
	require.NotNil(t, result)

	// Verify that the result contains resource analyses
	require.Greater(t, len(result.ResourceAnalyses), 0, "Expected at least one resource analysis")

	// Find the echo command analysis
	var cmdAnalysis *resource.ResourceAnalysis
	for i := range result.ResourceAnalyses {
		analysis := &result.ResourceAnalyses[i]
		if analysis.Type == resource.ResourceTypeCommand &&
			analysis.Operation == resource.OperationExecute &&
			(analysis.Target == "echo" || analysis.Target == "/bin/echo") {
			cmdAnalysis = analysis
			break
		}
	}
	require.NotNil(t, cmdAnalysis, "Expected to find analysis for echo command")

	// Verify that working_directory parameter exists and contains virtual temp dir path
	workDir, ok := cmdAnalysis.Parameters["working_directory"]
	require.True(t, ok, "Expected working_directory parameter in command analysis")
	workDirStr, ok := workDir.Value().(string)
	require.True(t, ok, "Expected working_directory value to be a string")

	// Verify virtual temp dir path pattern
	assert.Contains(t, workDirStr, "/tmp/scr-", "Expected virtual temp dir path to start with /tmp/scr-")
	assert.Contains(t, workDirStr, "test_group", "Expected group name in virtual temp dir path")

	// Verify that the virtual temp dir does NOT exist on the filesystem
	_, err = os.Stat(workDirStr)
	assert.True(t, os.IsNotExist(err),
		"Virtual temp dir should not exist on filesystem: %s", workDirStr)

	// Verify group parameter
	groupName, ok := cmdAnalysis.Parameters["group"]
	require.True(t, ok, "Expected group parameter in command analysis")
	assert.Equal(t, "test_group", groupName.Value(), "Expected group name to be 'test_group'")

	// Cleanup
	err = r.CleanupAllResources()
	require.NoError(t, err)
}

// TestIntegration_ErrorCleanup tests that temporary directories are cleaned up even when errors occur
func TestIntegration_ErrorCleanup(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "failing_cmd"
cmd = "echo"
args = ["working in: %{__runner_workdir}"]
risk_level = "medium"

[[groups.commands]]
name = "invalid_cmd"
cmd = "/nonexistent/invalid/command/path"
args = []
risk_level = "medium"
`

	// 1. Create Runner with output capture enabled (keepTempDirs=false for auto cleanup)
	r, outputBuf := createRunnerWithOutputCapture(t, configContent, false)

	// 2. Execute all groups - expecting error from invalid_cmd
	err := r.LoadSystemEnvironment()
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// ExecuteAll should fail due to invalid command
	err = r.ExecuteAll(ctx)
	require.Error(t, err, "Expected error from invalid command")

	// 3. Extract __runner_workdir value from command output (from the first successful command)
	output := outputBuf.String()
	workdirPath := extractWorkdirFromOutput(t, output)

	// 4. Verify temp dir pattern
	assert.True(t, isTempDirPattern(workdirPath),
		"Expected temp dir pattern 'scr-*-*', but got: %s", workdirPath)

	// 5. Verify temp dir is deleted even after error (auto-deleted after group execution)
	_, statErr := os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(statErr),
		"Temp dir should be auto-deleted even after error with keepTempDirs=false, but exists: %s",
		workdirPath)

	// 6. Cleanup all resources
	cleanupErr := r.CleanupAllResources()
	require.NoError(t, cleanupErr)

	// 7. Verify temp dir remains deleted after cleanup
	_, statErr = os.Stat(workdirPath)
	assert.True(t, os.IsNotExist(statErr),
		"Temp dir should remain deleted after cleanup: %s", workdirPath)
}

// TestIntegration_MultipleGroups tests that each group gets independent temporary directories
func TestIntegration_MultipleGroups(t *testing.T) {
	configContent := `
[[groups]]
name = "group1"

[[groups.commands]]
name = "cmd1"
cmd = "echo"
args = ["group1: %{__runner_workdir}"]
risk_level = "medium"

[[groups]]
name = "group2"

[[groups.commands]]
name = "cmd2"
cmd = "echo"
args = ["group2: %{__runner_workdir}"]
risk_level = "medium"

[[groups]]
name = "group3"

[[groups.commands]]
name = "cmd3"
cmd = "echo"
args = ["group3: %{__runner_workdir}"]
risk_level = "medium"
`

	// 1. Create Runner with output capture enabled (keepTempDirs=true to verify independence)
	r, outputBuf := createRunnerWithOutputCapture(t, configContent, true)

	// 2. Execute all groups
	executeRunnerWithTimeout(t, r, 30*time.Second)

	// 3. Parse output and extract workdir paths for each group using common helper
	output := outputBuf.String()
	groupPattern := regexp.MustCompile(`(group\d+): (.+)`)
	workdirPaths := extractPathsFromOutput(t, output, groupPattern)

	// 4. Verify we found exactly 3 workdir paths (one per group)
	require.Len(t, workdirPaths, 3, "Expected 3 workdir paths from 3 groups")

	// 5. Verify all paths are temporary directories
	assertAllPathsAreTempDirs(t, workdirPaths, "MultipleGroups")

	// 6. Verify all paths are unique (independent temp dirs for each group)
	assertUniquePaths(t, workdirPaths, "MultipleGroups")

	// 7. Verify all temp dirs exist (because keepTempDirs=true)
	assertAllPathsExist(t, workdirPaths, "MultipleGroups (before cleanup)")

	// 8. Cleanup all resources
	err := r.CleanupAllResources()
	require.NoError(t, err)

	// 9. Verify all temp dirs still exist after cleanup (because keepTempDirs=true)
	assertAllPathsExist(t, workdirPaths, "MultipleGroups (after cleanup)")

	// Register paths for cleanup at test end
	registerPathsForCleanup(t, workdirPaths)
}
