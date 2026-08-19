package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
	tu "github.com/isseis/go-safe-cmd-runner/internal/testutil"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestFlags initializes the command-line flags for testing and returns a cleanup function
func setupTestFlags() func() {
	// Save original command line arguments and flag.CommandLine
	oldArgs := os.Args
	oldCommandLine := flag.CommandLine

	// Create new flag set with ExitOnError handling
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// Initialize all flags - must match init() in main.go
	// High-priority flags with short forms
	flag.StringVar(&configPath, "config", "", "path to config file")
	flag.StringVar(&configPath, "c", "", "path to config file (short form)")

	flag.BoolVar(&dryRun, "dry-run", false, "print commands without executing them")
	flag.BoolVar(&dryRun, "n", false, "print commands without executing them (short form)")

	flag.StringVar(&groups, "groups", "", "comma-separated list of groups to execute (executes all groups if not specified)\nExample: --groups=build,test")
	flag.StringVar(&groups, "g", "", "comma-separated list of groups to execute (short form)")

	// Medium-priority flags with short forms
	flag.StringVar(&logLevel, "log-level", "info", "log level (debug, info, warn, error)")
	flag.StringVar(&logLevel, "l", "info", "log level (short form)")

	flag.BoolVar(&forceQuiet, "quiet", false, "force non-interactive mode (disables colored output)")
	flag.BoolVar(&forceQuiet, "q", false, "force non-interactive mode (short form)")

	// Other flags without short forms
	flag.StringVar(&logDir, "log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	flag.StringVar(&dryRunFormat, "dry-run-format", "text", "dry-run output format (text, json)")
	flag.StringVar(&dryRunDetail, "dry-run-detail", "detailed", "dry-run detail level (summary, detailed, full)")
	flag.BoolVar(&showSensitive, "show-sensitive", false, "show sensitive information in dry-run output (use with caution)")
	flag.StringVar(&runID, "run-id", "", "unique identifier for this execution run ("+logging.RunIDFormatDescription()+"; auto-generates ULID if not provided)")
	flag.BoolVar(&forceInteractive, "interactive", false, "force interactive mode with colored output (overrides environment detection)")
	flag.BoolVar(&keepTempDirs, "keep-temp-dirs", false, "keep temporary directories after execution")

	// Return cleanup function to restore original state
	return func() {
		os.Args = oldArgs
		flag.CommandLine = oldCommandLine
	}
}

// createTempHashDir creates a temporary directory for hash storage during testing
func createTempHashDir(t *testing.T) (string, func()) {
	t.Helper()
	tempDir, err := os.MkdirTemp("", "go-safe-cmd-runner-test-")
	require.NoError(t, err, "failed to create temp dir")

	cleanup := func() {
		_ = os.RemoveAll(tempDir) // Ignore cleanup errors in test helper
	}

	return tempDir, cleanup
}

// runForTestWithTempHashDir is a version that uses a temporary hash directory
func runForTestWithTempHashDir(t *testing.T, runID string) error {
	t.Helper()

	// Create temporary hash directory
	tempHashDir, cleanup := createTempHashDir(t)
	defer cleanup()

	// Set up context with cancellation
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize verification manager with temporary hash directory
	verificationManager, err := verification.NewManagerForTest(tempHashDir)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   "Verification manager initialization failed",
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	// Load and prepare configuration (verify, parse, and expand variables)
	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, runID)
	if err != nil {
		return err
	}

	// For testing, we skip the actual execution steps
	_ = ctx
	_ = cfg

	return nil
}

// runForTestWithManagerUsingTempDir is a helper that uses temporary hash directory
func runForTestWithManagerUsingTempDir(t *testing.T) (error, error) {
	t.Helper()

	// Create temporary hash directory
	tempHashDir, cleanup := createTempHashDir(t)
	defer cleanup()

	// Test manager creation directly with temp directory
	_, err := verification.NewManagerForTest(tempHashDir)
	if err != nil {
		return nil, err
	}

	// Test the full runForTestWithTempHashDir flow
	return runForTestWithTempHashDir(t, "test-run-id"), nil
}

