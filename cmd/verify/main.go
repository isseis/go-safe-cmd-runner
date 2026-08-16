// Package main provides the verify command for the go-safe-cmd-runner.
// It verifies file integrity using previously recorded hashes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/security"
)

// Exit codes returned by run(). 2 is deliberately unused: the Go runtime exits
// with status 2 on an uncaught panic, and this command has reachable panics
// (the policy declaration in init and the checker initialisation in
// checkDirPermissions). Reusing 2 would make "violation detected" and "verify
// crashed" indistinguishable.
const (
	exitOK = 0
	// exitVerificationFailed covers every ordinary failure, not only a failed
	// hash comparison: an argument error, an unresolvable permission-check UID,
	// a validator that could not be built, or one or more files failing
	// verification.
	exitVerificationFailed   = 1
	exitUntrustedEnvironment = 3
)

var errNoFilesProvided = errors.New("at least one file path must be provided as a positional argument or via -file (deprecated)")

// deps holds injectable dependencies for the verify command.
// This makes the dependency graph visible at call sites and keeps tests off
// process-wide mutable state. Only the fields whose comment says so may be left
// nil; the rest are required and are set by defaultDeps.
type deps struct {
	validatorFactory func(hashDir string) (hashValidator, error)
	// newPermChecker builds the directory permission checker. Injected as a
	// constructor rather than a ready-made checker so tests can exercise its error
	// return, which no current implementation produces, and so no test can bypass
	// the production construction path.
	newPermChecker func() (security.DirectoryPermChecker, error)
	// resolvePathForCheck resolves a path for the directory permission check.
	resolvePathForCheck func(path string) (string, error)
	// nil means use groupmembership.New().EnsurePermissionCheckUID.
	ensurePermissionCheckUID func() error
}

func defaultDeps() deps {
	return deps{
		validatorFactory: func(hashDir string) (hashValidator, error) {
			return cmdcommon.CreateReadOnlyValidator(hashDir)
		},
		newPermChecker:      security.NewDirectoryPermChecker,
		resolvePathForCheck: security.ResolvePathForCheck,
	}
}

func init() {
	// verify is invoked as `sudo verify ...`; the read-safety check must judge
	// access from the invoking user's point of view, so SUDO_UID is consulted.
	if err := groupmembership.SetProcessPermissionCheckUIDPolicy(groupmembership.SudoUIDAware); err != nil {
		panic(fmt.Sprintf("failed to declare permission check UID policy %s (current=%s): %v",
			groupmembership.SudoUIDAware, groupmembership.ProcessPermissionCheckUIDPolicy(), err))
	}
}

type hashValidator interface {
	Verify(filePath string) error
}

type verifyConfig struct {
	files          []string
	hashDir        string
	usedDeprecated bool
}

func main() {
	os.Exit(run(os.Args[1:], defaultDeps(), os.Stdout, os.Stderr))
}

// checkDirPermissions reports whether verification may proceed.
//
// Only the hash directory side is fail-closed. It is the root of trust: if it
// or an ancestor is writable by someone else, hash records can be replaced and
// any verdict this command produces is meaningless. A violation confined to a
// target file's ancestors is the opposite case — that is what verify exists to
// inspect — so it stays a warning and verification continues.
func checkDirPermissions(cfg *verifyConfig, d deps, stderr io.Writer) bool {
	secValidator, secErr := d.newPermChecker()
	if secErr != nil {
		// NewDirectoryPermChecker only fails when standalone checker setup fails,
		// which is not recoverable in this startup path.
		panic(fmt.Sprintf("security validator initialisation failed: %v", secErr))
	}
	absHashDir := cfg.hashDir
	if abs, err := filepath.Abs(cfg.hashDir); err == nil {
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			absHashDir = resolved
		} else {
			absHashDir = abs
		}
	}

	logger := slog.Default()
	hashDirs := security.CollectTOCTOUCheckDirs(nil, nil, absHashDir)
	// The ERROR below is in addition to the WARN RunTOCTOUPermissionCheck already
	// logs, so the level distinguishes a violation that stopped the run from one
	// that did not.
	if violations := security.RunTOCTOUPermissionCheck(secValidator, hashDirs, logger).Violations; len(violations) > 0 {
		// The remediation names neither a directory nor a command: v.Path is the
		// directory that was checked, not necessarily the one at fault (the checker
		// walks from the root down), and ErrInvalidDirPermissions covers causes
		// needing chmod go-w, chown or chmod g-w, which cannot be told apart without
		// matching on the error text. The violation attribute carries both.
		const remediation = "fix the permissions and ownership of the directory named in the violation, or move the hash directory to a properly permissioned path, then re-run verify"
		for _, v := range violations {
			logger.Error(
				"hash directory permission violation detected — refusing to verify",
				slog.String("path", v.Path),
				slog.String("violation", v.Err.Error()),
				slog.String("remediation", remediation),
			)
		}
		fmt.Fprintln(stderr, "Error: permission violation in hash directory or its ancestor directories — verification results cannot be trusted; no file was verified. Fix directory permissions and re-run.") //nolint:errcheck
		return false
	}

	// Resolved here rather than alongside the hash directory: a run that fails
	// closed does not touch the target files at all.
	absFiles := make([]string, 0, len(cfg.files))
	for _, f := range cfg.files {
		abs, err := filepath.Abs(f)
		if err != nil {
			abs = f
		}
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			absFiles = append(absFiles, resolved)
		} else {
			absFiles = append(absFiles, abs)
		}
	}
	// Directories already covered above are skipped so a shared ancestor is not
	// warned about twice.
	checked := make(map[string]struct{}, len(hashDirs))
	for _, dir := range hashDirs {
		checked[dir] = struct{}{}
	}
	var targetDirs []string
	for _, dir := range security.CollectTOCTOUCheckDirs(absFiles, nil, "") {
		if _, ok := checked[dir]; !ok {
			targetDirs = append(targetDirs, dir)
		}
	}
	// Violations on the target files are warnings only; the run continues.
	security.RunTOCTOUPermissionCheck(secValidator, targetDirs, logger)
	return true
}

