// Package main provides the verify command for the go-safe-cmd-runner.
// It verifies file integrity using previously recorded hashes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
)

var (
	// ErrFileArgumentRequired is returned when the required file argument is missing.
	ErrFileArgumentRequired = errors.New("file argument is required")
	// ErrGetCurrentDir is returned when there's an error getting the current directory.
	ErrGetCurrentDir = errors.New("error getting current directory")
	// ErrCreateHashDir is returned when there's an error creating or accessing the hash directory.
	ErrCreateHashDir = errors.New("error creating/accessing hash directory")
)

const (
	// hashDirPermissions defines the default permissions for the hash directory.
	hashDirPermissions = 0o750
)

// config holds the configuration for the verify command.
type config struct {
	File    string
	HashDir string
}

// parseFlags parses the command-line flags for the verify command.
// It returns a config and any error encountered.
func parseFlags() (*config, error) {
	var (
		file    = flag.String("file", "", "Path to the file to process")
		hashDir = flag.String("hash-dir", "", "Directory containing hash files (default: "+cmdcommon.DefaultHashDirectory+")")
	)
	flag.Parse()

	if *file == "" {
		flag.Usage()
		return nil, ErrFileArgumentRequired
	}

	// Set default hash directory to current directory if not specified
	dir := *hashDir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrGetCurrentDir, err)
		}
		dir = cwd
	}

	// Ensure the directory exists
	if err := os.MkdirAll(dir, hashDirPermissions); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCreateHashDir, err)
	}

	return &config{
		File:    *file,
		HashDir: dir,
	}, nil
}

// printUsage prints the usage message for the verify command.
func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: %s -file <path> [-hash-dir <dir>]\n", filepath.Base(os.Args[0]))
	flag.PrintDefaults()
}

func main() {
	// Parse command line flags
	cfg, err := parseFlags()
	if err != nil {
		printUsage()
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Create validator with SHA256 hasher
	validator, err := cmdcommon.CreateValidator(cfg.HashDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		os.Exit(1)
	}

	// Verify file hash
	err = validator.Verify(cfg.File)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK: %s\n", cfg.File)
}