// runForTestWithCustomHashDir is a helper for testing custom hash directories
func runForTestWithCustomHashDir(t *testing.T, hashDir string) (error, error) {
	t.Helper()

	// Test manager creation with custom hash directory
	verificationManager, err := verification.NewManagerForTest(hashDir)
	if err != nil {
		return nil, err
	}

	// Try to load and prepare config (will fail without config file, but tests manager creation)
	_, configErr := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, "test-run-id")
	return configErr, nil
}

func TestConfigPathRequired(t *testing.T) {
	// Setup test flags
	cleanup := setupTestFlags()
	defer cleanup()

	// Test args without --config (hash directory is now set automatically to default)
	os.Args = []string{"runner"}

	// Parse flags
	flag.Parse()

	// Test runForTestWithTempHashDir() function to avoid CI hash directory issues
	runID := "test-run-id"
	err := runForTestWithTempHashDir(t, runID)
	assert.Error(t, err, "expected error when --config is not provided")

	// Check if the error is a PreExecutionError with the correct type
	var preExecErr *logging.PreExecutionError
	require.ErrorAs(t, err, &preExecErr, "expected PreExecutionError, got: %T (error: %v)", err, err)

	assert.Equal(t, logging.ErrorTypeRequiredArgumentMissing, preExecErr.Type)
}

func TestNewManagerProduction(t *testing.T) {
	t.Run("creates manager with default hash directory", func(t *testing.T) {
		// Use temporary hash directory to avoid CI environment issues
		runErr, managerErr := runForTestWithManagerUsingTempDir(t)
		require.NoError(t, managerErr, "manager creation should not fail")
		if runErr != nil {
			// In tests, we expect this to fail due to missing config file
			assert.Contains(t, runErr.Error(), "config")
		}
	})
}

// TestNewManagerForTestValidation tests the testing API validation
func TestNewManagerForTestValidation(t *testing.T) {
	t.Run("valid custom hash directory", func(t *testing.T) {
		// Create temporary directory for testing
		tempDir := tu.SafeTempDir(t)

		// This should work since we're in a test file
		configErr, managerErr := runForTestWithCustomHashDir(t, tempDir)
		require.NoError(t, managerErr, "manager creation should not fail")
		if configErr != nil {
			// We expect config errors, not manager creation errors
			assert.Contains(t, configErr.Error(), "config")
		}
	})

	t.Run("relative path allowed in testing", func(t *testing.T) {
		// Custom hash directories (even relative ones) are allowed in testing mode
		configErr, managerErr := runForTestWithCustomHashDir(t, "relative/path")
		// This will fail due to directory not existing, but not due to relative path restriction
		// We expect either a config error or manager error (directory doesn't exist)
		assert.True(t, configErr != nil || managerErr != nil, "expected an error for non-existent directory")
	})
}

// TestShortFlags tests that short flags work the same as long flags
func TestShortFlags(t *testing.T) {
	tests := []struct {
		name      string
		args      []string
		checkFunc func(t *testing.T)
	}{
		{
			name: "short flag -c sets configPath",
			args: []string{"runner", "-c", "/path/to/config.toml"},
			checkFunc: func(t *testing.T) {
				assert.Equal(t, "/path/to/config.toml", configPath)
			},
		},
		{
			name: "short flag -n sets dryRun",
			args: []string{"runner", "-n"},
			checkFunc: func(t *testing.T) {
				assert.True(t, dryRun)
			},
		},
		{
			name: "short flag -g sets groups",
			args: []string{"runner", "-g", "build,test"},
			checkFunc: func(t *testing.T) {
				assert.Equal(t, "build,test", groups)
			},
		},
		{
			name: "short flag -l sets logLevel",
			args: []string{"runner", "-l", "debug"},
			checkFunc: func(t *testing.T) {
				assert.Equal(t, "debug", logLevel)
			},
		},
		{
			name: "short flag -q sets forceQuiet",
			args: []string{"runner", "-q"},
			checkFunc: func(t *testing.T) {
				assert.True(t, forceQuiet)
			},
		},
		{
			name: "long and short flags can be mixed",
			args: []string{"runner", "-c", "/path/to/config.toml", "--dry-run", "-g", "build"},
			checkFunc: func(t *testing.T) {
				assert.Equal(t, "/path/to/config.toml", configPath)
				assert.True(t, dryRun)
				assert.Equal(t, "build", groups)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test flags
			cleanup := setupTestFlags()
			defer cleanup()

			// Set test args
			os.Args = tt.args

			// Parse flags
			flag.Parse()

			// Check result
			tt.checkFunc(t)
		})
	}
}

