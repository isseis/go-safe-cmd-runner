// Package main provides the verify command for the go-safe-cmd-runner.
// It verifies file integrity using previously recorded hashes.
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

const hashDirPermissions = 0o750

var (
	errNoFilesProvided = errors.New("at least one file path must be provided as a positional argument or via -file (deprecated)")
	errEnsureHashDir   = errors.New("error creating/accessing hash directory")
	validatorFactory   = func(hashDir string) (hashValidator, error) {
		return cmdcommon.CreateValidator(hashDir)
	}
	mkdirAll = os.MkdirAll
)

type hashValidator interface {
	Verify(filePath string) error
}

type verifyConfig struct {
	files          []string
	hashDir        string
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
		fmt.Fprintf(stderr, "Error: %v\n", err) //nolint:errcheck
		return 1
	}

	if cfg.usedDeprecated {
		fmt.Fprintln(stderr, "Warning: -file flag is deprecated and will be removed in a future release. Specify files as positional arguments.") //nolint:errcheck
	}

	validator, err := validatorFactory(cfg.hashDir)
	if err != nil {
		fmt.Fprintf(stderr, "Error creating validator: %v\n", err) //nolint:errcheck
		return 1
	}

	return processFiles(validator, cfg.files, stdout, stderr)
}

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

	if err := mkdirAll(dir, hashDirPermissions); err != nil {
		return nil, fs, fmt.Errorf("%w: %w", errEnsureHashDir, err)
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
	fmt.Fprintf(w, "Usage: %s [flags] <file> [<file>...]\n", filepath.Base(os.Args[0])) //nolint:errcheck
	fs.PrintDefaults()
}

func processFiles(validator hashValidator, files []string, stdout, stderr io.Writer) int {
	total := len(files)
	label := "files"
	if total == 1 {
		label = "file"
	}

	fmt.Fprintf(stdout, "Verifying %d %s...\n", total, label) //nolint:errcheck

	successes := 0
	failures := 0

	for idx, filePath := range files {
		fmt.Fprintf(stdout, "[%d/%d] %s: ", idx+1, total, filePath) //nolint:errcheck
		if err := validator.Verify(filePath); err != nil {
			failures++
			fmt.Fprintln(stdout, "FAILED")                                         //nolint:errcheck
			fmt.Fprintf(stderr, "Verification failed for %s: %v\n", filePath, err) //nolint:errcheck
			continue
		}
		successes++
		fmt.Fprintln(stdout, "OK") //nolint:errcheck
	}

	fmt.Fprintf(stdout, "\nSummary: %d succeeded, %d failed\n", successes, failures) //nolint:errcheck
	if failures > 0 {
		return 1
	}
	return 0
}
