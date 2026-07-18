//go:build test

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTempConfig creates a temporary directory with a config file containing the given content.
// Returns configFile path.
func setupTempConfig(t *testing.T, configContent string) string {
	t.Helper()
	tmpDir := tu.SafeTempDir(t)
	configFile := filepath.Join(tmpDir, "config.toml")
	err := os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)
	return configFile
}

// runDryRunCommand executes the runner in dry-run mode with the given config file and additional arguments.
// Returns the command and its combined output.
func runDryRunCommand(t *testing.T, configFile string, extraArgs ...string) (*exec.Cmd, []byte) {
	t.Helper()
	appArgs := []string{"-config", configFile, "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "text"}
	appArgs = append(appArgs, extraArgs...)
	cmd := newGoRunCmd(t, appArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command output:\n%s", string(output))
	}
	return cmd, output
}

// recordHash saves a hash record for filePath under hashDir using the same
// validator/algorithm the runner uses to verify files.
func recordHash(t *testing.T, hashDir, filePath string) {
	t.Helper()
	validator := filevalidator.NewTestDynLibValidator(t, hashDir)
	_, _, err := validator.SaveRecord(filePath, false)
	require.NoError(t, err)
}

// TestDryRunE2E_HashFilesNotFound tests dry-run with hash files not found
func TestDryRunE2E_HashFilesNotFound(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	configFile := setupTempConfig(t, configContent)
	cmd, output := runDryRunCommand(t, configFile)

	outputStr := string(output)
	// Verify file verification section is present
	assert.Contains(t, outputStr, "===== File Verification =====")
	assert.Contains(t, outputStr, "Hash Directory:")

	// hash_file_not_found is an environment cause (config.toml has no
	// recorded hash), so the preview exits with DryRunExitVerificationUnavailable.
	assert.Equal(t, resource.DryRunExitVerificationUnavailable, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_AllSuccess tests dry-run with all verifications successful
func TestDryRunE2E_AllSuccess(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	configFile := setupTempConfig(t, configContent)
	hashDir := tu.SafeTempDir(t)
	recordHash(t, hashDir, configFile)
	recordHash(t, hashDir, "/bin/echo")

	// Run with summary detail level instead of full
	cmd := newGoRunCmdWithHashDir(t, hashDir, "-config", configFile, "-dry-run", "-dry-run-detail", "summary", "-dry-run-format", "text")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command output:\n%s", string(output))
	}
	require.NoError(t, err, "dry-run should succeed once config.toml's and /bin/echo's hashes are recorded")

	outputStr := string(output)
	// Verify file verification section is present
	assert.Contains(t, outputStr, "===== File Verification =====")
	assert.Contains(t, outputStr, "Verified: 2")
	assert.Contains(t, outputStr, "Failed: 0")

	// Verify exit code is 0
	assert.Equal(t, resource.DryRunExitAllow, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_JSONOutput tests dry-run JSON output with file verification
func TestDryRunE2E_JSONOutput(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	configFile := setupTempConfig(t, configContent)
	hashDir := tu.SafeTempDir(t)
	recordHash(t, hashDir, configFile)
	recordHash(t, hashDir, "/bin/echo")

	// Run command in dry-run mode with JSON output
	cmd := newGoRunCmdWithHashDir(t, hashDir, "-config", configFile, "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "json", "-log-level", "error")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	require.NoError(t, err, "dry-run should succeed once config.toml's and /bin/echo's hashes are recorded")

	// Parse JSON output
	var result struct {
		Status           string                                `json:"status"`
		Phase            string                                `json:"phase"`
		FileVerification *verification.FileVerificationSummary `json:"file_verification,omitempty"`
	}

	err = json.Unmarshal([]byte(stdout.String()), &result)
	require.NoError(t, err, "stdout should be valid JSON")

	// Verify JSON structure
	assert.Equal(t, "success", result.Status)
	assert.Equal(t, "completed", result.Phase)

	// Verify file verification is included
	require.NotNil(t, result.FileVerification, "file_verification should be present in JSON output")
	assert.NotNil(t, result.FileVerification.HashDirStatus)

	// Verify exit code is 0
	assert.Equal(t, resource.DryRunExitAllow, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_MixedResults tests dry-run with mixed verification results
func TestDryRunE2E_MixedResults(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd-1"
cmd = "/bin/echo"
args = ["hello"]

[[groups.commands]]
name = "test-cmd-2"
cmd = "/bin/ls"
args = ["-l"]
`

	configFile := setupTempConfig(t, configContent)

	// Run command in dry-run mode with detailed output
	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run", "-dry-run-detail", "detailed", "-dry-run-format", "text")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command output:\n%s", string(output))
	}

	outputStr := string(output)
	// Verify file verification section is present
	assert.Contains(t, outputStr, "===== File Verification =====")

	// Verify detailed level shows failures if present
	if !strings.Contains(outputStr, "Failed: 0") {
		// If there are failures, they should be shown in detailed mode
		assert.Contains(t, outputStr, "Failures:")
	}

	// config.toml has no recorded hash (hash_file_not_found, an environment
	// cause), so the preview exits with DryRunExitVerificationUnavailable.
	assert.Equal(t, resource.DryRunExitVerificationUnavailable, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_NoSideEffects tests that dry-run with file verification has no side effects
func TestDryRunE2E_NoSideEffects(t *testing.T) {
	// Create temporary config file
	tmpDir := tu.SafeTempDir(t)
	configFile := filepath.Join(tmpDir, "config.toml")
	logDir := filepath.Join(tmpDir, "logs")

	// Create log directory
	err := os.MkdirAll(logDir, 0o755)
	require.NoError(t, err)

	configContent := `
version = "1.0"

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	err = os.WriteFile(configFile, []byte(configContent), 0o644)
	require.NoError(t, err)

	// Capture initial state of temp directory
	entriesBefore, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	// Capture initial state of log directory
	logEntriesBefore, err := os.ReadDir(logDir)
	require.NoError(t, err)

	// Run command in dry-run mode
	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "text", "-log-level", "debug")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command output:\n%s", string(output))
	}
	require.NotEmpty(t, output, "output should not be empty")

	outputStr := string(output)

	// Verify command was not actually executed
	// The output should not contain the command's actual output (which would be "hello" on its own line)
	// However, "hello" will appear in the args field as `args: ["hello"]`, which is expected
	// We verify that the command wasn't executed by checking it's not printed as actual output
	lines := strings.Split(outputStr, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Check for standalone "hello" output (actual command execution)
		// But exclude lines that are part of the parameter display
		if trimmed == "hello" {
			require.FailNow(t, "dry-run should not execute the command (found standalone 'hello' in output)")
		}
	}

	// Verify args are shown in the analysis
	assert.Contains(t, outputStr, `args: ["hello"]`, "dry-run should show command arguments in analysis")

	// Verify no files were created in temp directory (compare before/after)
	entriesAfter, err := os.ReadDir(tmpDir)
	require.NoError(t, err)
	assert.Equal(t, len(entriesBefore), len(entriesAfter), "dry-run should not create any files in temp directory")
	if len(entriesBefore) == len(entriesAfter) {
		for i := range entriesBefore {
			assert.Equal(t, entriesBefore[i].Name(), entriesAfter[i].Name())
		}
	}

	// Verify no log files were created (compare before/after)
	logEntriesAfter, err := os.ReadDir(logDir)
	require.NoError(t, err)
	assert.Equal(t, len(logEntriesBefore), len(logEntriesAfter), "dry-run should not create log files")

	// config.toml has no recorded hash (hash_file_not_found, an environment
	// cause), so the preview exits with DryRunExitVerificationUnavailable.
	// The point of this test is the absence of side effects (checked above),
	// not the exit code itself.
	assert.Equal(t, resource.DryRunExitVerificationUnavailable, cmd.ProcessState.ExitCode())
}

// TestDryRunE2E_RemovedFlagRejected verifies that passing the removed
// -dry-run-fail-unverified flag causes the runner to reject it as an
// undefined flag and exit non-zero.
func TestDryRunE2E_RemovedFlagRejected(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/echo"
args = ["hello"]
`

	configFile := setupTempConfig(t, configContent)
	cmd := newGoRunCmd(t, "-config", configFile, "-dry-run", "-dry-run-fail-unverified")
	output, err := cmd.CombinedOutput()

	// The runner must reject the undefined flag.
	assert.Error(t, err, "runner should fail with removed flag")
	exitErr, ok := err.(*exec.ExitError)
	require.True(t, ok, "error should be ExitError")
	assert.NotEqual(t, 0, exitErr.ExitCode(), "exit code should be non-zero")
	assert.Contains(t, string(output), "flag provided but not defined",
		"output should contain Go's undefined-flag error message")
}

// TestDryRunE2E_VerifyFilesHashMismatch verifies that a verify_files
// hash_mismatch causes the dry-run preview to exit with
// DryRunExitPolicyDeny (1), while the config file itself is correctly
// verified so the exit code is solely attributed to verify_files.
func TestDryRunE2E_VerifyFilesHashMismatch(t *testing.T) {
	hashDir := tu.SafeTempDir(t)

	// Create a target file and record its initial hash.
	targetFile := filepath.Join(tu.SafeTempDir(t), "verify_target.txt")
	err := os.WriteFile(targetFile, []byte("original content"), 0o644)
	require.NoError(t, err)
	recordHash(t, hashDir, targetFile)

	// Tamper with the target file so its content no longer matches the
	// recorded hash.
	err = os.WriteFile(targetFile, []byte("tampered content"), 0o644)
	require.NoError(t, err)

	// Use /bin/true as the group command rather than /bin/echo. On some
	// environments /bin/echo (and other coreutils) are symlinks straight
	// into a Rust-based coreutils install (e.g.
	// /usr/lib/cargo/bin/coreutils/echo) that isn't present on every CI
	// runner. /bin/true stays low-risk like echo but, where such a
	// transitional coreutils setup exists, resolves to a local sibling
	// (e.g. /bin/gnutrue) instead of reaching outside /bin.
	const cmdPath = "/bin/true"

	configContent := `
version = "1.0"

[global]
verify_files = ["` + targetFile + `"]

[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "` + cmdPath + `"
args = ["hello"]
`
	configFile := setupTempConfig(t, configContent)

	// Record the config file's hash so it is correctly verified. Also
	// record the command binary's hash to avoid an unrelated
	// uncertain_unverified_identity from polluting the exit code.
	// Only verify_files' hash_mismatch should push the code to deny.
	recordHash(t, hashDir, configFile)
	recordHash(t, hashDir, cmdPath)

	cmd := newGoRunCmdWithHashDir(t, hashDir, "-config", configFile, "-dry-run", "-dry-run-detail", "full", "-dry-run-format", "text")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Command output:\n%s", string(output))
	}

	outputStr := string(output)
	// Verify the tampered file appears in the output with a hash-mismatch
	// indication. The formatter uses the human-readable "Hash mismatch"
	// label, not the raw FailureReason enum name.
	assert.Contains(t, outputStr, targetFile, "output should reference the tampered file")
	assert.Contains(t, outputStr, "Hash mismatch", "output should indicate hash mismatch")

	// Confirm exactly one verification failure occurred (the tampered
	// verify_files entry) and no other files failed verification, so the
	// deny exit code below is unambiguously attributable to it.
	assert.Contains(t, outputStr, "Failed: 1", "exactly one file should fail verification")

	// The exit code must be DryRunExitPolicyDeny (1) -- tampering signal from
	// verify_files.
	assert.Equal(t, resource.DryRunExitPolicyDeny, cmd.ProcessState.ExitCode(),
		"verify_files hash_mismatch must cause exit 1")
}

// TestDryRunE2E_HashDirectoryNotFound tests that when the hash directory
// does not exist, dry-run:
//   - does not create the hash directory as a side effect,
//   - reports the failure faithfully as ReasonHashDirNotFound rather than
//     downgrading it to hash_file_not_found,
//   - records adopted content with the verify_failed_hash_directory_not_found
//     unverified reason instead of skipped_no_validator,
//   - exits with DryRunExitVerificationUnavailable (3).
func TestDryRunE2E_HashDirectoryNotFound(t *testing.T) {
	configContent := `
[[groups]]
name = "test_group"

[[groups.commands]]
name = "test-cmd"
cmd = "/bin/true"
args = ["hello"]
`

	configFile := setupTempConfig(t, configContent)

	// Pick a non-existent path under a fresh temp directory so the
	// runner never sees an existing hash directory.
	hashDir := filepath.Join(tu.SafeTempDir(t), "does-not-exist")

	cmd := newGoRunCmdWithHashDir(t, hashDir,
		"-config", configFile,
		"-dry-run",
		"-dry-run-detail", "full",
		"-dry-run-format", "json",
		"-log-level", "error")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// The dry-run preview is expected to exit with a non-zero status
	// (DryRunExitVerificationUnavailable). The command therefore returns
	// an *exec.ExitError; require that specifically so a failure to even
	// start the process (which would leave ProcessState nil) fails the test
	// gracefully instead of panicking on ExitCode() below.
	if err := cmd.Run(); err != nil {
		t.Logf("Command stderr:\n%s", stderr.String())
		t.Logf("Command exit error: %v", err)
		var exitErr *exec.ExitError
		require.ErrorAs(t, err, &exitErr,
			"command must exit with a non-zero status, not fail to start")
	}

	require.NotNil(t, cmd.ProcessState, "ProcessState must not be nil")
	assert.Equal(t, resource.DryRunExitVerificationUnavailable, cmd.ProcessState.ExitCode(),
		"missing hash directory must produce exit code 3")

	var result struct {
		Status           string                                `json:"status"`
		Phase            string                                `json:"phase"`
		FileVerification *verification.FileVerificationSummary `json:"file_verification,omitempty"`
	}
	err := json.Unmarshal([]byte(stdout.String()), &result)
	require.NoError(t, err, "stdout should be valid JSON")

	require.NotNil(t, result.FileVerification, "file_verification should be present in JSON output")
	assert.False(t, result.FileVerification.HashDirStatus.Exists,
		"HashDirStatus.Exists should be false when the hash directory is missing")

	require.NotEmpty(t, result.FileVerification.Failures,
		"at least one failure should be reported for the missing hash directory")
	foundDirNotFound := false
	for _, failure := range result.FileVerification.Failures {
		if failure.Reason == verification.ReasonHashDirNotFound {
			foundDirNotFound = true
			break
		}
	}
	assert.True(t, foundDirNotFound,
		"Failures should include a ReasonHashDirNotFound entry")

	require.NotEmpty(t, result.FileVerification.UnverifiedFiles,
		"at least one unverified file should be recorded when the hash directory is missing")
	expectedUnverifiedReason := string(verification.UnverifiedReasonFromFailure(verification.ReasonHashDirNotFound))
	foundVerifyFailedDirNotFound := false
	for _, u := range result.FileVerification.UnverifiedFiles {
		if u.Reason == expectedUnverifiedReason {
			foundVerifyFailedDirNotFound = true
			break
		}
	}
	assert.True(t, foundVerifyFailedDirNotFound,
		"UnverifiedFiles should include a verify_failed_hash_directory_not_found entry")

	// The hash directory must not have been created as a side effect of
	// the dry-run preview.
	_, statErr := os.Stat(hashDir)
	assert.True(t, os.IsNotExist(statErr),
		"dry-run must not create the hash directory")
}
