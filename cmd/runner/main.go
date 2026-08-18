// Package main provides the entry point for the command runner application.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/logging"
	"github.com/isseis/go-safe-cmd-runner/internal/redaction"
	"github.com/isseis/go-safe-cmd-runner/internal/runner"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/privilege"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/base/runnertypes"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/bootstrap"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/cli"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/config"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/resource"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
	"github.com/isseis/go-safe-cmd-runner/internal/verification"
)

// SilentExitError indicates that the program should exit with status 1
// without printing additional error messages (e.g., for validation failures
// where the validation report has already been displayed)
type SilentExitError struct{}

func (e SilentExitError) Error() string {
	return "silent exit requested"
}

var (
	configPath       string
	logLevel         string
	logDir           string
	dryRun           bool
	dryRunFormat     string
	dryRunDetail     string
	showSensitive    bool
	runID            string
	forceInteractive bool
	forceQuiet       bool
	keepTempDirs     bool
	groups           string
)

func init() {
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

	flag.StringVar(&logDir, "log-dir", "", "directory to place per-run JSON log (auto-named). Overrides TOML/env if set.")
	flag.StringVar(&dryRunFormat, "dry-run-format", "text", "dry-run output format (text, json)")
	flag.StringVar(&dryRunDetail, "dry-run-detail", "detailed", "dry-run detail level (summary, detailed, full)")
	flag.BoolVar(&showSensitive, "show-sensitive", false, "show sensitive information in dry-run output (use with caution)")
	flag.StringVar(&runID, "run-id", "", "unique identifier for this execution run ("+logging.RunIDFormatDescription()+"; auto-generates ULID if not provided)")
	flag.BoolVar(&forceInteractive, "interactive", false, "force interactive mode with colored output (overrides environment detection)")
	flag.BoolVar(&keepTempDirs, "keep-temp-dirs", false, "keep temporary directories after execution")

	// runner runs as a setuid-root binary started by an unprivileged user; sudo
	// invocation is not a supported deployment, so SUDO_UID is never consulted.
	// This declares the same value as the final default policy: it exists to
	// make the intent explicit, not to change behavior.
	if err := groupmembership.SetProcessPermissionCheckUIDPolicy(groupmembership.RealUIDOnly); err != nil {
		panic(fmt.Sprintf("failed to declare permission check UID policy %s (current=%s): %v",
			groupmembership.RealUIDOnly, groupmembership.ProcessPermissionCheckUIDPolicy(), err))
	}
}

// startupPrivilegeStage identifies which step of the startup privilege drop failed.
type startupPrivilegeStage string

const (
	stageSetegid startupPrivilegeStage = "setegid"
	stageSeteuid startupPrivilegeStage = "seteuid"
)

// startupPrivilegeError reports a failure of the startup privilege drop.
type startupPrivilegeError struct {
	Stage startupPrivilegeStage
	Err   error
}

func (e *startupPrivilegeError) Error() string {
	return fmt.Sprintf("%s failed: %v", e.Stage, e.Err)
}

func (e *startupPrivilegeError) Unwrap() error {
	return e.Err
}

// dropStartupPrivileges lowers the effective GID and UID to the given targets.
//
// The GID is dropped first so that a failure never leaves the process holding a
// privileged group while running as an unprivileged user. The targets are
// parameters only so tests can pass one the process cannot reach; production
// passes the real UID and GID.
func dropStartupPrivileges(targetUID, targetGID int) error {
	if err := syscall.Setegid(targetGID); err != nil {
		return &startupPrivilegeError{Stage: stageSetegid, Err: err}
	}
	if err := syscall.Seteuid(targetUID); err != nil {
		return &startupPrivilegeError{Stage: stageSeteuid, Err: err}
	}
	return nil
}

// reportStartupPrivilegeFailure reports err and returns the process exit code.
// It generates its own run ID because the drop runs before this execution's run
// ID exists, and a report must never carry an empty one.
func reportStartupPrivilegeFailure(err error) int {
	logging.HandlePreExecutionError(logging.ErrorTypePrivilegeDrop,
		fmt.Sprintf("Failed to drop startup privileges: %v", err), "main", logging.GenerateRunID())
	return 1
}