// TestShortFlagsEquivalence tests that short flags are equivalent to long flags
func TestShortFlagsEquivalence(t *testing.T) {
	tests := []struct {
		name      string
		shortArgs []string
		longArgs  []string
	}{
		{
			name:      "config flag equivalence",
			shortArgs: []string{"runner", "-c", "/path/to/config.toml"},
			longArgs:  []string{"runner", "--config", "/path/to/config.toml"},
		},
		{
			name:      "dry-run flag equivalence",
			shortArgs: []string{"runner", "-n"},
			longArgs:  []string{"runner", "--dry-run"},
		},
		{
			name:      "groups flag equivalence",
			shortArgs: []string{"runner", "-g", "build,test"},
			longArgs:  []string{"runner", "--groups", "build,test"},
		},
		{
			name:      "log-level flag equivalence",
			shortArgs: []string{"runner", "-l", "debug"},
			longArgs:  []string{"runner", "--log-level", "debug"},
		},
		{
			name:      "quiet flag equivalence",
			shortArgs: []string{"runner", "-q"},
			longArgs:  []string{"runner", "--quiet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test short form
			cleanup1 := setupTestFlags()
			os.Args = tt.shortArgs
			flag.Parse()
			shortConfigPath := configPath
			shortDryRun := dryRun
			shortGroups := groups
			shortLogLevel := logLevel
			shortForceQuiet := forceQuiet
			cleanup1()

			// Test long form
			cleanup2 := setupTestFlags()
			os.Args = tt.longArgs
			flag.Parse()
			longConfigPath := configPath
			longDryRun := dryRun
			longGroups := groups
			longLogLevel := logLevel
			longForceQuiet := forceQuiet
			cleanup2()

			// Verify equivalence
			assert.Equal(t, longConfigPath, shortConfigPath, "configPath should be the same")
			assert.Equal(t, longDryRun, shortDryRun, "dryRun should be the same")
			assert.Equal(t, longGroups, shortGroups, "groups should be the same")
			assert.Equal(t, longLogLevel, shortLogLevel, "logLevel should be the same")
			assert.Equal(t, longForceQuiet, shortForceQuiet, "forceQuiet should be the same")
		})
	}
}

// TestRunnerDeclaresRealUIDOnlyPolicy verifies that this binary's init()
// declared RealUIDOnly as the process-wide permission check UID policy, and
// that under that policy SUDO_UID is not adopted even when the real UID is 0,
// and the existence check is never invoked. This test only reads the
// process-wide default policy; it does not modify it, so it must not run in
// parallel with tests that do.
func TestRunnerDeclaresRealUIDOnlyPolicy(t *testing.T) {
	require.Equal(t, groupmembership.RealUIDOnly, groupmembership.ProcessPermissionCheckUIDPolicy())

	deps := groupmembership.NewPermissionCheckUIDDepsForTesting()
	deps.Getenv = func(string) string { return "1000" }
	deps.VerifyUserExists = func(int) error {
		t.Error("VerifyUserExists must not be called under RealUIDOnly")
		return nil
	}
	deps.ReportAdoption = func(groupmembership.PermissionCheckUIDPolicy, int, int) {
		t.Error("ReportAdoption must not be called under RealUIDOnly")
	}

	uid, err := groupmembership.ResolvePermissionCheckUID(
		groupmembership.ProcessPermissionCheckUIDPolicy(), 0, deps)
	require.NoError(t, err)
	assert.Equal(t, 0, uid)
}

