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
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/security"
)

// Exit codes returned by run(). 2 is deliberately unused: the Go runtime exits
// with status 2 on an uncaught panic, and one panic remains in this command
// (the policy declaration in init, which fires only on a programming error).
// Reusing 2 would make "violation detected" and "verify crashed"
// indistinguishable.
const (
	exitOK = 0
	// exitVerificationFailed covers every ordinary failure, not only a failed
	// hash comparison: an argument error, an unresolvable permission-check UID,
	// a validator that could not be built, or one or more files failing
	// verification.
	exitVerificationFailed = 1
	// exitUntrustedEnvironment means no file was verified at all because the
	// environment could not be trusted. Its causes are told apart by the
	// identification tokens below, not by this code.
	exitUntrustedEnvironment = 3
)

// Identification tokens. Every message that ends the run without verifying a
// single file carries one, spelled "verify-error=<cause>", so a calling script
// can tell the causes apart without matching on prose -- exit code 3 has four of
// them, and the misconfiguration below shares exit code 1 with an ordinary
// verification failure. The user documentation lists them (task 0164 step 4-11);
// tests refer to these constants rather than repeating the strings.
const (
	// The hash directory or an ancestor is writable by someone else.
	causeHashDirPermissionViolation = "hash_dir_permission_violation"
	// The hash directory path could not be resolved, so the directory whose
	// permissions were checked is not reliably the one that would be read.
	causePathResolutionFailed = "path_resolution_failed"
	// The hash directory does not exist. There is nothing to verify against.
	causeHashDirNotFound = "hash_dir_not_found"
	// The hash directory exists but its records cannot be reached.
	causeHashDirUnreadable = "hash_dir_unreadable"
	// The directory permission checker could not be built.
	causePermissionCheckerInitFailed = "permission_checker_init_failed"
	// The hash directory path names something other than a directory. Unlike the
	// causes above this is a misconfiguration, so it exits with
	// exitVerificationFailed.
	causeHashDirNotADirectory = "hash_dir_not_a_directory"
)

var errNoFilesProvided = errors.New("at least one file path must be provided as a positional argument or via -file (deprecated)")