// resolveRunID returns the run ID to use for this execution, falling back to
// bootstrapID when --run-id was not supplied.
//
// Invariant: given a valid bootstrapID the returned run ID is valid too, so no
// downstream caller has to re-check it. An empty flagValue counts as "not
// supplied" because Go's flag package cannot distinguish it from an unset flag.
func resolveRunID(flagValue, bootstrapID string) (string, error) {
	if flagValue == "" {
		return bootstrapID, nil
	}
	if err := logging.ValidateRunID(flagValue); err != nil {
		return "", err
	}
	return flagValue, nil
}

func main() {
	// First in main's body so that no input is processed at the privileges the
	// process was started with. Only the effective IDs are lowered, so the
	// privilege manager can still raise them for the commands configured to
	// need it.
	if err := dropStartupPrivileges(syscall.Getuid(), syscall.Getgid()); err != nil {
		os.Exit(reportStartupPrivilegeFailure(err))
	}

	// Produced before flag parsing so that rejecting --run-id never has to fall
	// back on the rejected value to identify the run.
	bootstrapID := logging.GenerateRunID()

	flag.Parse()

	// Everything downstream (log file names, RUN_SUMMARY lines, log attributes,
	// Slack notifications) reads the package-level runID, so the validated value
	// is assigned back to it and the raw flag value never travels further.
	resolvedRunID, err := resolveRunID(runID, bootstrapID)
	if err != nil {
		// The rejected value is deliberately absent from both the message and
		// the reported run ID: reporting it would put attacker-controlled bytes
		// on the very output paths this validation protects.
		logging.HandlePreExecutionError(logging.ErrorTypeInvalidRunID,
			fmt.Sprintf("Invalid run ID passed to --run-id: %v (accepted format: %s)", err, logging.RunIDFormatDescription()),
			"main", bootstrapID)
		os.Exit(1)
	}
	runID = resolvedRunID

	// Should never fail in production; catches build-time misconfiguration.
	if !filepath.IsAbs(cmdcommon.DefaultHashDirectory) {
		logging.HandlePreExecutionError(logging.ErrorTypeBuildConfig, fmt.Sprintf("Invalid default hash directory: must be absolute path, got: %s", cmdcommon.DefaultHashDirectory), "main", runID)
		os.Exit(1)
	}

	exitCode := mainWithExitCode(runID)

	// Slack notifications are queued, not sent inline. Nothing below writes to
	// Slack, so this is the last point at which the queue can be complete.
	bootstrap.FlushSlackNotifications()

	bootstrap.ReportRedactionFailures()

	os.Exit(exitCode)
}

// mainWithExitCode runs the main logic and returns the exit code
func mainWithExitCode(runID string) int {
	if err := run(runID); err != nil {
		var silentErr SilentExitError
		var preExecErr *logging.PreExecutionError
		var execErr *logging.ExecutionError
		var previewExit dryRunPreviewExit
		switch {
		case errors.As(err, &previewExit):
			// The preview was already printed, so exit with its recommended code
			// without logging an error.
			return previewExit.code
		case errors.As(err, &silentErr):
			// The validation report was already printed.
			// revive:disable:empty-block This empty block is intentional to handle specific cases
		case errors.As(err, &preExecErr):
			logging.HandlePreExecutionError(preExecErr.Type, preExecErr.Message, preExecErr.Component, runID)
		case errors.As(err, &execErr):
			logging.HandleExecutionError(execErr)
		default:
			logging.HandlePreExecutionError(logging.ErrorTypeSystemError, err.Error(), "main", runID)
		}
		return 1
	}
	return 0
}

// parseLogLevel parses a log level string and returns the corresponding slog.Level value.
// It returns a PreExecutionError if the log level string is invalid.
func parseLogLevel(logLevelStr string, runID string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(logLevelStr)); err != nil {
		return level, &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Invalid log level %q: %v", logLevelStr, err),
			Component: string(resource.ComponentMain),
			RunID:     runID,
		}
	}
	return level, nil
}

