// Package main provides the record command for the go-safe-cmd-runner.
// It records the hashes of one or more files for later verification.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/dynamicanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/dynlib/machodylib"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/groupmembership"
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	"github.com/isseis/go-safe-cmd-runner/internal/security"
	"github.com/isseis/go-safe-cmd-runner/internal/security/elfanalyzer"
)

func init() {
	// record is invoked as `sudo record ...`; the read-safety check must judge
	// access from the invoking user's point of view, so SUDO_UID is consulted.
	if err := groupmembership.SetProcessPermissionCheckUIDPolicy(groupmembership.SudoUIDAware); err != nil {
		panic(fmt.Sprintf("failed to declare permission check UID policy %s (current=%s): %v",
			groupmembership.SudoUIDAware, groupmembership.ProcessPermissionCheckUIDPolicy(), err))
	}
}

const libcCacheSubDir = "lib-cache"

var (
	errNoFilesProvided = errors.New("at least one file path must be provided as a positional argument or via -file (deprecated)")
	errEnsureHashDir   = errors.New("error creating hash directory")
)

// deps holds injectable dependencies for the record command.
// This makes the dependency graph visible at call sites and simplifies testing.
// Only the fields whose comment says so may be left nil; the rest are required
// and are set by defaultDeps.
type deps struct {
	validatorFactory           func(hashDir string, cfg filevalidator.ValidatorConfig) (*filevalidator.Validator, error)
	elfDynlibAnalyzerFactory   func() *elfdynlib.DynLibAnalyzer       // nil means dynlib analysis is disabled
	machoDynlibAnalyzerFactory func() *machodylib.MachODynLibAnalyzer // nil means Mach-O dynlib analysis is disabled
	mkdirAll                   func(path string, perm os.FileMode) error
	// newPermChecker builds the directory permission checker.
	// nil means security.NewDirectoryPermChecker. Injected as a constructor rather
	// than a ready-made checker so tests can exercise its error return, which no
	// current implementation produces.
	newPermChecker func() (security.DirectoryPermChecker, error)
	// nil means use groupmembership.New().EnsurePermissionCheckUID.
	ensurePermissionCheckUID func() error
}

func defaultDeps() deps {
	return deps{
		validatorFactory: func(hashDir string, cfg filevalidator.ValidatorConfig) (*filevalidator.Validator, error) {
			return filevalidator.New(&filevalidator.SHA256{}, hashDir, cfg)
		},
		elfDynlibAnalyzerFactory: func() *elfdynlib.DynLibAnalyzer {
			return elfdynlib.NewDynLibAnalyzer(safefileio.NewFileSystem(safefileio.FileSystemConfig{}))
		},
		machoDynlibAnalyzerFactory: func() *machodylib.MachODynLibAnalyzer {
			return machodylib.NewMachODynLibAnalyzer(safefileio.NewFileSystem(safefileio.FileSystemConfig{}))
		},
		mkdirAll:       os.MkdirAll,
		newPermChecker: security.NewDirectoryPermChecker,
	}
}

// hashRecorder records the hash of a file and returns the hash file path, the
// content hash, and any error. Implementations must return the content hash in
// "<algorithm>:<hex>" form (e.g. "sha256:abc..."), as it is passed directly to
// syscall analysis storage.
type hashRecorder interface {
	SaveRecord(filePath string, force bool) (string, string, error)
}

type recordConfig struct {
	files          []string
	hashDir        string
	force          bool
	usedDeprecated bool
	debugInfo      bool
}

func main() {
	os.Exit(run(os.Args[1:], defaultDeps(), os.Stdout, os.Stderr))
}