// TestResolveRunID covers every branch of the boundary validation applied to
// --run-id: both spellings of "not supplied", an accepted value, and a rejected
// one.
func TestResolveRunID(t *testing.T) {
	const bootstrapID = "01K2YK812JA735M4TWZ6BK0JH9"

	tests := []struct {
		name      string
		flagValue string
		want      string
		wantErr   bool
	}{
		{
			name:      "flag_not_supplied_uses_bootstrap_id",
			flagValue: "",
			want:      bootstrapID,
		},
		{
			name:      "accepted_value_is_adopted",
			flagValue: "my-custom-run-001",
			want:      "my-custom-run-001",
		},
		{
			name:      "rejected_value_yields_error_and_empty_run_id",
			flagValue: "../evil",
			want:      "",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRunID(tt.flagValue, bootstrapID)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, logging.ErrInvalidRunID)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, got)
		})
	}

	// An explicitly empty --run-id must behave exactly like an unset one. This
	// goes through the real flag set rather than calling resolveRunID with ""
	// directly, because the claim under test is that flag parsing cannot tell
	// the two apart in the first place.
	t.Run("explicitly_empty_flag_uses_bootstrap_id", func(t *testing.T) {
		cleanup := setupTestFlags()
		defer cleanup()

		os.Args = []string{"runner", "-run-id="}
		flag.Parse()
		require.Empty(t, runID, "an explicitly empty --run-id parses to the empty string")

		got, err := resolveRunID(runID, bootstrapID)
		require.NoError(t, err)
		assert.Equal(t, bootstrapID, got)
	})
}

// The startup directory permission audit records its summary through the
// default slog logger, which is process-wide state. Every test below replaces
// it, so none of them may call t.Parallel.

// captureLogs redirects the default slog logger into a buffer for the duration
// of the test and returns the buffer.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })
	return buf
}

// fakeDirPermChecker reports a violation for exactly the directories named in
// violating and passes everything else, so a test can fix the verdict without
// depending on the permissions of the host's real directories.
type fakeDirPermChecker struct {
	violating map[string]struct{}
}

func (f *fakeDirPermChecker) ValidateDirectoryPermissions(path string) error {
	if _, ok := f.violating[path]; ok {
		return errFakeDirViolation
	}
	return nil
}

var (
	errFakeDirViolation   = errors.New("fake directory permission violation")
	errCheckerUnavailable = errors.New("checker construction failed")
)

// countingPermChecker answers from the file system -- absent for a directory
// that does not exist, sound for one that does -- and keeps its own tally of
// each answer, so a test can state the expected coverage figures without
// depending on how deep the host's temporary directories sit or on what the
// default hash directory's ancestors look like.
type countingPermChecker struct {
	asked    []string
	existing int
	missing  int
}

func (c *countingPermChecker) ValidateDirectoryPermissions(path string) error {
	c.asked = append(c.asked, path)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		c.missing++
		return fmt.Errorf("%s: %w", path, fs.ErrNotExist)
	}
	c.existing++
	return nil
}

// auditSummaryMessage is the message the startup audit logs its coverage under.
const auditSummaryMessage = "startup directory permission audit completed"