func run(runID string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Logging is set up from the command-line level alone, before the config is
	// read, so that verification manager logs already use the custom formatters.
	logLevelValue, err := parseLogLevel(logLevel, runID)
	if err != nil {
		return err
	}
	// In dry-run mode logs go to stderr to keep stdout clean for the preview.
	consoleWriter := os.Stdout
	if dryRun {
		consoleWriter = os.Stderr
	}

	slackConfig, err := bootstrap.ValidateSlackWebhookEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   err.Error(),
			Component: string(resource.ComponentLogging),
			RunID:     runID,
		}
	}

	if err := bootstrap.SetupLogging(bootstrap.SetupLoggingOptions{
		LogLevel:         logLevelValue,
		LogDir:           logDir,
		RunID:            runID,
		ForceInteractive: forceInteractive,
		ForceQuiet:       forceQuiet,
		ConsoleWriter:    consoleWriter,
		DryRun:           dryRun,
	}); err != nil {
		return err
	}

	// Checked before the verification manager exists, so a missing argument is
	// reported as such even when the hash directory is also unusable.
	if configPath == "" {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeRequiredArgumentMissing,
			Message:   "Config file path is required",
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	// Dry-run skips hash directory validation: no file is actually verified.
	var verificationManager *verification.Manager
	if dryRun {
		verificationManager, err = verification.NewManagerForDryRun()
	} else {
		verificationManager, err = bootstrap.NewVerificationManager()
	}
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("Verification manager initialization failed: %v", err),
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	cfg, err := bootstrap.LoadAndPrepareConfig(verificationManager, configPath, runID)
	if err != nil {
		return err
	}

	// Slack handlers are added only now because AllowedHost comes from the TOML.
	redactionConfig, err := bootstrap.SetupSlackLogging(slackConfig, bootstrap.SetupLoggingOptions{
		SlackAllowedHost: cfg.Global.SlackAllowedHost,
		RunID:            runID,
		DryRun:           dryRun,
	})
	if err != nil {
		return err
	}

	slog.Info("Verification and configuration completed",
		"config_path", configPath,
		"hash_directory", cmdcommon.DefaultHashDirectory,
		"dry_run", dryRun)

	runtimeGlobal, err := config.ExpandGlobal(&cfg.Global)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Failed to expand global configuration: %v", err),
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	// Runs after global expansion: templates can only reference global variables.
	if err := config.ValidateAllTemplates(cfg.CommandTemplates, runtimeGlobal.ExpandedVars); err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Template validation failed: %v", err),
			Component: string(resource.ComponentConfig),
			RunID:     runID,
		}
	}

	result, err := verificationManager.VerifyGlobalFiles(&verification.GlobalVerificationInput{
		ExpandedVerifyFiles: runtimeGlobal.ExpandedVerifyFiles,
	})
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   err.Error(),
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	if result.TotalFiles > 0 {
		slog.Info("Global files verification completed successfully",
			"verified", result.VerifiedFiles,
			"duration_ms", result.Duration.Milliseconds(),
			"run_id", runID)
	}

	// The returned validator is reused for per-group checks at execution time, so
	// that group-level paths with %{GROUP_VAR} references are also checked.
	secValidator, err := runTOCTOUCheck(cfg, runtimeGlobal, runID, isec.NewDirectoryPermChecker)
	if err != nil {
		return err
	}

	return executeRunner(ctx, cfg, runtimeGlobal, verificationManager, runID, secValidator, redactionConfig)
}

// checkCandidate pairs a configured path with what the caller knows about
// variable references in it. The knowledge has to travel with the path: the
// security package cannot read it off the text, because expansion has an escape
// and is not idempotent, so a "%{" in a value may be a literal one an escape
// produced rather than a reference still waiting to be expanded.
type checkCandidate struct {
	path  string
	state isec.PathExpansionState
}

// startupExpansionState judges a raw configuration template with the same parser
// expansion itself uses, so the escape rules are not maintained twice. It must be
// given the template, never a value expansion has already produced.
//
// A template that holds no reference is audited under its own text, which for one
// using the escape ("\%{X}") is not the text the path will finally have. That
// costs accuracy in the coverage figures rather than safety: auditing the wrong
// leaf is what the old rule did to every such path by dropping it, and the real
// path is audited before use by the per-group check.
func startupExpansionState(template string) isec.PathExpansionState {
	if config.HasVariableReference(template) {
		return isec.PathHasUnexpandedReference
	}
	return isec.PathExpanded
}

