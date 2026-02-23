// Package main provides the record command for the go-safe-cmd-runner.
// It records file hashes for later verification and now supports multiple files.
package main

import (
	"debug/elf"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
	"github.com/isseis/go-safe-cmd-runner/internal/fileanalysis"
	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
	"github.com/isseis/go-safe-cmd-runner/internal/runner/security/elfanalyzer"
	"github.com/isseis/go-safe-cmd-runner/internal/safefileio"
)

const (
	hashDirPermissions = 0o750
)

var (
	errNoFilesProvided = errors.New("at least one file path must be provided as a positional argument or via -file (deprecated)")
	errEnsureHashDir   = errors.New("error creating hash directory")
)

// deps holds injectable dependencies for the record command.
// This makes the dependency graph visible at call sites and simplifies testing.
type deps struct {
	validatorFactory      func(hashDir string) (hashRecorder, error)
	syscallContextFactory func(hashDir string) (*syscallAnalysisContext, error)
	mkdirAll              func(path string, perm os.FileMode) error
}

func defaultDeps() deps {
	return deps{
		validatorFactory: func(hashDir string) (hashRecorder, error) {
			return cmdcommon.CreateValidator(hashDir)
		},
		syscallContextFactory: newSyscallAnalysisContext,
		mkdirAll:              os.MkdirAll,
	}
}

// hashRecorder records the hash of a file and returns the hash file path,
// the content hash in prefixed format (e.g., "sha256:<hex>"), and any error.
// Implementations must return the content hash in "<algorithm>:<hex>" format,
// as it is passed directly to syscall analysis storage.
type hashRecorder interface {
	Record(filePath string, force bool) (string, string, error)
}

type recordConfig struct {
	files          []string
	hashDir        string
	force          bool
	usedDeprecated bool
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

	recorder, err := d.validatorFactory(cfg.hashDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating validator: %v\n", err) //nolint:errcheck
		return 1
	}

	syscallCtx, err := d.syscallContextFactory(cfg.hashDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error: Failed to initialize syscall analysis: %v\n", err) //nolint:errcheck
		return 1
	}

	return processFiles(recorder, syscallCtx, cfg, stdout, stderr)
}

func parseArgs(args []string, d deps, stderr io.Writer) (*recordConfig, *flag.FlagSet, error) {
	options := struct {
		deprecatedFile string
		hashDir        string
		force          bool
	}{}

	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(fs, stderr) }
	fs.StringVar(&options.deprecatedFile, "file", "", "DEPRECATED: Path to the file to process (use positional arguments instead)")
	fs.StringVar(&options.hashDir, "hash-dir", "", "Directory containing hash files (default: current working directory)")
	fs.StringVar(&options.hashDir, "d", "", "Short alias for -hash-dir")
	fs.BoolVar(&options.force, "force", false, "Force overwrite existing hash files")

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
	}, fs, nil
}

func printUsage(fs *flag.FlagSet, w io.Writer) {
	if fs == nil {
		return
	}
	fmt.Fprintf(w, "Usage: %s [flags] <file> [<file>...]\n", filepath.Base(os.Args[0])) //nolint:errcheck
	fs.PrintDefaults()
}

func processFiles(recorder hashRecorder, syscallCtx *syscallAnalysisContext, cfg *recordConfig, stdout, stderr io.Writer) int {
	total := len(cfg.files)
	label := "files"
	if total == 1 {
		label = "file"
	}
	fmt.Fprintf(stdout, "Processing %d %s...\n", total, label) //nolint:errcheck
	successes := 0
	failures := 0

	for idx, filePath := range cfg.files {
		fmt.Fprintf(stdout, "[%d/%d] %s: ", idx+1, total, filePath) //nolint:errcheck
		hashFile, contentHash, err := recorder.Record(filePath, cfg.force)
		if err != nil {
			failures++
			fmt.Fprintln(stdout, "FAILED")                                          //nolint:errcheck
			fmt.Fprintf(stderr, "Error recording hash for %s: %v\n", filePath, err) //nolint:errcheck
			continue
		}
		successes++
		fmt.Fprintf(stdout, "OK (%s)\n", hashFile) //nolint:errcheck

		// Perform syscall analysis for static ELF binaries
		if err := syscallCtx.analyzeFile(filePath, contentHash); err != nil {
			// ErrNotELF, ErrNotStaticELF, and file-not-found are expected for non-analyzable files
			if !errors.Is(err, elfanalyzer.ErrNotELF) && !errors.Is(err, elfanalyzer.ErrNotStaticELF) && !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintf(stderr, "Warning: Syscall analysis failed for %s: %v\n", filePath, err) //nolint:errcheck
			}
		}
	}

	fmt.Fprintf(stdout, "\nSummary: %d succeeded, %d failed\n", successes, failures) //nolint:errcheck
	if failures > 0 {
		return 1
	}
	return 0
}