// checkDirPermissions audits the permissions of the directories this
// operation touches, and reports whether recording may proceed. The hash DB is
// the root of trust — a permission violation in its ancestor directories means
// an attacker could replace hash records — so a violation fails closed, and no
// bypass flag is provided.
//
// On success it returns the resolved hash directory, the path the check was
// actually performed on. The caller must create and use that path rather than
// the one on the command line, so that the directory recorded into is the one
// whose permissions were established.
func checkDirPermissions(cfg *recordConfig, d deps, stderr io.Writer) (resolvedHashDir string, ok bool) {
	newChecker := d.newPermChecker
	if newChecker == nil {
		newChecker = security.NewDirectoryPermChecker
	}
	checker, err := newChecker()
	if err != nil {
		// Reported rather than panicked: a checker that cannot be built means the
		// run has no way to establish trust, which is an ordinary fail-closed exit
		// and not a programming error worth a stack trace.
		fmt.Fprintf(stderr, "Error: failed to initialise the directory permission checker: %v\n", err) //nolint:errcheck
		return "", false
	}

	logger := slog.Default()
	// A target file that cannot be resolved is still checked, on the lexical path
	// resolution hands back, and the reason is recorded rather than swallowed.
	absFiles, _ := security.ResolveAllForCheck(cfg.files, logger)
	// The hash directory is different: it is the root of trust, and an unresolvable
	// path does not reliably name the tree it would end up in. Resolution rejects a
	// ".." in the not-yet-existing part of the path, for instance, and hands back
	// the deepest existing ancestor -- checking that would establish one
	// directory's permissions and then create the hash directory somewhere else.
	absHashDir, resolveErr := security.ResolvePathForCheck(cfg.hashDir)
	if resolveErr != nil {
		logger.Error("cannot resolve the hash directory for the permission check — refusing to record",
			slog.String("path", cfg.hashDir),
			slog.String("error", resolveErr.Error()),
		)
		fmt.Fprintf(stderr, "Error: cannot resolve hash directory %s for the permission check: %v — refusing to generate hash records. Specify a hash directory whose existing ancestors are readable and whose remaining path components are plain names.\n", cfg.hashDir, resolveErr) //nolint:errcheck
		return "", false
	}
	auditDirs := security.CollectPermissionCheckDirs(absFiles, []string{absHashDir})
	// AuditDirectoryPermissions already logs each violation at WARN; the ERROR log
	// below is intentionally in addition to it, since record (unlike other callers
	// of this shared check) escalates violations to a fail-closed, non-zero exit.
	violations := security.AuditDirectoryPermissions(checker, auditDirs, logger).Violations
	if len(violations) == 0 {
		if !checkHashDirWriteSafety(absHashDir, logger, stderr) {
			return "", false
		}
		return absHashDir, true
	}
	for _, v := range violations {
		remediation := fmt.Sprintf("fix directory permissions/ownership and re-run record (reported violation: %v)", v.Err)
		if errors.Is(v.Err, security.ErrInvalidDirPermissions) {
			remediation = fmt.Sprintf("fix directory permissions with chmod (e.g. chmod go-w %s) and re-run record", v.Path)
		}
		logger.Error(
			"hash directory permission violation detected — refusing to record",
			slog.String("path", v.Path),
			slog.String("violation", v.Err.Error()),
			slog.String("remediation", remediation),
		)
	}
	fmt.Fprintln(stderr, "Error: permission violation in hash directory or its ancestor directories — refusing to generate hash records. Fix directory permissions and re-run.") //nolint:errcheck
	return "", false
}