// runTOCTOUCheck collects directory paths referenced by the configuration, audits
// their permissions and returns the checker used, so that per-group checks at
// execution time reuse it. A violation is reported as a PreExecutionError.
//
// Paths that cannot be pointed at a real location yet are left out of the audit:
// one still holding a variable reference has no location until its group is
// expanded, and a relative one is not anchored to this process's working
// directory, so making it absolute here would audit an unrelated tree. Both are
// counted by reason and reported in the summary logged at the end, so the range
// the audit actually covers is visible.
func runTOCTOUCheck(cfg *runnertypes.ConfigSpec, runtimeGlobal *runnertypes.RuntimeGlobal, runID string, newPermChecker func() (isec.DirectoryPermChecker, error)) (isec.DirectoryPermChecker, error) {
	logger := slog.Default()

	candidates := make([]checkCandidate, 0, len(runtimeGlobal.ExpandedVerifyFiles)+len(cfg.Groups))
	for _, f := range runtimeGlobal.ExpandedVerifyFiles {
		// This list is the output of expansion, so it holds no reference by
		// construction; there is nothing here to judge.
		candidates = append(candidates, checkCandidate{path: f, state: isec.PathExpanded})
	}
	for _, g := range cfg.Groups {
		// Group-level paths are still raw templates at startup, so each is judged
		// individually.
		for _, f := range g.VerifyFiles {
			candidates = append(candidates, checkCandidate{path: f, state: startupExpansionState(f)})
		}
		for _, cmd := range g.Commands {
			candidates = append(candidates, checkCandidate{path: cmd.Cmd, state: startupExpansionState(cmd.Cmd)})
		}
	}

	filePaths := make([]string, 0, len(candidates))
	var skippedVariableReference, skippedRelative int
	for _, c := range candidates {
		switch reason := isec.ClassifyCheckTarget(c.path, c.state); reason {
		case isec.CheckSkipNone:
			filePaths = append(filePaths, c.path)
		case isec.CheckSkipVariableReference:
			skippedVariableReference++
		case isec.CheckSkipRelative:
			skippedRelative++
		default:
			// Unreachable today: ClassifyCheckTarget's codomain is the three cases
			// above. A reason added without a case here would otherwise be counted
			// as nothing and silently audited or silently dropped. Refuse to start.
			return nil, &logging.PreExecutionError{
				Type:      logging.ErrorTypeFileAccess,
				Message:   fmt.Sprintf("unhandled check skip reason %d for path %s", reason, c.path),
				Component: string(resource.ComponentVerification),
				RunID:     runID,
			}
		}
	}

	secValidator, secErr := newPermChecker()
	if secErr != nil {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("directory permission checker initialisation failed: %v", secErr),
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}

	// The shared resolver walks to the deepest existing ancestor, resolves
	// symlinks there and appends the remainder lexically, so a directory that does
	// not exist yet is audited in the tree it will really be created under rather
	// than in the one its unresolved path appears to name. A path it cannot
	// resolve is logged at WARN and still handed back in a checkable form, so
	// nothing leaves the audit without a trace.
	resolvedFiles, _ := isec.ResolveAllForCheck(filePaths, logger)
	resolvedHashDirs, _ := isec.ResolveAllForCheck([]string{cmdcommon.DefaultHashDirectory}, logger)

	toctouDirs := isec.CollectPermissionCheckDirs(resolvedFiles, resolvedHashDirs)
	result := isec.RunTOCTOUPermissionCheck(secValidator, toctouDirs, logger)
	// Recorded before the verdict, so a run that aborts still says how much it
	// covered, and on every run rather than only when something was left out: a
	// missing record cannot be told apart from "nothing was left out". The first
	// three attributes count directories, since collection expands each path into
	// all of its ancestors; the last two count configured paths, which is what
	// exclusion happens to, before that expansion.
	logger.Info("startup directory permission audit completed",
		"collected_dirs", len(toctouDirs),
		"checked_dirs", result.Checked,
		"skipped_missing_dirs", result.Skipped,
		"skipped_variable_reference_paths", skippedVariableReference,
		"skipped_relative_paths", skippedRelative,
	)

	if len(result.Violations) > 0 {
		return nil, &logging.PreExecutionError{
			Type:      logging.ErrorTypeFileAccess,
			Message:   fmt.Sprintf("TOCTOU permission check failed: %d directory violation(s) detected; review directory permissions", len(result.Violations)),
			Component: string(resource.ComponentVerification),
			RunID:     runID,
		}
	}
	return secValidator, nil
}

