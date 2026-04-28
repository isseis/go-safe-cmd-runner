// Package main provides the record command for the go-safe-cmd-runner.
// It records file hashes for later verification and now supports multiple files.
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
	"github.com/isseis/go-safe-cmd-runner/internal/elfdynlib"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/libccache"
	"github.com/isseis/go-safe-cmd-runner/internal/machodylib"
	rsec "github.com/isseis/go-safe-cmd-runner/internal/runner/security"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
	isec "github.com/isseis/go-safe-cmd-runner/internal/security"
)

const (
	hashDirPermissions = 0o750
	libcCacheSubDir    = "lib-cache"
)

var (
	errNoFilesProvided = errors.New("at least one file path must be provided as a positional argument or via -file (deprecated)")
	errEnsureHashDir   = errors.New("error creating hash directory")
)

// deps holds injectable dependencies for the record command.
// This makes the dependency graph visible at call sites and simplifies testing.
type deps struct {
	validatorFactory           func(hashDir string) (hashRecorder, error)
	elfDynlibAnalyzerFactory   func() *elfdynlib.DynLibAnalyzer       // nil means dynlib analysis is disabled
	machoDynlibAnalyzerFactory func() *machodylib.MachODynLibAnalyzer // nil means Mach-O dynlib analysis is disabled
	mkdirAll                   func(path string, perm os.FileMode) error
}

func defaultDeps() deps {
	return deps{
		validatorFactory: func(hashDir string) (hashRecorder, error) {
			return filevalidator.New(&filevalidator.SHA256{}, hashDir)
		},
		elfDynlibAnalyzerFactory: func() *elfdynlib.DynLibAnalyzer {
			return elfdynlib.NewDynLibAnalyzer(safefileio.NewFileSystem(safefileio.FileSystemConfig{}))
		},
		machoDynlibAnalyzerFactory: func() *machodylib.MachODynLibAnalyzer {
			return machodylib.NewMachODynLibAnalyzer(safefileio.NewFileSystem(safefileio.FileSystemConfig{}))
		},
		mkdirAll: os.MkdirAll,
	}
}

// hashRecorder records the hash of a file and returns the hash file path,
// the content hash in prefixed format (e.g., "sha256:<hex>"), and any error.
// Implementations must return the content hash in "<algorithm>:<hex>" format,
// as it is passed directly to syscall analysis storage.
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

func run(args []string, d deps, stdout, stderr io.Writer) int {
	cfg, fs, err := parseArgs(args, d, stderr)
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

	// Run TOCTOU permission check on directories referenced by this operation.
	// record does not have a config with verify_files or commands; check the files being
	// recorded and the hash directory. Violations are logged as warnings only — record
	// continues even if the check fails.
	secValidator, secErr := rsec.NewValidatorForTOCTOU()
	if secErr != nil {
		// NewValidatorForTOCTOU only fails when a regex literal in DefaultConfig
		// is invalid — a programming error that cannot be recovered at runtime.
		panic(fmt.Sprintf("security validator initialisation failed (invalid built-in regex pattern): %v", secErr))
	}
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
	absHashDir := cfg.hashDir
	if abs, err := filepath.Abs(cfg.hashDir); err == nil {
		if resolved, err := filepath.EvalSymlinks(abs); err == nil {
			absHashDir = resolved
		} else {
			absHashDir = abs
		}
	}
	toctouDirs := isec.CollectTOCTOUCheckDirs(absFiles, nil, absHashDir)
	isec.RunTOCTOUPermissionCheck(secValidator, toctouDirs, slog.Default())

	validator, err := d.validatorFactory(cfg.hashDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating validator: %v\n", err) //nolint:errcheck
		return 1
	}

	// Inject DynLibAnalyzer, BinaryAnalyzer, SyscallAnalyzer, and LibcCache when the validator supports it.
	// Uses a type assertion so that test fakes implementing only hashRecorder are unaffected.
	if fv, ok := validator.(*filevalidator.Validator); ok {
		if d.elfDynlibAnalyzerFactory != nil {
			fv.SetELFDynLibAnalyzer(d.elfDynlibAnalyzerFactory())
		}
		if d.machoDynlibAnalyzerFactory != nil {
			fv.SetMachODynLibAnalyzer(d.machoDynlibAnalyzerFactory())
		}
		fv.SetBinaryAnalyzer(rsec.NewBinaryAnalyzer(runtime.GOOS))

		syscallAnalyzer := elfanalyzer.NewSyscallAnalyzer()
		fv.SetSyscallAnalyzer(libccache.NewSyscallAdapter(syscallAnalyzer))

		cacheDir := filepath.Join(cfg.hashDir, libcCacheSubDir)
		fs := safefileio.NewFileSystem(safefileio.FileSystemConfig{})
		libcAnalyzer := libccache.NewLibcWrapperAnalyzer(syscallAnalyzer)
		cacheMgr, cacheErr := libccache.NewLibcCacheManager(cacheDir, fs, libcAnalyzer)
		if cacheErr != nil {
			fmt.Fprintf(stderr, "Error: Failed to initialize libc cache: %v\n", cacheErr) //nolint:errcheck
			return 1
		}
		fv.SetLibcCache(libccache.NewCacheAdapter(cacheMgr, syscallAnalyzer))
		fv.SetIncludeDebugInfo(cfg.debugInfo)

		// Inject MachoLibSystemAdapter for Mach-O libSystem import-symbol matching.
		machoCacheDir := filepath.Join(cfg.hashDir, libcCacheSubDir)
		machoCacheMgr, machoCacheErr := libccache.NewMachoLibSystemCacheManager(machoCacheDir)
		if machoCacheErr != nil {
			fmt.Fprintf(stderr, "Error: Failed to initialize machoLibSystem cache: %v\n", machoCacheErr) //nolint:errcheck
			return 1
		}
		fv.SetLibSystemCache(libccache.NewMachoLibSystemAdapter(machoCacheMgr, fs))
		fv.SetMachoSyscallTable(libccache.MacOSSyscallTable{})
	}

	return processFiles(validator, cfg, stdout, stderr)
}

func parseArgs(args []string, d deps, stderr io.Writer) (*recordConfig, *flag.FlagSet, error) {
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

	if err := d.mkdirAll(dir, hashDirPermissions); err != nil {
		return nil, fs, fmt.Errorf("%w: %w", errEnsureHashDir, err)
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