func run(args []string, d deps, stdout, stderr io.Writer) int {
	cfg, fs, err := parseArgs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		printUsage(fs, stderr)
		fmt.Fprintf(stderr, "Error: %v\n", err) //nolint:errcheck
		return exitVerificationFailed
	}

	if cfg.usedDeprecated {
		fmt.Fprintln(stderr, "Warning: -file flag is deprecated and will be removed in a future release. Specify files as positional arguments.") //nolint:errcheck
	}

	// verify declares SudoUIDAware (see init), so this is where an unverifiable
	// SUDO_UID fails the run and where the adoption record is emitted. verify's
	// per-file reads would also reach it, but resolving here makes the failure
	// arrive once, before the first file, rather than once per file.
	ensureUID := d.ensurePermissionCheckUID
	if ensureUID == nil {
		ensureUID = groupmembership.New().EnsurePermissionCheckUID
	}
	if err := ensureUID(); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err) //nolint:errcheck
		return exitVerificationFailed
	}

	if !checkDirPermissions(cfg, d, stderr) {
		return exitUntrustedEnvironment
	}

	validator, err := d.validatorFactory(cfg.hashDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating validator: %v\n", err) //nolint:errcheck
		return exitVerificationFailed
	}

	return processFiles(validator, cfg.files, stdout, stderr)
}

// parseArgs turns the command line into a verifyConfig. It has no side effects:
// verify never creates the hash directory it names, so a missing one is reported
// later rather than filled in here.
func parseArgs(args []string, stderr io.Writer) (*verifyConfig, *flag.FlagSet, error) {
	options := struct {
		deprecatedFile string
		hashDir        string
	}{}

	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(fs, stderr) }
	fs.StringVar(&options.deprecatedFile, "file", "", "DEPRECATED: Path to the file to verify (use positional arguments instead)")
	fs.StringVar(&options.hashDir, "hash-dir", "", "Directory containing hash files (default: current working directory)")
	fs.StringVar(&options.hashDir, "d", "", "Short alias for -hash-dir")

	if err := fs.Parse(args); err != nil {
		return nil, fs, err
	}

	files := fs.Args()
	if options.deprecatedFile != "" {
		files = append([]string{options.deprecatedFile}, files...)
	}
	if len(files) == 0 {
		return nil, fs, errNoFilesProvided
	}

	dir := options.hashDir
	if dir == "" {
		dir = cmdcommon.DefaultHashDirectory
	}

	return &verifyConfig{
		files:          files,
		hashDir:        dir,
		usedDeprecated: options.deprecatedFile != "",
	}, fs, nil
}

func printUsage(fs *flag.FlagSet, w io.Writer) {
	if fs == nil {
		return
	}
	fmt.Fprintf(w, "Usage: %s [flags] <file> [<file>...]\n", filepath.Base(os.Args[0])) //nolint:errcheck,gosec // G705: writing to stdout/stderr, not an HTTP response
	fs.PrintDefaults()
}

func processFiles(validator hashValidator, files []string, stdout, stderr io.Writer) int {
	total := len(files)
	label := "files"
	if total == 1 {
		label = "file"
	}

	fmt.Fprintf(stdout, "Verifying %d %s...\n", total, label) //nolint:errcheck,gosec // G705: writing to stdout, not an HTTP response

	successes := 0
	failures := 0

	for idx, filePath := range files {
		fmt.Fprintf(stdout, "[%d/%d] %s: ", idx+1, total, filePath) //nolint:errcheck,gosec // G705: writing to stdout, not an HTTP response
		if err := validator.Verify(filePath); err != nil {
			failures++
			fmt.Fprintln(stdout, "FAILED")                                         //nolint:errcheck
			fmt.Fprintf(stderr, "Verification failed for %s: %v\n", filePath, err) //nolint:errcheck,gosec // G705: writing to stderr, not an HTTP response
			continue
		}
		successes++
		fmt.Fprintln(stdout, "OK") //nolint:errcheck
	}

	fmt.Fprintf(stdout, "\nSummary: %d succeeded, %d failed\n", successes, failures) //nolint:errcheck
	if failures > 0 {
		return exitVerificationFailed
	}
	return exitOK
}