// checkHashDirWriteSafety reports whether hash records may be written under the
// hash directory. It refuses a world-writable directory even when the sticky bit
// is set, which the shared permission check allows.
//
// The sticky bit stops anyone but the owner from removing an entry already in
// the directory; it says nothing about a name nobody has claimed yet. Both names
// this command is about to claim are of that kind: the hash directory itself,
// which an attacker can pre-create as a symlink for os.MkdirAll to follow, and
// every hash record for a file record has not processed, which an attacker can
// pre-plant for verify to trust. So neither the directory nor its creation site
// may be world-writable.
//
// The rule lives in record rather than in security.ValidateDirectoryPermissions
// because it depends on record being about to write those names, which the
// shared check knows nothing about. A directory or creation site whose safety
// cannot be established at all is refused for the same reason: this runs
// immediately before the write.
func checkHashDirWriteSafety(absHashDir string, logger *slog.Logger, stderr io.Writer) bool {
	// #nosec G703 -- the path comes from this command's own -hash-dir argument, and
	// inspecting it is the point: nothing is opened or written here.
	info, err := os.Lstat(absHashDir)
	if err == nil {
		return checkExistingHashDirMode(absHashDir, info, logger, stderr)
	}
	if !errors.Is(err, os.ErrNotExist) {
		logger.Error("cannot determine whether the hash directory exists — refusing to record",
			slog.String("path", absHashDir),
			slog.String("error", err.Error()),
		)
		fmt.Fprintf(stderr, "Error: cannot determine whether hash directory %s exists: %v — refusing to generate hash records.\n", absHashDir, err) //nolint:errcheck
		return false
	}
	return checkHashDirCreationSite(absHashDir, logger, stderr)
}

// checkExistingHashDirMode is the already-exists half of the rule above. The
// shared check has established that this is a directory with a permitted owner,
// so the sticky world-writable case is all that is left to refuse.
func checkExistingHashDirMode(absHashDir string, info os.FileInfo, logger *slog.Logger, stderr io.Writer) bool {
	if info.Mode().Perm()&0o002 == 0 {
		return true
	}
	remediation := fmt.Sprintf("restrict the hash directory with chmod (for example chmod go-w %s) and re-run record, or move it somewhere only you can write", absHashDir)
	logger.Error("hash directory is world-writable — refusing to record",
		slog.String("path", absHashDir),
		slog.String("mode", info.Mode().String()),
		slog.String("remediation", remediation),
	)
	fmt.Fprintf(stderr, "Error: hash directory %s is world-writable — refusing to generate hash records, because anyone could pre-plant a hash record there for a file that record has not processed yet. %s.\n", absHashDir, remediation) //nolint:errcheck
	return false
}

// checkHashDirCreationSite is the not-yet-created half of the rule above,
// applied to the directory the hash directory would be created in.
func checkHashDirCreationSite(absHashDir string, logger *slog.Logger, stderr io.Writer) bool {
	site, err := security.DeepestExistingAncestor(absHashDir)
	if err != nil {
		logger.Error("cannot determine where the hash directory would be created — refusing to record",
			slog.String("path", absHashDir),
			slog.String("error", err.Error()),
		)
		fmt.Fprintf(stderr, "Error: cannot determine where hash directory %s would be created: %v — refusing to generate hash records.\n", absHashDir, err) //nolint:errcheck
		return false
	}
	// Lstat, not Stat, and the entry must be a directory in its own right: were
	// the creation site a symlink, its mode would be the target's, whose ancestors
	// were never checked, while os.MkdirAll would follow the link there.
	info, err := os.Lstat(site) // #nosec G703 -- same as above: a stat of the caller-named creation site
	if err != nil {
		logger.Error("cannot inspect the directory the hash directory would be created in — refusing to record",
			slog.String("path", site),
			slog.String("error", err.Error()),
		)
		fmt.Fprintf(stderr, "Error: cannot inspect directory %s: %v — refusing to generate hash records.\n", site, err) //nolint:errcheck
		return false
	}
	if !info.IsDir() {
		logger.Error("the hash directory would be created under something that is not a directory — refusing to record",
			slog.String("path", absHashDir),
			slog.String("creation_site", site),
			slog.String("mode", info.Mode().String()),
		)
		fmt.Fprintf(stderr, "Error: hash directory %s would be created under %s, which is not a directory — refusing to generate hash records.\n", absHashDir, site) //nolint:errcheck
		return false
	}
	if info.Mode().Perm()&0o002 == 0 {
		return true
	}

	remediation := fmt.Sprintf("create %s yourself (for example with mkdir -m 700 -p) before running record, or move the hash directory somewhere only you can write", absHashDir)
	logger.Error("hash directory would be created in a world-writable directory — refusing to record",
		slog.String("path", absHashDir),
		slog.String("creation_site", site),
		slog.String("remediation", remediation),
	)
	fmt.Fprintf(stderr, "Error: hash directory %s does not exist and would be created in world-writable directory %s — refusing to generate hash records. %s.\n", absHashDir, site, remediation) //nolint:errcheck
	return false
}

