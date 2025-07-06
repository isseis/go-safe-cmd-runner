// Package main provides the verify command for the go-safe-cmd-runner.
// It verifies file integrity using previously recorded hashes.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/isseis/go-safe-cmd-runner/internal/filevalidator"
)

const (
	hashDirPermissions = 0o750
)

func main() {
	var (
		file    = flag.String("file", "", "Path to the file to verify")
		hashDir = flag.String("hash-dir", "", "Directory containing hash files (default: current directory)")
	)
	flag.Parse()

	if *file == "" {
		fmt.Fprintf(os.Stderr, "Usage: %s -file <path> [-hash-dir <dir>]\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Set default hash directory to current directory if not specified
	if *hashDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
			os.Exit(1)
		}
		hashDir = &cwd
	}

	// Create hash directory if it doesn't exist (for consistency with record)
	if err := os.MkdirAll(*hashDir, hashDirPermissions); err != nil {
		fmt.Fprintf(os.Stderr, "Error accessing hash directory: %v\n", err)
		os.Exit(1)
	}

	// Create validator with SHA256 hasher
	validator, err := filevalidator.New(&filevalidator.SHA256{}, *hashDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		os.Exit(1)
	}

	// Verify file hash
	err = validator.Verify(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Verification failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("OK: %s\n", *file)
}