// executeRunner initializes and executes the runner with proper cleanup
func executeRunner(ctx context.Context, cfg *runnertypes.ConfigSpec, runtimeGlobal *runnertypes.RuntimeGlobal, verificationManager *verification.Manager, runID string, secValidator isec.DirectoryPermChecker, redactionConfig *redaction.Config) error {
	logger := slog.Default()
	privMgr := privilege.NewManager(logger)

	runnerOptions := []runner.Option{
		runner.WithVerificationManager(verificationManager),
		runner.WithPrivilegeManager(privMgr),
		runner.WithRunID(runID),
		runner.WithRuntimeGlobal(runtimeGlobal),
		runner.WithKeepTempDirs(keepTempDirs),
		runner.WithRedactionConfig(redactionConfig),
	}
	if secValidator != nil {
		runnerOptions = append(runnerOptions, runner.WithTOCTOUValidator(secValidator))
	}

	// Parsed here rather than at the formatting site below, so an invalid option
	// is rejected before anything is executed.
	var detailLevel resource.DryRunDetailLevel
	var outputFormat resource.OutputFormat

	if dryRun {
		var err error
		detailLevel, err = cli.ParseDryRunDetailLevel(dryRunDetail)
		if err != nil {
			return fmt.Errorf("invalid detail level %q: %w", dryRunDetail, err)
		}

		outputFormat, err = cli.ParseDryRunOutputFormat(dryRunFormat)
		if err != nil {
			return fmt.Errorf("invalid output format %q: %w", dryRunFormat, err)
		}

		dryRunOpts := &resource.DryRunOptions{
			DetailLevel:   detailLevel,
			OutputFormat:  outputFormat,
			ShowSensitive: showSensitive,
			VerifyFiles:   true,
			HashDir:       cmdcommon.DefaultHashDirectory,
		}
		runnerOptions = append(runnerOptions, runner.WithDryRun(dryRunOpts))
	}

	r, err := runner.NewRunner(cfg, runnerOptions...)
	if err != nil {
		return fmt.Errorf("failed to initialize runner: %w", err)
	}

	defer func() {
		if err := r.CleanupAllResources(); err != nil {
			slog.Warn("Failed to cleanup resources", slog.Any("error", err), slog.String("run_id", runID))
		}
	}()

	groupNames, err := cli.FilterGroups(
		cli.ParseGroupNames(groups),
		cfg,
	)
	if err != nil {
		return &logging.PreExecutionError{
			Type:      logging.ErrorTypeConfigParsing,
			Message:   fmt.Sprintf("Invalid groups specified: %v", err),
			Component: string(resource.ComponentRunner),
			RunID:     runID,
		}
	}

	// An empty groupNames executes all groups.
	execErr := r.Execute(ctx, groupNames)

	// Carries the preview's recommended exit code, so a deny preview surfaces as
	// a non-zero process exit even when execution itself did not error.
	var dryRunPreviewCode int

	// The preview is printed even when execution failed, with the failure folded
	// into the result.
	if dryRun {
		if execErr != nil {
			r.SetDryRunExecutionError(
				string(resource.ErrorTypeExecutionError),
				execErr.Error(),
				string(resource.ComponentRunner),
				nil,
				resource.PhaseGroupExecution,
			)
		}

		result := r.GetDryRunResults()
		if result != nil {
			var formatter resource.Formatter
			switch outputFormat {
			case resource.OutputFormatText:
				formatter = resource.NewTextFormatter()
			case resource.OutputFormatJSON:
				formatter = resource.NewJSONFormatter()
			}

			output, err := formatter.FormatResult(result, resource.FormatterOptions{
				DetailLevel:   detailLevel,
				OutputFormat:  outputFormat,
				ShowSensitive: showSensitive,
			})
			if err != nil {
				return fmt.Errorf("formatting failed: %w", err)
			}
			fmt.Print(output)
			dryRunPreviewCode = result.PreviewExitCode
		}
	}

	if execErr != nil {
		var groupName, commandName string
		if cmdExecErr, ok := errors.AsType[*runner.CommandExecutionError](execErr); ok {
			groupName = cmdExecErr.GroupName
			commandName = cmdExecErr.CommandName
		}

		return &logging.ExecutionError{
			Message:     "error running commands",
			Component:   "runner",
			RunID:       runID,
			GroupName:   groupName,
			CommandName: commandName,
			Err:         execErr,
		}
	}

	// The distinct code lets CI tell a verification-unavailable deny apart from a
	// policy deny.
	if dryRunPreviewCode != 0 {
		return dryRunPreviewExit{code: dryRunPreviewCode}
	}

	return nil
}

// dryRunPreviewExit signals that the process should exit with a specific code from
// the dry-run preview. The preview output was already printed, so it carries no
// message to log; mainWithExitCode maps it directly to the exit code.
type dryRunPreviewExit struct{ code int }

func (e dryRunPreviewExit) Error() string {
	return fmt.Sprintf("dry-run preview requested exit code %d", e.code)
}