// auditAttr returns the value logged for key in the audit summary line. Whole
// key=value tokens are matched, so a lookup is not satisfied by a longer value
// that merely starts with the expected one.
func auditAttr(t *testing.T, logs, key string) string {
	t.Helper()
	for line := range strings.SplitSeq(logs, "\n") {
		if !strings.Contains(line, auditSummaryMessage) {
			continue
		}
		for field := range strings.FieldsSeq(line) {
			if name, value, ok := strings.Cut(field, "="); ok && name == key {
				return value
			}
		}
		t.Fatalf("audit summary line carries no %s attribute: %s", key, line)
	}
	t.Fatalf("no audit summary line was logged; got: %s", logs)
	return ""
}

// permCheckerReturning adapts a ready-made checker to the constructor seam
// auditConfiguredDirPermissions takes, so tests inject through the same parameter production
// passes security.NewDirectoryPermChecker to.
func permCheckerReturning(checker isec.DirectoryPermChecker) func() (isec.DirectoryPermChecker, error) {
	return func() (isec.DirectoryPermChecker, error) { return checker, nil }
}

// allowAllDirs returns a checker that reports no violation anywhere.
func allowAllDirs() isec.DirectoryPermChecker {
	return &fakeDirPermChecker{}
}

// auditConfig builds a configuration whose group-level verify_files are the
// given raw templates. They are deliberately left unexpanded: the startup audit
// is what decides whether each one can be resolved yet.
func auditConfig(groupVerifyFiles ...string) *runnertypes.ConfigSpec {
	return &runnertypes.ConfigSpec{
		Groups: []runnertypes.GroupSpec{{
			Name:        "audited",
			VerifyFiles: groupVerifyFiles,
		}},
	}
}

// TestStartupDirPermAudit_LogsZeroSkipCounts verifies that a run that leaves
// nothing out still records both skip counts. Recording only when something was
// excluded would make "nothing excluded" indistinguishable from "the audit never
// got as far as reporting".
func TestStartupDirPermAudit_LogsZeroSkipCounts(t *testing.T) {
	dir := tu.SafeTempDir(t)
	logs := captureLogs(t)

	cfg := auditConfig(filepath.Join(dir, "group-verify.txt"))
	cfg.Groups[0].Commands = []runnertypes.CommandSpec{{Name: "c", Cmd: filepath.Join(dir, "tool")}}
	runtimeGlobal := &runnertypes.RuntimeGlobal{
		ExpandedVerifyFiles: []string{filepath.Join(dir, "global-verify.txt")},
	}

	_, err := auditConfiguredDirPermissions(cfg, runtimeGlobal, "test-run", permCheckerReturning(allowAllDirs()))
	require.NoError(t, err)

	out := logs.String()
	assert.Equal(t, "0", auditAttr(t, out, "skipped_variable_reference_paths"))
	assert.Equal(t, "0", auditAttr(t, out, "skipped_relative_paths"))
}

// TestStartupDirPermAudit_LogsCoverageCounts verifies the three directory counts
// against a checker that answers from the file system and tallies its own
// answers. Reporting "no violations" says nothing about how much of what was
// collected the audit actually reached, which is the gap these three close.
func TestStartupDirPermAudit_LogsCoverageCounts(t *testing.T) {
	dir := tu.SafeTempDir(t)
	logs := captureLogs(t)

	// The two tallies have to differ, or reporting checked as skipped and skipped
	// as checked would still satisfy the assertions below. How many directories
	// exist above a temporary directory -- and how much of the default hash
	// directory is installed -- is the host's choice, so measure it rather than
	// assume it: a probe run over a single existing path counts exactly the
	// existing directories this host contributes.
	probe := &countingPermChecker{}
	_, err := auditConfiguredDirPermissions(auditConfig(filepath.Join(dir, "present.txt")),
		&runnertypes.RuntimeGlobal{}, "test-run", permCheckerReturning(probe))
	require.NoError(t, err)
	require.NotZero(t, probe.existing, "the audit must reach at least one existing directory")
	logs.Reset()

	// One path under a directory that exists and one under a chain of absent
	// directories longer than everything the probe found, so missing exceeds
	// existing on any host and transposing the two fails this test.
	absent := filepath.Join(dir, "absent")
	for range probe.existing + 1 {
		absent = filepath.Join(absent, "deeper")
	}
	cfg := auditConfig(
		filepath.Join(dir, "present.txt"),
		filepath.Join(absent, "still-absent.txt"),
	)
	checker := &countingPermChecker{}

	_, err = auditConfiguredDirPermissions(cfg, &runnertypes.RuntimeGlobal{}, "test-run", permCheckerReturning(checker))
	require.NoError(t, err)

	require.NotZero(t, checker.existing, "the tally must distinguish two non-empty groups")
	require.Greater(t, checker.missing, checker.existing,
		"equal tallies would let checked and skipped be reported the wrong way round")

	out := logs.String()
	assert.Equal(t, strconv.Itoa(len(checker.asked)), auditAttr(t, out, "collected_dirs"))
	assert.Equal(t, strconv.Itoa(checker.existing), auditAttr(t, out, "checked_dirs"))
	assert.Equal(t, strconv.Itoa(checker.missing), auditAttr(t, out, "skipped_missing_dirs"))
}

