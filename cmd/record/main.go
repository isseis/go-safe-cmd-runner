// Package main provides the record command for the go-safe-cmd-runner.
// It records file hashes for later verification and now supports multiple files.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
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
)

type hashRecorder interface {
	Record(filePath string, force bool) (string, error)
}

type recordConfig struct {
	files          []string
	hashDir        string
	force          bool
	usedDeprecated bool
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

	return processFiles(recorder, cfg.files, cfg.force, stdout, stderr)
}

func parseArgs(args []string, stderr io.Writer) (*recordConfig, *flag.FlagSet, error) {
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

	if err := os.MkdirAll(dir, hashDirPermissions); err != nil {
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
	_, _ = fmt.Fprintf(w, "Usage: %s [flags] <file> [<file>...]\n", filepath.Base(os.Args[0]))
	fs.PrintDefaults()
}

func processFiles(recorder hashRecorder, files []string, force bool, stdout, stderr io.Writer) int {
	total := len(files)
	label := "files"
	if total == 1 {
		label = "file"
	}
	_, _ = fmt.Fprintf(stdout, "Processing %d %s...\n", total, label)
	successes := 0
	failures := 0

	for idx, filePath := range files {
		_, _ = fmt.Fprintf(stdout, "[%d/%d] %s: ", idx+1, total, filePath)
		hashFile, err := recorder.Record(filePath, force)
		if err != nil {
			failures++
			_, _ = fmt.Fprintln(stdout, "FAILED")
			_, _ = fmt.Fprintf(stderr, "Error recording hash for %s: %v\n", filePath, err)
			continue
		}
		successes++
		_, _ = fmt.Fprintf(stdout, "OK (%s)\n", hashFile)
	}

	_, _ = fmt.Fprintf(stdout, "\nSummary: %d succeeded, %d failed\n", successes, failures)
	if failures > 0 {
		return 1
	}
	return 0
}