func run(args []string, d deps, stdout, stderr io.Writer) int {
	cfg, fs, err := parseArgs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		printUsage(fs, stderr)
		fmt.Fprintf(stderr, "Error: %v\n", err) //nolint:errcheck
		return 1
	}

	if cfg.usedDeprecated {
		fmt.Fprintln(stderr, "Warning: -file flag is deprecated and will be removed in a future release. Specify files as positional arguments.") //nolint:errcheck
	}

	// record declares SudoUIDAware (see init), so this is where an unverifiable
	// SUDO_UID fails the run and where the adoption record is emitted. It must
	// happen here rather than in the per-file reads; see
	// GroupMembership.EnsurePermissionCheckUID.
	ensureUID := d.ensurePermissionCheckUID
	if ensureUID == nil {
		ensureUID = groupmembership.New().EnsurePermissionCheckUID
	}
	if err := ensureUID(); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err) //nolint:errcheck
		return 1
	}

	resolvedHashDir, ok := checkDirPermissions(cfg, d, stderr)
	if !ok {
		return 1
	}
	// Everything below uses the checked path, not the command-line one; see
	// checkDirPermissions.
	cfg.hashDir = resolvedHashDir

	// The first write of the run, and it must stay first: the libc caches, the
	// dynamic analysis store, and validatorFactory by way of filevalidator.New all
	// reach the hash directory through os.MkdirAll on a path underneath it, and
	// would otherwise create it with their own, wider mode.
	if err := d.mkdirAll(cfg.hashDir, fileanalysis.HashDirPerm); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", fmt.Errorf("%w: %w", errEnsureHashDir, err)) //nolint:errcheck
		return 1
	}

	safeFS := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
	syscallAnalyzer := elfanalyzer.NewSyscallAnalyzer()
	cacheDir := filepath.Join(cfg.hashDir, libcCacheSubDir)
	libcAnalyzer := libccache.NewLibcWrapperAnalyzer(syscallAnalyzer)
	cacheMgr, cacheErr := libccache.NewLibcCacheManager(cacheDir, safeFS, libcAnalyzer)
	if cacheErr != nil {
		fmt.Fprintf(stderr, "Error: Failed to initialize libc cache: %v\n", cacheErr) //nolint:errcheck
		return 1
	}

	// Inject MachoLibSystemAdapter for Mach-O libSystem import-symbol matching.
	machoCacheMgr, machoCacheErr := libccache.NewMachoLibSystemCacheManager(cacheDir)
	if machoCacheErr != nil {
		fmt.Fprintf(stderr, "Error: Failed to initialize machoLibSystem cache: %v\n", machoCacheErr) //nolint:errcheck
		return 1
	}

	vCfg := filevalidator.ValidatorConfig{
		BinaryAnalyzer:    security.NewBinaryAnalyzer(runtime.GOOS),
		SyscallAnalyzer:   libccache.NewSyscallAdapter(syscallAnalyzer),
		LibcCache:         libccache.NewCacheAdapter(cacheMgr, syscallAnalyzer),
		LibSystemCache:    libccache.NewMachoLibSystemAdapter(machoCacheMgr, safeFS),
		MachoSyscallTable: libccache.MacOSSyscallTable{},
		DebugInfo:         cfg.debugInfo,
	}
	if d.elfDynlibAnalyzerFactory != nil {
		vCfg.ELFDynLibAnalyzer = d.elfDynlibAnalyzerFactory()
	}
	if d.machoDynlibAnalyzerFactory != nil {
		vCfg.MachODynLibAnalyzer = d.machoDynlibAnalyzerFactory()
	}

	validator, err := d.validatorFactory(cfg.hashDir, vCfg)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating validator: %v\n", err) //nolint:errcheck
		return 1
	}

	dynlibStoreDir := filepath.Join(cfg.hashDir, dynamicanalysis.StoreSubDir)
	dynlibStore, dynlibStoreErr := dynamicanalysis.New(dynlibStoreDir, validator)
	if dynlibStoreErr != nil {
		fmt.Fprintf(stderr, "Error: Failed to initialize dynamic library analysis store: %v\n", dynlibStoreErr) //nolint:errcheck
		return 1
	}
	validator.SetDynamicLibAnalysisStore(dynlibStore)

	return processFiles(validator, cfg, stdout, stderr)
}

