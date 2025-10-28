// Package main provides the record command for the go-safe-cmd-runner.
// It records file hashes for later verification.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/cmdcommon"
)

func main() {
	// Parse command line flags with record-specific options
	var (
		file    = flag.String("file", "", "Path to the file to process")
		hashDir = flag.String("hash-dir", "", "Directory containing hash files")
		force   = flag.Bool("force", false, "Force overwrite existing hash files")
	)
	flag.Parse()

	if *file == "" {
		flag.Usage()
		fmt.Fprintf(os.Stderr, "Error: file argument is required\n")
		os.Exit(1)
	}

	// Set default hash directory if not specified
	dir := *hashDir
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
			os.Exit(1)
		}
		dir = cwd
	}

	// Ensure the directory exists
	const dirPerm = 0o750
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating hash directory: %v\n", err)
		os.Exit(1)
	}

	// Create validator with SHA256 hasher
	validator, err := cmdcommon.CreateValidator(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		os.Exit(1)
	}

	// Record file hash
	hashFile, err := validator.Record(*file, *force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error recording file hash: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Recorded hash for %s in %s\n", *file, hashFile)
}