// elfSyscallAnalyzer is the interface for analyzing syscalls from an ELF file.
// It is satisfied by *elfanalyzer.SyscallAnalyzer and can be replaced in tests.
type elfSyscallAnalyzer interface {
	AnalyzeSyscallsFromELF(elfFile *elf.File) (*elfanalyzer.SyscallAnalysisResult, error)
}

// syscallAnalysisContext holds resources for syscall analysis.
type syscallAnalysisContext struct {
	syscallStore fileanalysis.SyscallAnalysisStore
	analyzer     elfSyscallAnalyzer
	fs           safefileio.FileSystem
}

// newSyscallAnalysisContext creates a new syscall analysis context.
func newSyscallAnalysisContext(hashDir string) (*syscallAnalysisContext, error) {
	pathGetter := filevalidator.NewHybridHashFilePathGetter()
	store, err := fileanalysis.NewStore(hashDir, pathGetter)
	if err != nil {
		return nil, fmt.Errorf("failed to create analysis store: %w", err)
	}

	return &syscallAnalysisContext{
		syscallStore: fileanalysis.NewSyscallAnalysisStore(store),
		analyzer:     elfanalyzer.NewSyscallAnalyzer(),
		fs:           safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
	}, nil
}

// analyzeFile performs syscall analysis on a file if it's a static ELF binary.
// contentHash is the prefixed hash (e.g., "sha256:<hex>") already computed by Record.
// Returns ErrNotELF if the file is not an ELF binary.
// Returns ErrNotStaticELF if the ELF file is dynamically linked.
func (ctx *syscallAnalysisContext) analyzeFile(path string, contentHash string) error {
	// Open file securely - single open for both check and analysis
	file, err := ctx.fs.SafeOpenFile(path, os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open file securely: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Parse ELF from secure file handle
	elfFile, err := elf.NewFile(file)
	if err != nil {
		// Not an ELF file - this is not an error, just skip analysis
		return elfanalyzer.ErrNotELF
	}
	defer func() { _ = elfFile.Close() }()

	// Check if the ELF is dynamically linked (i.e., not static) by checking for a .dynsym section
	if dynsym := elfFile.Section(".dynsym"); dynsym != nil {
		return elfanalyzer.ErrNotStaticELF
	}

	// Perform syscall analysis
	result, err := ctx.analyzer.AnalyzeSyscallsFromELF(elfFile)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Convert elfanalyzer.SyscallAnalysisResult to fileanalysis.SyscallAnalysisResult
	faResult := convertToFileanalysisResult(result)

	// Save syscall analysis using the reusable store
	if err := ctx.syscallStore.SaveSyscallAnalysis(path, contentHash, faResult); err != nil {
		return fmt.Errorf("failed to save analysis result: %w", err)
	}

	// Log summary
	slog.Info("Syscall analysis completed",
		"path", path,
		"total_detected_events", result.Summary.TotalDetectedEvents,
		"network_syscalls", result.Summary.NetworkSyscallCount,
		"high_risk", result.Summary.IsHighRisk)

	return nil
}

// convertToFileanalysisResult converts elfanalyzer.SyscallAnalysisResult to fileanalysis.SyscallAnalysisResult.
// Both types embed common.SyscallAnalysisResultCore, enabling direct struct copy
// without field-by-field assignment. The elfanalyzer-specific DecodeStats field
// is intentionally excluded as it is not persisted to storage.
func convertToFileanalysisResult(result *elfanalyzer.SyscallAnalysisResult) *fileanalysis.SyscallAnalysisResult {
	return &fileanalysis.SyscallAnalysisResult{
		SyscallAnalysisResultCore: result.SyscallAnalysisResultCore,
	}
}