// parseArgs turns the command line into a recordConfig. It has no side effects:
// the hash directory it names is created in run, after the permission check.
func parseArgs(args []string, stderr io.Writer) (*recordConfig, *flag.FlagSet, error) {
	options := struct {
		deprecatedFile string
		hashDir        string
		force          bool
		debugInfo      bool
	}{}

	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(fs, stderr) }
	fs.StringVar(&options.deprecatedFile, "file", "", "DEPRECATED: Path to the file to process (use positional arguments instead)")
	fs.StringVar(&options.hashDir, "hash-dir", "", "Directory containing hash files (default: current working directory)")
	fs.StringVar(&options.hashDir, "d", "", "Short alias for -hash-dir")
	fs.BoolVar(&options.force, "force", false, "Force overwrite existing hash files")
	fs.BoolVar(&options.debugInfo, "debug-info", false, "Include debug information (Occurrences, DeterminationStats) in output")

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

	return &recordConfig{
		files:          files,
		hashDir:        dir,
		force:          options.force,
		usedDeprecated: options.deprecatedFile != "",
		debugInfo:      options.debugInfo,
	}, fs, nil
}

func printUsage(fs *flag.FlagSet, w io.Writer) {
	if fs == nil {
		return
	}
	fmt.Fprintf(w, "Usage: %s [flags] <file> [<file>...]\n", filepath.Base(os.Args[0])) //nolint:errcheck,gosec // G705: writing to stdout/stderr, not an HTTP response
	fs.PrintDefaults()
}

func processFiles(recorder hashRecorder, cfg *recordConfig, stdout, stderr io.Writer) int {
	total := len(cfg.files)
	label := "files"
	if total == 1 {
		label = "file"
	}
	fmt.Fprintf(stdout, "Processing %d %s...\n", total, label) //nolint:errcheck,gosec // G705: writing to stdout, not an HTTP response
	successes := 0
	failures := 0

	for idx, filePath := range cfg.files {
		fmt.Fprintf(stdout, "[%d/%d] %s: ", idx+1, total, filePath) //nolint:errcheck,gosec // G705: writing to stdout, not an HTTP response
		hashFile, _, err := recorder.SaveRecord(filePath, cfg.force)
		if err != nil {
			failures++
			fmt.Fprintln(stdout, "FAILED")                                          //nolint:errcheck
			fmt.Fprintf(stderr, "Error recording hash for %s: %v\n", filePath, err) //nolint:errcheck,gosec // G705: writing to stderr, not an HTTP response
			continue
		}
		successes++
		fmt.Fprintf(stdout, "OK (%s)\n", hashFile) //nolint:errcheck,gosec // G705: writing to stdout, not an HTTP response
	}

	fmt.Fprintf(stdout, "\nSummary: %d succeeded, %d failed\n", successes, failures) //nolint:errcheck
	if failures > 0 {
		return 1
	}
	return 0
}