// TestStartupDirPermAudit_LogsSkipBreakdownByReason verifies that the two
// exclusion reasons are counted separately, so a reader can tell a path awaiting
// group expansion from one that was never anchored to anything.
func TestStartupDirPermAudit_LogsSkipBreakdownByReason(t *testing.T) {
	logs := captureLogs(t)

	cfg := auditConfig("/opt/%{TOOLCHAIN}/bin/cc", "relative/tool.sh")
	runtimeGlobal := &runnertypes.RuntimeGlobal{}

	_, err := auditConfiguredDirPermissions(cfg, runtimeGlobal, "test-run", permCheckerReturning(allowAllDirs()))
	require.NoError(t, err)

	out := logs.String()
	assert.Equal(t, "1", auditAttr(t, out, "skipped_variable_reference_paths"))
	assert.Equal(t, "1", auditAttr(t, out, "skipped_relative_paths"))
}

// TestStartupDirPermAudit_EscapedBraceTemplateIsStillAudited pins which layer
// decides that a group template holds a reference. The configuration escape
// "\\%{X}" produces a literal "%{X}", so this template holds none and must be
// audited -- only the expansion parser can tell it from a real reference, and
// this assertion fails if the decision reverts to a substring test on "%{".
func TestStartupDirPermAudit_EscapedBraceTemplateIsStillAudited(t *testing.T) {
	dir := tu.SafeTempDir(t)
	logs := captureLogs(t)

	template := filepath.Join(dir, `\%{NOT_A_REFERENCE}`, "tool")
	checker := &countingPermChecker{}

	_, err := auditConfiguredDirPermissions(auditConfig(template), &runnertypes.RuntimeGlobal{}, "test-run",
		permCheckerReturning(checker))
	require.NoError(t, err)

	assert.Equal(t, "0", auditAttr(t, logs.String(), "skipped_variable_reference_paths"),
		"an escaped brace is a literal, so nothing here is awaiting expansion")
	assert.Contains(t, checker.asked, filepath.Dir(template),
		"the template's own directory must reach the checker, not merely fail to raise a violation")
}

// TestStartupDirPermAudit_ExpandedGlobalPathWithBraceIsAudited pins the other
// half of the per-origin declaration. Global verify files are the output of
// expansion, so a "%{" surviving in one is a literal an escape produced and the
// path is audited as written. This assertion fails if the loop is ever
// "tidied up" into judging those paths the way group templates are judged.
func TestStartupDirPermAudit_ExpandedGlobalPathWithBraceIsAudited(t *testing.T) {
	dir := tu.SafeTempDir(t)
	braceDir := filepath.Join(dir, "%{NOT_A_REFERENCE}")
	require.NoError(t, os.Mkdir(braceDir, 0o755))

	logs := captureLogs(t)
	checker := &countingPermChecker{}
	runtimeGlobal := &runnertypes.RuntimeGlobal{
		ExpandedVerifyFiles: []string{filepath.Join(braceDir, "file.txt")},
	}

	_, err := auditConfiguredDirPermissions(auditConfig(), runtimeGlobal, "test-run", permCheckerReturning(checker))
	require.NoError(t, err)

	assert.Equal(t, "0", auditAttr(t, logs.String(), "skipped_variable_reference_paths"),
		"an already-expanded path holds no reference, whatever its text looks like")
	assert.Contains(t, checker.asked, braceDir)
}

