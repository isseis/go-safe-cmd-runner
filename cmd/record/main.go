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
	validatorFactory   = func(hashDir string) (hashRecorder, error) {
		return cmdcommon.CreateValidator(hashDir)
	}
	mkdirAll = os.MkdirAll
)

type hashRecorder interface {
	Record(filePath string, force bool) (string, error)
}

type recordConfig struct {
	files           []string
	hashDir         string
	force           bool
	analyzeSyscalls bool
	usedDeprecated  bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	cfg, fs, err := parseArgs(args, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		printUsage(fs, stderr)
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}

	if cfg.usedDeprecated {
		_, _ = fmt.Fprintln(stderr, "Warning: -file flag is deprecated and will be removed in a future release. Specify files as positional arguments.")
	}

	recorder, err := validatorFactory(cfg.hashDir)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error creating validator: %v\n", err)
		return 1
	}

	return processFiles(recorder, cfg, stdout, stderr)
}

func parseArgs(args []string, stderr io.Writer) (*recordConfig, *flag.FlagSet, error) {
	options := struct {
		deprecatedFile  string
		hashDir         string
		force           bool
		analyzeSyscalls bool
	}{}

	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { printUsage(fs, stderr) }
	fs.StringVar(&options.deprecatedFile, "file", "", "DEPRECATED: Path to the file to process (use positional arguments instead)")
	fs.StringVar(&options.hashDir, "hash-dir", "", "Directory containing hash files (default: current working directory)")
	fs.StringVar(&options.hashDir, "d", "", "Short alias for -hash-dir")
	fs.BoolVar(&options.force, "force", false, "Force overwrite existing hash files")
	fs.BoolVar(&options.analyzeSyscalls, "analyze-syscalls", false, "Analyze syscalls for static ELF binaries")

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

	if err := mkdirAll(dir, hashDirPermissions); err != nil {
		return nil, fs, fmt.Errorf("%w: %w", errEnsureHashDir, err)
	}

	return &recordConfig{
		files:           files,
		hashDir:         dir,
		force:           options.force,
		analyzeSyscalls: options.analyzeSyscalls,
		usedDeprecated:  options.deprecatedFile != "",
	}, fs, nil
}

func printUsage(fs *flag.FlagSet, w io.Writer) {
	if fs == nil {
		return
	}
	_, _ = fmt.Fprintf(w, "Usage: %s [flags] <file> [<file>...]\n", filepath.Base(os.Args[0]))
	fs.PrintDefaults()
}

func processFiles(recorder hashRecorder, cfg *recordConfig, stdout, stderr io.Writer) int {
	total := len(cfg.files)
	label := "files"
	if total == 1 {
		label = "file"
	}
	_, _ = fmt.Fprintf(stdout, "Processing %d %s...\n", total, label)
	successes := 0
	failures := 0

	// Create syscall analyzer context if enabled
	var syscallCtx *syscallAnalysisContext
	if cfg.analyzeSyscalls {
		var err error
		syscallCtx, err = newSyscallAnalysisContext(cfg.hashDir)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "Warning: Failed to initialize syscall analysis: %v\n", err)
			// Continue without syscall analysis
		}
	}

	for idx, filePath := range cfg.files {
		_, _ = fmt.Fprintf(stdout, "[%d/%d] %s: ", idx+1, total, filePath)
		hashFile, err := recorder.Record(filePath, cfg.force)
		if err != nil {
			failures++
			_, _ = fmt.Fprintln(stdout, "FAILED")
			_, _ = fmt.Fprintf(stderr, "Error recording hash for %s: %v\n", filePath, err)
			continue
		}
		successes++
		_, _ = fmt.Fprintf(stdout, "OK (%s)\n", hashFile)

		// Perform syscall analysis if enabled
		if syscallCtx != nil {
			if err := syscallCtx.analyzeFile(filePath); err != nil {
				// ErrNotELF and ErrNotStaticELF are expected for non-analyzable files
				if !errors.Is(err, elfanalyzer.ErrNotELF) && !errors.Is(err, elfanalyzer.ErrNotStaticELF) {
					_, _ = fmt.Fprintf(stderr, "Warning: Syscall analysis failed for %s: %v\n", filePath, err)
				}
			}
		}
	}

	_, _ = fmt.Fprintf(stdout, "\nSummary: %d succeeded, %d failed\n", successes, failures)
	if failures > 0 {
		return 1
	}
	return 0
}

// syscallAnalysisContext holds resources for syscall analysis.
type syscallAnalysisContext struct {
	store        *fileanalysis.Store
	syscallStore fileanalysis.SyscallAnalysisStore
	analyzer     *elfanalyzer.SyscallAnalyzer
	hashAlgo     filevalidator.HashAlgorithm
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
		store:        store,
		syscallStore: fileanalysis.NewSyscallAnalysisStore(store),
		analyzer:     elfanalyzer.NewSyscallAnalyzer(),
		hashAlgo:     &filevalidator.SHA256{},
		fs:           safefileio.NewFileSystem(safefileio.FileSystemConfig{}),
	}, nil
}

// analyzeFile performs syscall analysis on a file if it's a static ELF binary.
// Returns ErrNotELF if the file is not an ELF binary.
// Returns ErrNotStaticELF if the ELF file is dynamically linked.
func (ctx *syscallAnalysisContext) analyzeFile(path string) error {
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

	// Check if static ELF (no .dynsym section) using the already-opened file
	if dynsym := elfFile.Section(".dynsym"); dynsym != nil {
		return elfanalyzer.ErrNotStaticELF
	}

	// Perform syscall analysis
	result, err := ctx.analyzer.AnalyzeSyscallsFromELF(elfFile)
	if err != nil {
		return fmt.Errorf("analysis failed: %w", err)
	}

	// Rewind file for hash calculation
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to rewind file: %w", err)
	}

	// Calculate file hash with algorithm prefix
	rawHash, err := ctx.hashAlgo.Sum(file)
	if err != nil {
		return fmt.Errorf("failed to calculate hash: %w", err)
	}
	// ContentHash requires prefixed format: "sha256:<hex>"
	contentHash := fmt.Sprintf("%s:%s", ctx.hashAlgo.Name(), rawHash)

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
// This is necessary because the two packages have independent type definitions to avoid import cycles.
func convertToFileanalysisResult(result *elfanalyzer.SyscallAnalysisResult) *fileanalysis.SyscallAnalysisResult {
	syscalls := make([]fileanalysis.SyscallInfo, len(result.DetectedSyscalls))
	for i, s := range result.DetectedSyscalls {
		syscalls[i] = fileanalysis.SyscallInfo{
			Number:              s.Number,
			Name:                s.Name,
			IsNetwork:           s.IsNetwork,
			Location:            s.Location,
			DeterminationMethod: s.DeterminationMethod,
		}
	}

	return &fileanalysis.SyscallAnalysisResult{
		DetectedSyscalls:   syscalls,
		HasUnknownSyscalls: result.HasUnknownSyscalls,
		HighRiskReasons:    result.HighRiskReasons,
		Summary: fileanalysis.SyscallSummary{
			HasNetworkSyscalls:  result.Summary.HasNetworkSyscalls,
			IsHighRisk:          result.Summary.IsHighRisk,
			TotalDetectedEvents: result.Summary.TotalDetectedEvents,
			NetworkSyscallCount: result.Summary.NetworkSyscallCount,
		},
	}
}
