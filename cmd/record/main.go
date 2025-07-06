// Package main provides the record command for the go-safe-cmd-runner.
// It records file hashes for later verification.
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
		file    = flag.String("file", "", "Path to the file to record")
		hashDir = flag.String("hash-dir", "", "Directory to store hash files (default: current directory)")
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

	// Create hash directory if it doesn't exist
	if err := os.MkdirAll(*hashDir, hashDirPermissions); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating hash directory: %v\n", err)
		os.Exit(1)
	}

	// Create validator with SHA256 hasher
	validator, err := filevalidator.New(&filevalidator.SHA256{}, *hashDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating validator: %v\n", err)
		os.Exit(1)
	}

	// Record file hash
	hashFile, err := validator.Record(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error recording file hash: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Recorded hash for %s in %s\n", *file, hashFile)
}