// TestStartupDirPermAudit_SkipDoesNotAffectVerdict verifies that leaving a path
// out is not itself a fault, and that it neither adds to nor masks the
// violations found among the paths that were audited.
func TestStartupDirPermAudit_SkipDoesNotAffectVerdict(t *testing.T) {
	skipped := []string{"/opt/%{TOOLCHAIN}/bin/cc", "relative/tool.sh"}

	t.Run("skipped_paths_alone_are_not_violations", func(t *testing.T) {
		captureLogs(t)

		checker, err := auditConfiguredDirPermissions(auditConfig(skipped...), &runnertypes.RuntimeGlobal{}, "test-run",
			permCheckerReturning(allowAllDirs()))
		require.NoError(t, err)
		assert.NotNil(t, checker, "a clean audit returns the checker for reuse by per-group checks")
	})

	t.Run("violation_count_matches_the_violating_directories", func(t *testing.T) {
		captureLogs(t)

		firstDir := tu.SafeTempDir(t)
		secondDir := tu.SafeTempDir(t)
		checker := &fakeDirPermChecker{violating: map[string]struct{}{
			firstDir:  {},
			secondDir: {},
		}}

		cfg := auditConfig(append(slices.Clone(skipped),
			filepath.Join(firstDir, "a.txt"),
			filepath.Join(secondDir, "b.txt"))...)

		_, err := auditConfiguredDirPermissions(cfg, &runnertypes.RuntimeGlobal{}, "test-run", permCheckerReturning(checker))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "2 directory violation(s)",
			"only the two violating directories count; the excluded paths must not inflate or mask the total")
	})
}

// TestStartupDirPermAudit_CheckerInitFailureReturnsPreExecutionError verifies
// that a checker that cannot be built is reported through the ordinary
// pre-execution error path instead of terminating the process with a stack
// trace. The real constructor never fails today, so the failure has to be
// injected through the same seam production passes it to.
func TestStartupDirPermAudit_CheckerInitFailureReturnsPreExecutionError(t *testing.T) {
	captureLogs(t)

	failing := func() (isec.DirectoryPermChecker, error) { return nil, errCheckerUnavailable }

	_, err := auditConfiguredDirPermissions(auditConfig(), &runnertypes.RuntimeGlobal{}, "test-run", failing)
	require.Error(t, err)

	preExec, ok := errors.AsType[*logging.PreExecutionError](err)
	require.True(t, ok, "checker construction failure must reach the pre-execution error path")
	assert.Equal(t, logging.ErrorTypeFileAccess, preExec.Type)
	assert.Equal(t, string(resource.ComponentVerification), preExec.Component)
	assert.Equal(t, "test-run", preExec.RunID)
	assert.Contains(t, preExec.Message, errCheckerUnavailable.Error())
}

// TestNewDryRunFormatter_UnknownFormatReturnsError verifies the fail-secure
// default. cli.ParseDryRunOutputFormat rejects unknown strings, so the value is
// constructed directly here; without the default branch the caller would be
// handed a nil formatter and panic on the next call.
func TestNewDryRunFormatter_UnknownFormatReturnsError(t *testing.T) {
	t.Parallel()

	formatter, err := newDryRunFormatter(resource.OutputFormatJSON + 1)
	require.ErrorIs(t, err, errUnknownDryRunOutputFormat)
	assert.Nil(t, formatter)
}