// deps holds injectable dependencies for the verify command, keeping tests off
// process-wide mutable state. defaultDeps sets every field; only the ones whose
// comment says so may be left nil.
type deps struct {
	validatorFactory func(hashDir string) (hashValidator, error)
	// A constructor rather than a ready-made checker, so that no test can bypass
	// the production construction path.
	newPermChecker      func() (security.DirectoryPermChecker, error)
	resolvePathForCheck func(path string) (string, error)
	// hashDirSearchable is the probe below. It is injected for the same reason the
	// permission checker is: a test whose subject is something else must not
	// depend on the state of a real directory on the host -- the default hash
	// directory exists on a machine where the command has been installed and not
	// on a CI runner.
	hashDirSearchable func(dir string) error
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
		hashDirSearchable:   hashDirSearchable,
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

// hashValidator is the smallest surface verify needs from a validator.
type hashValidator interface {
	Verify(filePath string) error
	// HashDirError reports a hash directory that could not be used, detected when
	// the validator was built. Nil means it was usable.
	HashDirError() error
}

type verifyConfig struct {
	files          []string
	hashDir        string
	usedDeprecated bool
}

func main() {
	os.Exit(run(os.Args[1:], defaultDeps(), os.Stdout, os.Stderr))
}

// hashDirCheck is what a passing hash directory check hands to the rest of the
// run: the checker and the directories it checked, both reused by the target
// file side, and the resolved hash directory -- the path whose permissions were
// actually established, which is the one the validator must be built on.
type hashDirCheck struct {
	checker  security.DirectoryPermChecker
	dirs     []string
	resolved string
}

// checkHashDirPermissions reports whether verification may proceed, judged on
// the hash directory alone. It returns exitOK to proceed; any other exit code is
// run's result.
//
// Only the hash directory side is fail-closed. It is the root of trust: if it
// or an ancestor is writable by someone else, hash records can be replaced and
// any verdict this command produces is meaningless. A violation confined to a
// target file's ancestors is the opposite case — that is what verify exists to
// inspect — so it stays a warning and verification continues (see
// checkTargetFilePermissions).
func checkHashDirPermissions(cfg *verifyConfig, d deps, logger *slog.Logger, stderr io.Writer) (hashDirCheck, int) {
	checker, err := d.newPermChecker()
	if err != nil {
		// Reported rather than panicked: a checker that cannot be built means this
		// run has no way to establish trust, which is an ordinary fail-closed exit
		// and not a programming error worth a stack trace.
		fmt.Fprintf(stderr, "Error: failed to initialise the directory permission checker: %v — no file was verified (verify-error=%s)\n", err, causePermissionCheckerInitFailed) //nolint:errcheck
		return hashDirCheck{}, exitUntrustedEnvironment
	}

	// The hash directory need not exist, so resolving it cannot mean following
	// every component: symlink resolution fails outright on an absent path, and
	// the unresolved path left behind makes a symlinked ancestor look like a "not
	// a directory" violation. The shared helper resolves as far as the deepest
	// existing ancestor instead.
	absHashDir, resolveErr := d.resolvePathForCheck(cfg.hashDir)
	if resolveErr != nil {
		// Fail closed rather than check the path resolution handed back. For most
		// failures that path is one the checker would reject anyway, so stopping
		// here only names the cause more precisely. But when the not-yet-existing
		// part holds a "..", resolution returns the deepest existing ancestor --
		// a shorter, possibly healthy directory that is not the one this run would
		// read from. Checking that would establish the permissions of one
		// directory and then trust another.
		logger.Error("cannot resolve the hash directory for the permission check — refusing to verify",
			slog.String("path", cfg.hashDir),
			slog.String("error", resolveErr.Error()),
		)
		fmt.Fprintf(stderr, "Error: cannot resolve hash directory %s for the permission check: %v — verification results cannot be trusted; no file was verified (verify-error=%s)\n", cfg.hashDir, resolveErr, causePathResolutionFailed) //nolint:errcheck
		return hashDirCheck{}, exitUntrustedEnvironment
	}

	// A hash path that exists but is not a directory is a misconfiguration, not
	// an untrusted environment: nothing can be read from it, so there is no trust
	// to establish. It is diagnosed here because the permission check below would
	// otherwise reach it first and report it as a directory permission violation,
	// telling the operator to fix the permissions of a plain file.
	// filevalidator.NewReadOnly names the same case ErrHashPathNotDir, but the
	// validator is not built until the hash directory has been judged.
	// A failing Lstat is left alone: a missing directory is diagnosed after the
	// validator is built, and one that cannot be stat'ed is a violation the check
	// below reports. absHashDir has already been resolved, so a symlink to a
	// directory is a directory here.
	// #nosec G703 -- the path comes from this command's own -hash-dir argument, and
	// inspecting it is the point: nothing is opened or read here.
	if info, err := os.Lstat(absHashDir); err == nil && !info.Mode().IsDir() {
		fmt.Fprintf(stderr, "Error: hash directory %s is not a directory — no file was verified (verify-error=%s)\n", cfg.hashDir, causeHashDirNotADirectory) //nolint:errcheck
		return hashDirCheck{}, exitVerificationFailed
	}

	hashDirs := security.CollectPermissionCheckDirs(nil, []string{absHashDir})
	if violations := security.RunTOCTOUPermissionCheck(checker, hashDirs, logger).Violations; len(violations) > 0 {
		// The remediation names neither a directory nor a command: v.Path is the
		// directory that was checked, not necessarily the one at fault (the checker
		// walks from the root down), and ErrInvalidDirPermissions covers causes
		// needing chmod go-w, chown or chmod g-w, which cannot be told apart without
		// matching on the error text. The violation attribute carries both.
		const remediation = "fix the permissions and ownership of the directory named in the violation, or move the hash directory to a properly permissioned path, then re-run verify"
		// ERROR, on top of the WARN RunTOCTOUPermissionCheck already logged: the
		// level is what separates a violation that stopped the run from one that
		// did not.
		for _, v := range violations {
			logger.Error(
				"hash directory permission violation detected — refusing to verify",
				slog.String("path", v.Path),
				slog.String("violation", v.Err.Error()),
				slog.String("remediation", remediation),
			)
		}
		fmt.Fprintf(stderr, "Error: permission violation in hash directory or its ancestor directories — verification results cannot be trusted; no file was verified. Fix directory permissions and re-run. (verify-error=%s)\n", causeHashDirPermissionViolation) //nolint:errcheck
		return hashDirCheck{}, exitUntrustedEnvironment
	}

	return hashDirCheck{checker: checker, dirs: hashDirs, resolved: absHashDir}, exitOK
}

// hashDirSearchable reports whether the hash records inside dir can be reached.
//
// Reading a record opens <dir>/<name>, which needs search permission on dir and
// nothing else. A probe that opened dir itself would demand read permission too
// and so refuse a search-only directory that verify can in fact use. Stat'ing
// "<dir>/." performs exactly the lookup a record read performs; filepath.Join
// cannot build that path, since it cleans the "." away.
//
// filevalidator.NewReadOnly does not cover this: it stats the directory, which
// only needs search permission on the parent, so an unusable hash directory
// reaches per-file verification and every file fails as if its hash had changed.
func hashDirSearchable(dir string) error {
	// #nosec G703 -- the path comes from this command's own -hash-dir argument;
	// this stats it and opens nothing.
	_, err := os.Stat(dir + string(filepath.Separator) + ".")
	return err
}

// checkTargetFilePermissions warns about permission violations around the files
// being verified. Unlike the hash directory side it never stops the run: a
// target file sitting in a writable directory is what verify exists to inspect.
//
// hashDirs is what checkHashDirPermissions already checked; passing it in keeps
// a shared ancestor from being warned about twice.
func checkTargetFilePermissions(cfg *verifyConfig, checker security.DirectoryPermChecker, hashDirs []string, logger *slog.Logger) {
	// Resolved here rather than alongside the hash directory: a run that fails
	// closed does not touch the target files at all. A file that cannot be
	// resolved is still checked, on the lexical path resolution hands back, and
	// the reason is recorded rather than swallowed.
	absFiles, _ := security.ResolveAllForCheck(cfg.files, logger)

	checked := make(map[string]struct{}, len(hashDirs))
	for _, dir := range hashDirs {
		checked[dir] = struct{}{}
	}
	var targetDirs []string
	for _, dir := range security.CollectPermissionCheckDirs(absFiles, nil) {
		if _, ok := checked[dir]; !ok {
			targetDirs = append(targetDirs, dir)
		}
	}
	// The result is deliberately dropped: each violation is already logged at
	// WARN by the shared check, and none of them changes the verdict here.
	security.RunTOCTOUPermissionCheck(checker, targetDirs, logger)
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

	logger := slog.Default()

	// The hash directory is judged before anything is read from it: an untrusted
	// directory stops the run before this command asks what is inside it.
	check, exitCode := checkHashDirPermissions(cfg, d, logger, stderr)
	if exitCode != exitOK {
		return exitCode
	}

	// Built on the resolved path, not on the one from the command line, so that
	// the directory read from is the one whose permissions were established. It
	// also spares the validator the symlink it would otherwise reject as "not a
	// directory".
	validator, err := d.validatorFactory(check.resolved)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating validator: %v\n", err) //nolint:errcheck
		return exitVerificationFailed
	}
	// The hash directory's usability is diagnosed once, here, rather than left to
	// the per-file deferred error. It is not a property of any one file, and
	// reporting it as the first file's FAILED line reads as if that file had been
	// tampered with.
	if err := validator.HashDirError(); err != nil {
		if errors.Is(err, filevalidator.ErrHashDirNotExist) {
			fmt.Fprintf(stderr, "Error: hash directory %s does not exist, so there is nothing to verify against — record hashes first with the record command; no file was verified (verify-error=%s)\n", cfg.hashDir, causeHashDirNotFound) //nolint:errcheck
		} else {
			fmt.Fprintf(stderr, "Error: hash directory %s exists but could not be used: %v — check its permissions; no file was verified (verify-error=%s)\n", cfg.hashDir, err, causeHashDirUnreadable) //nolint:errcheck
		}
		return exitUntrustedEnvironment
	}
	if err := d.hashDirSearchable(check.resolved); err != nil {
		fmt.Fprintf(stderr, "Error: hash directory %s exists but its records cannot be reached: %v — check its permissions; no file was verified (verify-error=%s)\n", cfg.hashDir, err, causeHashDirUnreadable) //nolint:errcheck
		return exitUntrustedEnvironment
	}

	checkTargetFilePermissions(cfg, check.checker, check.dirs, logger)

	return processFiles(validator, cfg.files, stdout, stderr)
}

// parseArgs has no side effects: verify never creates the hash directory it
// names, so a missing one is reported later rather than filled in here.
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
